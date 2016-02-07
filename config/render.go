package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/droyo/metaphite/multi"
	"github.com/droyo/metaphite/query"
)

// render routes a graphite render query to a backend graphite server
// based on its content. Each target expression must contain metrics
// that map one (and only one) of the prefixes in a configuration.
// render will strip the prefix and proxy the request to the appropriate
// backend server. If multiple target expressions are provided, requests
// are made to each of the matching servers in parallel and merged
// before being returned to the client.
func (c *Config) render(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Println(err)
		badrequest(w)
		return
	}

	searches := r.Form["target"]
	queries := make([]*query.Query, 0, len(searches))
	for _, s := range searches {
		if q, err := query.Parse(s); err != nil {
			w.WriteHeader(400)
			fmt.Fprintf(w, "Invalid query %q: %v", s, err)
			return
		} else {
			queries = append(queries, q)
		}
	}

	requests := make(map[string]*http.Request, len(c.targets))
	for _, q := range queries {
		var prefix query.Metric
		for _, m := range q.Metrics() {
			pfx, path := m.Split()
			if prefix != pfx && prefix != "" {
				log.Printf("multiple backends in single query: %q", q)
				break
			}
			prefix = pfx
			*m = path
		}
		for _, tgt := range c.matchingTargets(prefix) {
			req, ok := requests[tgt.Name]
			if !ok {
				req = tgt.CopyRequest(r)
				req.Form["target"] = nil
			}
			req.Form["target"] = append(req.Form["target"], q.String())
		}
	}

	type renderLine struct {
		Datapoints json.RawMessage `json:"datapoints"`
		Target     string          `json:"target"`
	}
	var results, chunk []renderLine
	for rsp := range multi.ProxyRequests(c.client, requests) {
		// breaking out of this loop would leak goroutines
		chunk = chunk[:0]
		if err := readJSON(rsp.Body, &chunk); err != nil {
			log.Println("error reading response from %q: %s",
				rsp.Target, err)
		}
		results = append(results, chunk...)
	}
	writeJSON(w, results)
}
