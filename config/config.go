/*
Package config parses config files for metaphite.

metaphite config files are in JSON format, and consists
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
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/droyo/metaphite/certs"
	"github.com/droyo/metaphite/multi"
	"github.com/droyo/metaphite/query"
)

type backend struct {
	url *url.URL
	*httputil.ReverseProxy
}

// A Config contains the necessary information for running
// a metaphite server. Most importantly, it contains the
// mappings of metrics prefixes to backend servers. In the
// config JSON, the value of the "mappings" key must be
// an object of prefix -> URL pairs.
type Config struct {
	// Do not validate HTTPS certs
	InsecureHTTPS bool
	// directory to load CA certs from
	CACertDir string
	// file to load CA certs from
	CACert string
	// The address to listen on, if not specified on the command line.
	Address string
	// Maps from metrics prefix to backend URL.
	Mappings map[string]string
	// Dump proxied requests
	Debug bool

	targets []multi.Target
	client  *http.Client
	*http.ServeMux
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
	var pool certs.Pool
	tlsconfig := new(tls.Config)
	cfg := Config{
		Mappings: make(map[string]string),
	}
	d := json.NewDecoder(r)
	if err := d.Decode(&cfg); err != nil {
		return nil, err
	}
	if cfg.InsecureHTTPS {
		tlsconfig.InsecureSkipVerify = true
	}
	if cfg.CACert != "" {
		pool = certs.Append(pool, certs.FromFile(cfg.CACert))
	}
	if cfg.CACertDir != "" {
		pool = certs.Append(pool, certs.FromDir(cfg.CACertDir))
	}
	if pool != nil {
		tlsconfig.RootCAs = pool.CertPool()
	}
	cfg.client = &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsconfig},
	}
	for k, v := range cfg.Mappings {
		if u, err := url.Parse(v); err != nil {
			return nil, err
		} else {
			cfg.targets = append(cfg.targets, multi.Target{
				Name: k,
				URL:  u,
			})
		}
	}
	cfg.ServeMux = http.NewServeMux()
	cfg.HandleFunc("/metrics/find/", cfg.metricsFind)
	cfg.HandleFunc("/metrics", cfg.metricsFind)
	cfg.HandleFunc("/render", cfg.render)
	return &cfg, nil
}

func (c *Config) matchingTargets(pat query.Metric) []multi.Target {
	result := make([]multi.Target, 0, len(c.targets))

	metrics := pat.Expand()
	for _, t := range c.targets {
		for _, m := range metrics {
			if m.Match(t.Name) {
				result = append(result, t)
				break
			}
		}
	}
	return result
}
