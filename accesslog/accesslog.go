// Package accesslog logs HTTP requests.
package accesslog

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// Handler wraps an existing http.Handler and logs any requests
// routed along to the handler, in the following format:
//
// 	127.0.0.1 user-identifier frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
//
// Output is logged to the dest parameter. If dest is nil, the default
// logger of the log package is used.
func Handler(existing http.Handler, dest Logger) http.Handler {
	return handler{handler: existing, dest: dest}
}

// Types implementing the Logger interface can be used as destinations
// for access log messages. The Printf method must be safe for concurrent
// use among multiple goroutines.
type Logger interface {
	Printf(format string, v ...interface{})
}

type handler struct {
	handler http.Handler
	dest    Logger
}

func (h handler) logf(format string, v ...interface{}) {
	if h.dest != nil {
		h.dest.Printf(format, v...)
	} else {
		log.Printf(format, v...)
	}
}

type responseWriter struct {
	http.ResponseWriter
	status, n int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.n += n
	return n, err
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// From https://en.wikipedia.org/wiki/Common_Log_Format
	//
	// 127.0.0.1 user-identifier frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
	const format = "%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\""
	const layout = "2/Jan/2006:15:04:05 -0700"

	uri := r.URL.RequestURI()
	userAgent := "-"
	if agent := r.UserAgent(); agent != "" {
		userAgent = agent
	}
	referer := "-"
	if ref := r.Referer(); ref != "" {
		referer = ref
	}

	shim := responseWriter{ResponseWriter: w}

	//start := time.Now()
	h.handler.ServeHTTP(&shim, r)
	end := time.Now()

	h.logf(format,
		strings.Split(r.RemoteAddr, ":")[0],
		end.Format(layout),
		r.Method,
		uri,
		r.Proto,
		shim.status,
		shim.n,
		referer,
		userAgent)
}
