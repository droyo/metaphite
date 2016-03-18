// Package backend proxies a graphite API requests to a backend server.
package backend

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/droyo/metaphite/query"
)

// A Mux proxies graphite-web requests to zero or more
// backend servers based on the request content.
type Mux struct {
	client   *http.Client
	servers  map[string]server
	serveMux *http.ServeMux
}

type server struct {
	dest  *url.URL
	name  string
	proxy func(*http.Request) (*http.Response, error)
}

// Result of /metrics/find API
type metricNode struct {
	Leaf int    `json:"is_leaf"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Result of /render API
type renderTarget struct {
	Target     string          `json:"target"`
	Datapoints json.RawMessage `json:"datapoints"`
}

// http response plus metadata
type response struct {
	err    error
	server server
	*http.Response
}

func stripPrefix(q *query.Query) {
	for _, metric := range q.Metrics() {
		_, *metric = metric.Split()
	}
}

// NewMux creates a new Mux that uses tr to proxy HTTP requests to the
// appropriate backend servers.  If transport is nil, http.DefaultTransport
// is used. The keys of mappings are used as metrics prefixes to match
// metrics and route them to server at the corresponding url value.
// An error is returned if any invalid url or prefix strings are
// provided.
func NewMux(tr http.RoundTripper, mappings map[string]string) (*Mux, error) {
	mux := &Mux{
		serveMux: http.NewServeMux(),
	}
	servers := make(map[string]server, len(mappings))
	for pfx, urlStr := range mappings {
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}

		srv := server{dest: u, name: pfx}
		rev := httputil.NewSingleHostReverseProxy(u)
		srv.proxy = func(r *http.Request) (*http.Response, error) {
			r = copyReq(r)
			rev.Director(r)
			return tr.RoundTrip(r)
		}
		servers[srv.name] = srv
	}
	mux.servers = servers
	mux.serveMux.HandleFunc("/render", mux.render)
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

// matching returns the servers that match a query
func (m *Mux) matchingServers(q *query.Query) []server {
	result := make([]server, 0, len(m.servers))
	for _, srv := range m.servers {
		for _, metric := range q.Metrics() {
			pfx, _ := metric.Split()
			if pfx.Match(srv.name) {
				result = append(result, srv)
				break
			}
		}
	}
	return result
}

// common parts of /metrics/find and /metrics/expand handlers. second
// return value is true if the metrics pattern has nothing after the prefix.
func (m *Mux) metricsInfo(w http.ResponseWriter, r *http.Request) ([]server, bool, error) {
	if r.Method != "GET" {
		httperror(w, 405)
		return nil, false, fmt.Errorf("/metrics/* requests must be GET requests, not %s",
			r.Method)
	}
	q, err := query.Parse(r.FormValue("query"))
	if err != nil {
		httperror(w, 400)
		return nil, false, fmt.Errorf("failed to parse query %q: %s", q, err)
	}
	metric, ok := q.Expr.(*query.Metric)
	if !ok {
		httperror(w, 400)
		return nil, false, fmt.Errorf("query parameter must be Metric, not %T", q.Expr)
	}
	_, rest := metric.Split()
	r.Form.Set("query", rest.String())
	r.URL.RawQuery = r.Form.Encode()

	matches := m.matchingServers(q)
	if len(matches) == 0 {
		httperror(w, 404)
		return nil, false, fmt.Errorf("no matches for query %q", q)
	}
	return matches, len(rest) == 0, nil
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
			rsp, err := srv.proxy(r)
			responses <- response{err: err, server: srv, Response: rsp}
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
	servers, toplevel, err := m.metricsInfo(w, r)
	if err != nil {
		log.Printf("/metrics/find request failed: %s", err)
		return
	}
	var result, chunk []metricNode
	if toplevel {
		for _, srv := range servers {
			result = append(result, metricNode{
				Leaf: 0,
				Name: srv.name,
				Path: srv.name + ".",
			})
		}
		json.NewEncoder(w).Encode(result)
		return
	}
	var rsp response
	for rsp = range m.proxyMetrics(servers, r) {
		if rsp.err != nil {
			log.Printf("error contacting %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		if rsp.StatusCode != 200 {
			continue
		}
		if err := decodeJSON(rsp.Body, &chunk); err != nil {
			log.Printf("error reading response from %s: %s", rsp.server.dest, err)
			continue
		}
		for i, v := range chunk {
			chunk[i].Path = rsp.server.name + "." + v.Path
		}
		result = append(result, chunk...)
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
	servers, toplevel, err := m.metricsInfo(w, r)
	if err != nil {
		log.Printf("/metrics/expand request failed: %s", err)
		return
	}
	var result, chunk []string
	if toplevel {
		for _, srv := range servers {
			result = append(result, srv.name)
		}
		json.NewEncoder(w).Encode(result)
		return
	}
	var rsp response
	for rsp = range m.proxyMetrics(servers, r) {
		if rsp.err != nil {
			log.Printf("error contacting %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		if rsp.StatusCode != 200 {
			continue
		}
		if err := decodeJSON(rsp.Body, &chunk); err != nil {
			log.Printf("error reading response from %s: %s", rsp.server.dest, err)
			continue
		}
		for i, v := range chunk {
			chunk[i] = rsp.server.name + "." + v
		}
		result = append(result, chunk...)
	}

	if len(result) > 0 {
		json.NewEncoder(w).Encode(result)
	} else if rsp.Response != nil {
		rsp.Write(w)
	} else {
		httperror(w, 503)
	}
}

func parseQueries(expr []string) ([]*query.Query, error) {
	queries := make([]*query.Query, 0, len(expr))
	for _, s := range expr {
		if q, err := query.Parse(s); err != nil {
			return nil, err
		} else {
			queries = append(queries, q)
		}
	}
	return queries, nil
}

func (m *Mux) proxyRender(requests map[string]*http.Request) <-chan response {
	var wg sync.WaitGroup

	responses := make(chan response, len(requests))
	for name, r := range requests {
		wg.Add(1)
		srv := m.servers[name]
		go func(r *http.Request, srv server) {
			rsp, err := srv.proxy(r)
			log.Printf("proxied %s", srv.name)
			responses <- response{err: err, server: srv, Response: rsp}
			wg.Done()
		}(r, srv)
	}
	go func() {
		wg.Wait()
		close(responses)
	}()
	return responses
}

func (m *Mux) render(w http.ResponseWriter, r *http.Request) {
	var result, chunk []renderTarget
	if r.Method != "GET" && r.Method != "POST" {
		httperror(w, 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		httperror(w, 400)
		return
	}
	queries, err := parseQueries(r.Form["target"])
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	buckets := make(map[string][]*query.Query, len(queries))
	for _, q := range queries {
		for _, srv := range m.matchingServers(q) {
			buckets[srv.name] = append(buckets[srv.name], q)
		}
		stripPrefix(q)
	}
	requests := make(map[string]*http.Request, len(buckets))
	for srv, queries := range buckets {
		req := copyReq(r)
		req.Form.Del("target")
		for _, q := range queries {
			req.Form.Add("target", q.String())
		}
		if req.Method == "POST" {
			s := req.Form.Encode()
			req.ContentLength = int64(len(s))
			req.Body = ioutil.NopCloser(
				strings.NewReader(s))
		} else {
			req.URL.RawQuery = req.Form.Encode()
		}
		requests[srv] = req
	}
	var rsp response
	for rsp = range m.proxyRender(requests) {
		if rsp.err != nil {
			log.Print("error contacting %s: %s", rsp.server.dest, rsp.err)
			continue
		}
		if rsp.StatusCode != 200 {
			continue
		}
		if err := decodeJSON(rsp.Body, &chunk); err != nil {
			log.Printf("error reading response from %s: %s", rsp.server.dest, err)
			continue
		}
		for i, v := range chunk {
			chunk[i].Target = rsp.server.name + "." + v.Target
		}
		result = append(result, chunk...)
	}
	if len(result) > 0 {
		json.NewEncoder(w).Encode(result)
	} else if rsp.Response != nil {
		rsp.Write(w)
	} else {
		httperror(w, 503)
	}
}
