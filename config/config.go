/*
Package config parses config files for meta-graphite.

meta-graphite config files are in JSON format, and consists
of a single JSON object containing string/string pairs. The
key should be a metrics prefix to match, and the value should
be a URL for the graphite server. For example,

	{
		"address": ":80",
		"mappings": {
			"dev": "https://dev-graphite.example.net/",
			"production": "https://graphite.example.net/",
			"staging": "https://stage-graphite.example.net/"
		}
	}
*/
package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/droyo/meta-graphite/query"
)

type backend struct {
	url *url.URL
	*httputil.ReverseProxy
}

// A Config contains the necessary information for running
// a meta-graphite server. Most importantly, it contains the
// mappings of metrics prefixes to backend servers. In the
// config JSON, the value of the "mappings" key must be
// an object of prefix -> URL pairs.
type Config struct {
	// Do not validate HTTPS certs
	InsecureHTTPS bool
	// The address to listen on, if not specified on the command line.
	Address string
	// Maps from metrics prefix to backend URL.
	Mappings map[string]string
	// Dump proxied requests
	Debug bool

	proxy map[string]backend
}

// ParseFile opens the config file at path and calls Parse
// on it.
func ParseFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return Parse(file)
}

// Parse parses the config data from r and
// parses its content into a *Config value.
func Parse(r io.Reader) (*Config, error) {
	cfg := Config{
		Mappings: make(map[string]string),
		proxy:    make(map[string]backend),
	}
	d := json.NewDecoder(r)
	if err := d.Decode(&cfg); err != nil {
		return nil, err
	}
	for k, v := range cfg.Mappings {
		if u, err := url.Parse(v); err != nil {
			return nil, err
		} else {
			b := backend{
				ReverseProxy: httputil.NewSingleHostReverseProxy(u),
				url:          u,
			}
			if cfg.InsecureHTTPS {
				b.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				}
			}
			cfg.proxy[k] = b
		}
	}
	return &cfg, nil
}

// some utility functions
func httperror(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func badrequest(w http.ResponseWriter)  { httperror(w, 400) }
func notfound(w http.ResponseWriter)    { httperror(w, 404) }
func badmethod(w http.ResponseWriter)   { httperror(w, 405) }
func unavailable(w http.ResponseWriter) { httperror(w, 503) }

// ServeHTTP routes a graphite render query to a backend
// graphite server based on its content. If the query contains
// metrics that map one (and only one) of the prefixes in
// a configuration, ServeHTTP will strip the prefix and proxy
// the request to the appropriate backend server.
func (c *Config) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/render" {
		notfound(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Println(err)
		badrequest(w)
		return
	}

	targets := r.Form["target"]
	queries := make([]*query.Query, 0, len(targets))
	for _, target := range targets {
		if q, err := query.Parse(target); err != nil {
			w.WriteHeader(400)
			fmt.Fprintf(w, "Invalid query %q: %v", target, err)
			return
		} else {
			queries = append(queries, q)
		}
	}
	form, server := c.proxyTargets(queries)
	for k, v := range r.Form {
		if k != "target" {
			form[k] = v
		}
	}

	if server.ReverseProxy == nil {
		log.Printf("no backend for %q", queries)
		badrequest(w)
		return
	}

	switch r.Method {
	case "GET":
		r.URL.RawQuery = form.Encode()
		r.Host = server.url.Host
		if c.Debug {
			if dmp, err := httputil.DumpRequest(r, false); err == nil {
				log.Printf("%s", dmp)
			}
		}
	case "POST":
		r.Body = ioutil.NopCloser(
			strings.NewReader(form.Encode()))
	}

	server.ServeHTTP(w, r)
}

func (c *Config) proxyTargets(queries []*query.Query) (url.Values, backend) {
	var server backend
	var targets []string
	for _, q := range queries {
		tgt, srv := c.route(q)
		targets = append(targets, tgt)
		server = srv
	}
	return url.Values{"target": targets}, server
}

func (c *Config) route(q *query.Query) (target string, server backend) {
	for _, m := range q.Metrics() {
		pfx, rest := m.Split()
		if c.Debug {
			log.Printf("%q -> %q, %q", *m, pfx, rest)
		}
		s, ok := c.proxy[string(pfx)]
		if ok {
			server = s
		}
		*m = rest
	}
	return q.String(), server
}
