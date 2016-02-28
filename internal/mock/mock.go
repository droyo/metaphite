// Package mock implements a mock graphite-web server
// that replies with canned responses.
package mock

import (
	"bufio"
	"io"
	"net/http"
)

type Server struct {
	data map[string]string
	ln   *pipeListener
	*http.ServeMux
}

var testData = map[string]string{
	"metrics": `{"metrics": [{
    "is_leaf": 0,
    "name": "db01",
    "path": "collectd.db01."
}, {
    "is_leaf": 1,
    "name": "foo",
    "path": "collectd.foo"
}]}`,
	"expand": `["collectd.db01", "collectd.foo"]`,
	"render": `[{
  "target": "entries",
  "datapoints": [
    [1.0, 1311836008],
    [2.0, 1311836009],
    [3.0, 1311836010],
    [5.0, 1311836011],
    [6.0, 1311836012]
  ]
}]`,
}

func NewServer() *Server {
	srv := &Server{
		data: testData,
		ln:   listener(),
	}
	srv.ServeMux = http.NewServeMux()
	srv.HandleFunc("/metrics/find", srv.metricsFind)
	srv.HandleFunc("/metrics/expand", srv.metricsExpand)
	srv.HandleFunc("/metrics", srv.metricsFind)
	srv.HandleFunc("/render", srv.render)
	httpsrv := &http.Server{Handler: srv}
	go httpsrv.Serve(srv.ln)
	return srv
}

// RoundTrip sends all http requests to the Server, ignoring port and
// host sections of the request URL.
func (srv *Server) RoundTrip(req *http.Request) (*http.Response, error) {
	c, err := srv.ln.Dial()
	if err != nil {
		return nil, err
	}
	if err := req.Write(c); err != nil {
		return nil, err
	}
	br := bufio.NewReader(c)
	return http.ReadResponse(br, req)
}

func (srv *Server) metricsFind(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, srv.data["metrics"])
}

func (srv *Server) metricsExpand(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, srv.data["expand"])
}

func (srv *Server) render(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, srv.data["render"])
}
