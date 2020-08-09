package main

import (
	"bytes"
	"fmt"
	"strings"
)

type RawRequest map[string]interface{}
type RawRequests []RawRequest

type Request struct {
	raw    RawRequest
	id     interface{}
	method string
	sig    string
}

func NewRequest(r RawRequest) *Request {
	method, _ := r["method"].(string)
	params := r["params"]

	// Signature is stable as Sprintf is lexically sorting maps
	sig := strings.ToLower(fmt.Sprintf("%s(%v)", method, params))

	return &Request{
		raw:    r,
		method: strings.ToLower(method),
		sig:    sig,
	}
}

func (r *Request) Cachable() bool {
	switch r.method {
	case "eth_getblockbynumber":
		params := r.raw["params"]
		if vals, ok := params.([]interface{}); ok && len(vals) == 2 {
			if s, ok := vals[0].(string); ok {
				if strings.HasPrefix(s, "0x") {
					return true
				}
			}
		}
		return false
	}

	return true
}

type Requests []*Request

func (r Requests) Raw() RawRequests {
	result := RawRequests{}
	for _, req := range r {
		result = append(result, req.raw)
	}
	return result
}

func decodeRequests(data []byte) (Requests, bool, error) {
	var dec interface{}
	jd := fjson.NewDecoder(bytes.NewReader(data))
	jd.UseNumber()
	if err := jd.Decode(&dec); err != nil {
		return nil, false, err
	}
	/*
		if err := fjson.Unmarshal(data, &dec); err != nil {
			return nil, false, err
		}
	*/

	var reqs Requests
	batch := false

	switch v := dec.(type) {
	case []interface{}:
		for _, ent := range v {
			if raw, ok := ent.(map[string]interface{}); ok {
				reqs = append(reqs, NewRequest(raw))
			}
		}
		batch = true
	case map[string]interface{}:
		reqs = append(reqs, NewRequest(v))
	default:
		return nil, false, fmt.Errorf("Unsupported request object: %T", v)
	}

	return reqs, batch, nil
}
