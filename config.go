package main

import (
	"os"
	"regexp"

	"github.com/pkg/errors"
)

const (
	DefaultInterface = "127.0.0.1:9545"
	DefaultLogLevel  = "debug"
)

type Config struct {
	Interface string
	CertFile  string
	KeyFile   string
	LogFile   string
	LogLevel  string
	CacheDir  string
	Backends  map[string]*Backend
	Routes    []*Route
}

var reTokens = regexp.MustCompile(`[,\s]+`)

func NewConfig() *Config {
	return &Config{
		Interface: any(os.Getenv("LISTEN"), DefaultInterface),
		CertFile:  os.Getenv("CERTFILE"),
		KeyFile:   os.Getenv("KEYFILE"),
		LogFile:   os.Getenv("LOGFILE"),
		LogLevel:  any(os.Getenv("LOGLEVEL"), DefaultLogLevel),
		CacheDir:  os.Getenv("CACHEDIR"),
		Backends:  map[string]*Backend{},
		Routes:    nil,
	}
}

func any(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}

func (c *Config) Validate() error {
	switch {
	case c == nil:
		return errors.New("is nil")
	case c.CertFile != "" && c.KeyFile == "":
		return errors.New("private key not set")
	case c.CertFile == "" && c.KeyFile != "":
		return errors.New("certificate not set")
	case len(c.Backends) == 0:
		return errors.New("backend not set")
	}

	for _, b := range c.Backends {
		if err := b.Validate(); err != nil {
			return errors.Wrap(err, "backend")
		}
	}

	return nil
}
