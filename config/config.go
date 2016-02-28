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
	"os"

	"github.com/droyo/metaphite/backend"
	"github.com/droyo/metaphite/certs"
)

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

	mux *backend.Mux
}

// ParseFile opens the config file at path and calls Parse on it.
func ParseFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return Parse(file)
}

// Parse parses the config data from r and parses its content into a
// *Config value.
func Parse(r io.Reader) (*Config, error) {
	var pool certs.Pool
	tlsconfig := new(tls.Config)
	cfg := Config{
		Mappings: make(map[string]string),
		proxy:    make(map[string]backend),
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
	tr := &http.Transport{TLSClientConfig: tlsconfig}
	if servers, err := backend.NewMux(tr, cfg.Mappings); err != nil {
		return nil, err
	} else {
		cfg.mux = mux
	}
	return &cfg, nil
}

// ServeHTTP routes graphite queries to zero or more backend graphite
// servers based on their content.
func (c *Config) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}
