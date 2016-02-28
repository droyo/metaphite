// Package backend proxies a graphite API requests to a backend server.
package backend

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/droyo/metaphite/query"
)

// A Mux proxies graphite-web requests to zero or more
// backend servers based on the request content.
type Mux struct {
	client   *http.Client
	servers  []server
	serveMux *http.ServeMux
}

type server struct {
	dest *url.URL
	name string
}

// Result of /metrics/find API
type metricNode struct {
	Leaf int    `json:"is_leaf"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// http response plus metadata
type response struct {
	err    error
	server server
	*http.Response
}

// NewMux creates a new Mux that uses tr to proxy HTTP requests to the
// appropriate backend servers.  If transport is nil, http.DefaultTransport
// is used. The keys of mappings are used as metrics prefixes to match
// metrics and route them to server at the corresponding url value.
// An error is returned if any invalid url or prefix strings are
// provided.
func NewMux(tr http.RoundTripper, mappings map[string]string) (*Mux, error) {
	client := http.DefaultClient
	if tr != nil {
		client := &http.Client{Transport: tr}
	}
	mux := &Mux{
		client:   client,
		servers:  make(map[string]url.URL),
		serveMux: http.NewServeMux(),
	}
	for pfx, urlStr := range mappings {
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
		mux.servers = append(mux.servers, server{dest: u, name: pfx})
	}
	mux.serveMux.HandleFunc("/render/", mux.render)
	mux.serveMux.HandleFunc("/metrics", mux.metrics)
	mux.serveMux.HandleFunc("/metrics/find/", mux.metrics)
	mux.serveMux.HandleFunc("/metrics/expand/", mux.expand)
	return mux, nil
}

// ServeHTTP proxies graphite-web API requests to zero or more backend
// graphite servers based on the metric names in the request. For
// instance, given a request such as
//
// 	GET /render?target=keepLastValue(dev.myhost01.loadavg.05, 100)
//
// ServeHTTP will proxy the request to the server registered under the
// "dev" prefix. When sending the request to the backend, the "dev"
// prefix is stripped, and when sending the response to the client,
// the "dev"  prefix is added. If a request is made that matches
// multiple backends, such as
//
// 	GET /metrics?query=*.servers.mysql*.memory.MemFree
//
// The requests are proxied to each of the matching backends, and
// responses for each server are merged.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.serveMux.ServeHTTP(w, r)
}

// common parts of /metrics/find and /metrics/expand handlers
func (m *Mux) metricsInfo(w http.ResponseWriter, r *http.Request) ([]server, error) {
	var matches []server
	if r.Method != "GET" {
		httperror(w, 405)
		return nil, fmt.Errorf("/metrics/* requests must be GET requests, not %s", r.Method)
	}
	q, err := query.Parse(r.FormValue("query"))
	if err != nil {
		httperror(w, 400)
		return nil, fmt.Errorf("failed to parse query %q: %s", q, err)
	}
	metric, ok := q.Expr.(*query.Metric)
	if !ok {
		httperror(w, 400)
		return nil, fmt.Errorf("query parameter must be Metric, not %T", q.Expr)
	}
	pfx, rest := metric.Split()
	r.Form.Set("query", rest.String())
	r.URL.RawQuery = r.Form.Encode()
	for _, srv := range m.servers {
		if pfx.Match(srv.name) {
			matches = append(matches, srv)
		}
	}
	if len(matches) == 0 {
		httperror(w, 404)
		return nil, fmt.Errorf("no matches for query %q", q)
	}
	return matches, nil
}

// proxy r to the list of servers and send results to a channel
func (m *Mux) proxyMetrics(servers []server, r *http.Request) <-chan response {
	var wg sync.WaitGroup

	// Making the response channel large enough to hold all responses
	// avoids leaking goroutines should consumers return before
	// exhausting the channel.
	responses := make(chan response, len(servers))
	for _, srv := range servers {
		wg.Add(1)
		go func(r *http.Request, srv server) {
			rsp, err := m.proxy(srv, r)
			responses <- response{err: err, server: server, Response: rsp}
			wg.Done()
		}(r, srv)
	}
	go func() {
		wg.Wait()
		close(responses)
	}()
	return responses
}

// GET /metrics/find?query=foo.*
// http://graphite-api.readthedocs.org/en/latest/api.html#metrics-find
func (m *Mux) metrics(w http.ResponseWriter, r *http.Request) {
	servers, err := m.metricsInfo(w, r)
	if err != nil {
		log.Printf("/metrics/find request failed: %s", err)
		return
	}
	var result []metricNode
	var rsp response
	for rsp = range m.proxyMetrics(servers, r) {
		if rsp.err != nil {
			log.Printf("error contacting %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		if rsp.StatusCode != 200 {
			continue
		}
		if err := decodeJSON(rsp.Body, &result); err != nil {
			log.Printf("error reading response from %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		for i, v := range result {
			result[i].Path = rsp.server.name + "." + v.Path
		}
	}
	if len(result) > 0 {
		json.NewEncoder(w).Encode(result)
	} else if rsp.Response != nil {
		rsp.Write(w)
	} else {
		httperror(w, 503)
	}
}

// GET /metrics/expand?query=foo.*
// http://graphite-api.readthedocs.org/en/latest/api.html#metrics-expand
func (m *Mux) expand(w http.ResponseWriter, r *http.Request) {
	var result []string
	servers, err := m.metricsInfo(w, r)
	if err != nil {
		log.Printf("/metrics/expand request failed: %s", err)
		return
	}
	if err := m.proxyMetrics(w, r, servers, &result); err != nil {
		log.Print(err)
		return
	}
	var result []string
	var rsp response
	for rsp = range m.proxyMetrics(servers, r) {
		if rsp.err != nil {
			log.Printf("error contacting %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		if rsp.StatusCode != 200 {
			continue
		}
		if err := decodeJSON(rsp.Body, &result); err != nil {
			log.Printf("error reading response from %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		for i, v := range result {
			result[i] = rsp.server.name + "." + v
		}
	}
	if len(result) > 0 {
		json.NewEncoder(w).Encode(result)
	} else if rsp.Response != nil {
		rsp.Write(w)
	} else {
		httperror(w, 503)
	}
}
