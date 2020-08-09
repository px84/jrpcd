package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/hashicorp/go-cleanhttp"
	jsoniter "github.com/json-iterator/go"
	"github.com/natefinch/lumberjack"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

var fjson = jsoniter.ConfigCompatibleWithStandardLibrary

type Cache struct {
	Config *Config
	Log    zerolog.Logger
	DB     *badger.DB
	Client *http.Client

	id int64
}

func NewCache(c *Config) *Cache {
	var log zerolog.Logger

	cw := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339Nano,
	}

	if c.LogFile != "" {
		log = zerolog.New(
			io.MultiWriter(
				cw,
				zerolog.ConsoleWriter{
					Out: &lumberjack.Logger{
						Filename:   c.LogFile,
						MaxSize:    10, // megabytes
						MaxBackups: 5,
						MaxAge:     7,    //days
						Compress:   true, // disabled by default
					},
					TimeFormat: time.RFC3339Nano,
				}))
	} else {
		log = zerolog.New(cw)
	}
	lvl, err := zerolog.ParseLevel(c.LogLevel)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return &Cache{
		Config: c,
		Log:    log.Level(lvl).With().Timestamp().Logger(),
		DB:     nil,
		// TODO: Support disabling SSL verification
		Client: cleanhttp.DefaultPooledClient(),

		id: 0,
	}
}

func (c *Cache) Start() error {
	if err := c.Config.Validate(); err != nil {
		return errors.Wrap(err, "invalid config")
	}

	if c.Config.CacheDir != "" {
		db, err := badger.Open(badger.DefaultOptions(c.Config.CacheDir))
		if err != nil {
			return err
		}
		c.DB = db
	}

	c.Log.Info().Msg("Listening for JSON RPC on " + c.Config.Interface)
	if c.Config.CertFile != "" {
		return fasthttp.ListenAndServeTLS(c.Config.Interface, c.Config.CertFile, c.Config.KeyFile, c.handleRequest)
	} else {
		return fasthttp.ListenAndServe(c.Config.Interface, c.handleRequest)
	}
}

func (c *Cache) handleRequest(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		c.Log.Error().Msg("Unsupported method")
		ctx.Error("Invalid method", fasthttp.StatusMethodNotAllowed)
		return
	}

	ct := strings.TrimSpace(strings.ToLower(string(ctx.Request.Header.ContentType())))
	switch {
	case ct == "":
		c.Log.Error().Msg("Missing content type")
		ctx.Error("Missing content type", fasthttp.StatusBadRequest)
		return
	case !strings.HasPrefix(ct, "application/json"):
		c.Log.Error().Msg("Invalid content type")
		ctx.Error("Invalid content type", fasthttp.StatusUnsupportedMediaType)
		return
	}

	body := ctx.PostBody()
	c.Log.Debug().Msg("Request: " + string(body))
	reqs, batch, err := decodeRequests(body)
	if err != nil {
		c.Log.Err(err).Msg("Failed to decode request")
		ctx.Error("Failed to decode request: "+err.Error(), fasthttp.StatusBadRequest)
		return
	}

	var resps Responses
	forward := map[*Route]Requests{}
	id2key := map[interface{}]string{}

	for _, req := range reqs {
		req := req
		id, hasID := req.raw["id"]

		if req.method == "" {
			resps = append(resps, Response{"id": id, "error": "Missing method"})
			continue
		}

		if c.DB != nil && req.Cachable() {
			key := []byte(req.sig)
			if resp := c.Get(key); resp != nil {
				c.Log.Debug().Str("key", req.sig).Msg("Request is cached")
				resp["id"] = id
				resps = append(resps, resp)
				continue
			}
			if hasID {
				id2key[id] = req.sig
			}
		}

		var route *Route
		for _, r := range c.Config.Routes {
			if r.MatchRequest(req) {
				route = r
				break
			}
		}

		forward[route] = append(forward[route], req)
	}

	if len(forward) > 0 {
		var mu sync.Mutex
		var fwresps Responses
		var wg sync.WaitGroup

		var errors []string
		for r, reqs := range forward {
			wg.Add(1)
			go func(r *Route, reqs Requests) {
				defer wg.Done()
				resps, err := c.forwardRequest(r, reqs)
				if err != nil {
					errors = append(errors, err.Error())
					return
				}
				/* TurboGeth workaround
				for _, resp := range resps {
					if result, ok := resp["result"].(map[string]interface{}); ok {
						if v, ok := result["totalDifficulty"]; ok {
							if n, ok := v.(json.Number); ok {
								if ni, err := n.Int64(); err == nil {
									result["totalDifficulty"] = fmt.Sprintf("0x%x", ni)
								}
							}
						}
					}
				}
				*/

				mu.Lock()
				fwresps = append(fwresps, resps...)
				mu.Unlock()
			}(r, reqs)
		}

		wg.Wait()
		if len(errors) > 0 {
			ctx.Error("Failed to forward request", fasthttp.StatusInternalServerError)
			return
		}

		resps = append(resps, fwresps...)

		go func() {
			keyval := map[string][]byte{}
			for _, resp := range fwresps {
				id, hasID := resp["id"]
				if !hasID {
					// Ignore replies without id
					continue
				}

				if _, ok := resp["error"]; ok {
					// Do not cache errors
					continue
				}

				if key := id2key[id]; key != "" {
					data, err := fjson.Marshal(resp)
					if err != nil {
						c.Log.Err(err).Msg("Failed to marshal cached response")
						continue
					}
					keyval[key] = data
				}
			}

			if len(keyval) > 0 {
				if err := c.SetMany(keyval); err != nil {
					c.Log.Err(err).Msg("Failed to cache responses")
				}
			}
		}()
	}

	if batch {
		data := toJSON(resps)
		c.Log.Debug().Msgf("Sending response to client: %s", string(data))
		ctx.Success("application/json", data)
		return
	}

	if len(resps) > 0 {
		data := toJSON(resps[0])
		c.Log.Debug().Msgf("Sending response to client: %s", string(data))
		ctx.Success("application/json", data)
		return
	}

	c.Log.Error().Msg("No response from backend")
	ctx.Success("application/json", toJSON(Response{"error": "No response from backend"}))
}

func (c *Cache) forwardRequest(r *Route, reqs Requests) (Responses, error) {
	msgs := reqs.Raw()
	body, err := fjson.Marshal(msgs)
	if err != nil {
		c.Log.Err(err).Msg("Failed to marshal request body")
		return nil, err
	}

	for _, b := range r.backends {
		if c.Log.GetLevel() <= zerolog.DebugLevel {
			c.Log.Debug().Str("url", b.URL).Int("reqs", len(msgs)).Msgf("Forwarding requests: %s", string(body))
		} else {
			c.Log.Info().Str("url", b.URL).Int("reqs", len(msgs)).Msg("Forwarding requests")
		}
		req, err := http.NewRequest("POST", b.URL, bytes.NewReader(body))
		if err != nil {
			c.Log.Err(err).Msg("NewRequest failed")
			return nil, errors.Wrap(err, "NewRequest")
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.Client.Do(req)
		if err != nil {
			c.Log.Err(err).Msg("Backend request failed")
			continue
		}

		defer func() { resp.Body.Close() }()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			c.Log.Err(err).Msg("Failed to read body")
			continue
		}
		c.Log.Debug().Str("url", b.URL).Str("status", resp.Status).Msgf("Received response: %s", string(body))

		if resp.StatusCode != 200 {
			c.Log.Err(err).Str("status", resp.Status).Msg("Unexpected status")
			continue
		}

		resps, err := decodeResponses(body)
		if err != nil {
			c.Log.Err(err).Msg("Failed to decode backend response")
			continue
		}
		return resps, nil
	}

	return nil, errors.New("Backend request failed")
}

func (c *Cache) Get(key []byte) Response {
	var val []byte
	c.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err = item.ValueCopy(nil)
		return err
	})

	if val == nil {
		return nil
	}

	var resp Response
	if err := fjson.Unmarshal(val, &resp); err != nil {
		c.Log.Err(err).Str("key", string(key)).Msg("Failed to unmarshal cached data")
		return nil
	}

	return resp
}

func (c *Cache) Set(key []byte, r Response) error {
	id, hasID := r["id"]
	if hasID {
		delete(r, "id")
	}

	data, err := fjson.Marshal(r)
	if hasID {
		r["id"] = id
	}

	if err != nil {
		return errors.Wrap(err, "marshal")
	}

	txn := c.DB.NewTransaction(true)
	txn.Set(key, data)
	if err := txn.Commit(); err != nil {
		return errors.Wrap(err, "commit")
	}
	return nil
}

func (c *Cache) SetMany(keyval map[string][]byte) error {
	txn := c.DB.NewTransaction(true)
	for k, v := range keyval {
		if err := txn.Set([]byte(k), v); err != nil {
			txn.Discard()
			return err
		}
	}
	if err := txn.Commit(); err != nil {
		return errors.Wrap(err, "commit")
	}
	return nil
}

func toJSON(v interface{}) []byte {
	data, err := fjson.Marshal(v)
	if err != nil {
		log.Fatalf("JSON decoding failed: %v", err)
	}
	return data
}
