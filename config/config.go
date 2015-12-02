/*
Package config parses config files for meta-graphite.

meta-graphite config files are in JSON format, and consists
of a single JSON object containing string/string pairs. The
key should be a metrics prefix to match, and the value should
be a URL for the graphite server. For example,

	{
		"dev": "https://dev-graphite.example.net/",
		"production": "https://graphite.example.net/",
		"staging": "https://stage-graphite.example.net/"
	}
*/
package config

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	errPath        = errors.New("url path is not /render")
	errEmptyTarget = errors.New("empty target query parameter")
	errNotFound    = errors.New("prefix not found in config")
)

// A Config contains the necessary information for running
// a meta-graphite server. Most importantly, it contains the
// mappings of metrics prefixes to backend servers. In the
// config JSON, the value of the "mappings" key must be
// an object of prefix -> URL pairs.
type Config struct {
	// The address to listen on, if not specified on the command line.
	Address string
	// Maps from metrics prefix to backend URL.
	Mappings mapping
}

type mapping map[string]url.URL

func (c *mapping) UnmarshalJSON(data []byte) error {
	tmp := make(map[string]string)
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	for k, v := range tmp {
		if u, err := url.Parse(v); err != nil {
			return err
		} else {
			(*c)[k] = *u
		}
	}
	return nil
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
	cfg := Config{Mappings: make(mapping)}
	d := json.NewDecoder(r)
	if err := d.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func badRequest(r *http.Request) {
	r.URL.Path = "/notfound"
}

// MapRequest maps a request to the appropriate backend
// servers with the prefix pattern stripped. If the URL is not
// a valid graphite render query, an error is returned. Both
// GET and POST requests are supported.
func (c *Config) MapRequest(r *http.Request) {
	if r.URL.Path != "/render" {
		badRequest(r)
		return
	}

	target := r.FormValue("target")
	parts := strings.SplitN(target, ".", 2)

	if len(parts) < 2 {
		badRequest(r)
		return
	}

	prefix, rest := parts[0], parts[1]
	mapped, ok := c.Mappings[prefix]
	if !ok {
		badRequest(r)
		return
	}

	r.Form.Set("target", rest)
	mapped.User = r.URL.User
	mapped.Opaque = r.URL.Opaque
	mapped.Path = "/render"
	*r.URL = mapped
	r.Host = r.URL.Host

	if r.Method == "GET" {
		r.URL.RawQuery = r.Form.Encode()
	} else if r.Method == "POST" {
		params := r.Form.Encode()
		r.Header.Del("Content-Length")
		r.ContentLength = int64(len(params))
		r.Body = ioutil.NopCloser(strings.NewReader(params))
	}
}
