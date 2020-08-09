package main

import (
	"regexp"
	"strings"

	"github.com/minio/minio/pkg/wildcard"
	"github.com/pkg/errors"
)

type Route struct {
	Methods  map[string]bool
	Match    string
	Backends []string

	backends []*Backend
}

var reRoute = regexp.MustCompile(`^([a-z0-9,]+)=(.+)`)

func parseRoute(rspec string) (*Route, error) {
	rspec = strings.ToLower(rspec)
	m := reRoute.FindAllStringSubmatch(rspec, -1)
	if len(m) == 0 {
		return nil, errors.New("invalid route spec")
	}

	backends, rules := uniq(downcase(tokens(m[0][1])...)...), m[0][2]
	if len(backends) == 0 {
		return nil, errors.New("missing backends")
	}

	if strings.HasPrefix(rules, "match:") {
		pattern := strings.TrimSpace(strings.TrimPrefix(rules, "match:"))
		return &Route{Methods: nil, Match: pattern, Backends: backends}, nil
	}

	methods := map[string]bool{}
	for _, s := range uniq(tokens(rules)...) {
		methods[s] = true
	}

	return &Route{Methods: methods, Match: "", Backends: backends}, nil
}

func (r *Route) MatchRequest(req *Request) bool {
	if r.Match == "*" {
		return true
	}

	if r.Methods != nil {
		if _, ok := r.Methods[req.method]; ok {
			return true
		}
	}

	/*
		sig := fmt.Sprintf("%s(%v)\n", req.method, req.raw["params"])
		fmt.Printf("sig = %s\n", sig)
	*/

	if r.Match != "" {
		return wildcard.Match(r.Match, req.sig)
	}
	return false
}
