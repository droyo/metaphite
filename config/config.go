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

// MapURL maps a request URL to the appropriate backend
// servers with the prefix pattern stripped. If the URL is not
// a valid graphite render query, an error is returned.
func (c Config) MapURL(req *url.URL) (url.URL, error) {
	if req.Path != "/render" {
		return url.URL{}, errPath
	}

	params := req.Query()
	target := params.Get("target")
	parts := strings.SplitN(target, ".", 2)
	if len(parts) < 2 {
		return url.URL{}, errEmptyTarget
	}

	prefix, target := parts[0], parts[1]
	mapped, ok := c.Mappings[prefix]
	if !ok {
		return url.URL{}, errNotFound
	}

	params.Set("target", target)
	mapped.RawQuery = params.Encode()
	mapped.Path = "render"
	mapped.User = req.User
	mapped.Opaque = req.Opaque

	return mapped, nil
}

func (c *Config) MapRequest(r *http.Request) {
	if mapped, err := c.MapURL(r.URL); err != nil {
		r.URL = &url.URL{Host: r.Host, Path: "/404", Scheme: "http"}
	} else {
		r.URL = &mapped
	}
}
