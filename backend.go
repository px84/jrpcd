package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

type Backend struct {
	Name string
	URL  string
}

func (b *Backend) Validate() error {
	switch {
	case b == nil:
		return errors.New("is nil")
	case b.URL == "":
		return errors.New("URL is unset")
	}
	return nil
}

var reNamedBackend = regexp.MustCompile(`(?i)^([a-z0-9]+)=(.+)`)

func parseBackend(s string) (*Backend, error) {
	var name, uri string
	if m := reNamedBackend.FindAllStringSubmatch(s, -1); len(m) > 0 {
		name, uri = strings.ToLower(m[0][1]), m[0][2]
	} else {
		name, uri = "default", s
	}

	if _, err := url.Parse(uri); err != nil {
		return nil, errors.Wrap(err, "url")
	}
	return &Backend{Name: name, URL: uri}, nil
}
