package config

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/droyo/metaphite/query"
)

func httperror(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func badrequest(w http.ResponseWriter)  { httperror(w, 400) }
func notfound(w http.ResponseWriter)    { httperror(w, 404) }
func badmethod(w http.ResponseWriter)   { httperror(w, 405) }
func unavailable(w http.ResponseWriter) { httperror(w, 503) }

func writeJSON(w io.Writer, val interface{}) {
	e := json.NewEncoder(w)
	e.Encode(val)
}

func readJSON(r io.Reader, val interface{}) error {
	d := json.NewDecoder(r)
	return d.Decode(val)
}

func requestQuery(r *http.Request, param string) (*query.Query, error) {
	return query.Parse(r.FormValue(param))
}

func requestMetric(r *http.Request, param string) (*query.Metric, error) {
	q, err := requestQuery(r, param)
	if err != nil {
		return nil, err
	}
	if m, ok := q.Expr.(*query.Metric); ok {
		return m, nil
	}
	return nil, errors.New("must be a metric name")
}

var errMultipleTargets = errors.New("multiple prefixes in single query")

// stripPrefix strips the prefix from the specified form parameter in
// an http.Request. If a single query expression references multiple
// targets, an error is returned. A GET request is assumed.
func stripPrefix(r *http.Request, param string) (query.Metric, error) {
	q, err := query.Parse(r.FormValue(param))
	if err != nil {
		return "", err
	}
	var prefix query.Metric
	for _, m := range q.Metrics() {
		pfx, path := m.Split()
		if pfx != prefix && prefix != "" {
			return "", errMultipleTargets
		}
		*m = path
		prefix = pfx
	}
	v := r.URL.Query()
	v.Set(param, q.String())
	r.URL.RawQuery = v.Encode()
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return prefix, nil
}
