package config

import (
	"io"
	"log"
	"net/http"

	"github.com/droyo/metaphite/multi"
)

/*
	/metrics/find
Finds metrics under a given path. Other alias: /metrics.

Example:
	GET /metrics/find?query=collectd.*

	{"metrics": [{
	    "is_leaf": 0,
	    "name": "db01",
	    "path": "collectd.db01."
	}, {
	    "is_leaf": 1,
	    "name": "foo",
	    "path": "collectd.foo"
	}]}

Parameters:

	query (mandatory)
The query to search for.

	format
The output format to use. Can be completer (default) or treejson.

	wildcards (0 or 1)
Whether to add a wildcard result at the end or no. Default: 0.

	from
Epoch timestamp from which to consider metrics.

	until
Epoch timestamp until which to consider metrics.
*/
type metricNode struct {
	Leaf int    `json:"is_leaf"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (c *Config) metricsFind(w http.ResponseWriter, r *http.Request) {
	const emptyResponse = `[]`

	pat, err := stripPrefix(r, "query")
	if err != nil {
		io.WriteString(w, emptyResponse)
		return
	}
	targets := c.matchingTargets(pat)
	if len(targets) == 0 {
		io.WriteString(w, emptyResponse)
		return
	}

	var current, result struct {
		Metrics []metricNode `json:"metrics"`
	}

	if len(r.FormValue("query")) == 0 {
		for _, tgt := range targets {
			result.Metrics = append(result.Metrics, metricNode{
				Leaf: 0,
				Name: tgt.Name,
				Path: tgt.Name + ".",
			})
		}
		writeJSON(w, result)
		return
	}

	for rsp := range multi.Proxy(c.client, r, targets) {
		if err := readJSON(rsp.Body, &current); err != nil {
			log.Printf("received invalid %s response from %s: %s",
				r.URL.Path, rsp.Target, err)
		}
		for _, v := range current.Metrics {
			v.Path = rsp.Target + "." + v.Path
			result.Metrics = append(result.Metrics, v)
		}
		current.Metrics = current.Metrics[:0]
	}
	writeJSON(w, result)
}
