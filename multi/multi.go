// Package multi proxies one HTTP request to multiple servers
// and merges their responses.
package multi

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"sync"
)

// A Response wraps an http.Response and provides the name
// of the backend from which the response was received to
// merge functions.
type Response struct {
	Target string // the Name of the corresponding Target
	*http.Response
}

// A Target is a tuple of a target URL and a name. The Name
// field of a Target is passed along within a Response, so that
// calling code can know which backend a response is coming
// from.
type Target struct {
	Name string
	URL  *url.URL
}

type Client http.Client

// Proxy proxies r to each URL in targets, and sends the response
// on the returned channel. Once all responses are received, the
// channel is closed. If c is nil, http.DefaultClient is used.
func Proxy(c *http.Client, r *http.Request, tgt []Target) <-chan Response {
	var wg sync.WaitGroup

	if c == nil {
		c = http.DefaultClient
	}
	results := make(chan Response, len(tgt))
	if err := bufferBody(r); err != nil {
		close(results)
		return results
	}

	for _, t := range tgt {
		wg.Add(1)
		go func(t Target) {
			req := t.CopyRequest(r)
			if rsp, err := c.Do(req); err == nil {
				results <- Response{t.Name, rsp}
			}
			wg.Done()
		}(t)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func ProxyRequests(c *http.Client, requests map[string]*http.Request) <-chan Response {
	var wg sync.WaitGroup
	if c == nil {
		c = http.DefaultClient
	}
	results := make(chan Response, len(requests))
	for name, req := range requests {
		wg.Add(1)
		go func(name string, r *http.Request) {
			if rsp, err := c.Do(r); err == nil {
				results <- Response{name, rsp}
			}
			wg.Done()
		}(name, req)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

// Replace the body of a request with an io.ReaderAt, so it can
// be read multiple times.
func bufferBody(r *http.Request) error {
	if _, ok := r.Body.(io.ReaderAt); !ok && r.Body != nil {
		// NOTE(droyo) the net/http package should impose some
		// limits on how much a client can send. however, it
		// may be useful to impose our own, instead of calling
		// ioutil.ReadAll naively.
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body = ioutil.NopCloser(bytes.NewReader(data))
	}
	return nil
}

// CopyRequest makes a copy of an HTTP request, with the URL
// field changed to the Target's URL.
func (tgt *Target) CopyRequest(req *http.Request) *http.Request {
	cp := new(http.Request)
	*cp = *req
	cp.URL.Scheme = tgt.URL.Scheme
	cp.URL.Host = tgt.URL.Host
	cp.URL.Path = path.Join(tgt.URL.Path, req.URL.Path)
	if tgt.URL.RawQuery == "" || req.URL.RawQuery == "" {
		cp.URL.RawQuery = tgt.URL.RawQuery + req.URL.RawQuery
	} else {
		cp.URL.RawQuery = tgt.URL.RawQuery + "&" + req.URL.RawQuery
	}

	if req.Body == nil {
		return cp
	}
	bufferBody(req)
	r := req.Body.(io.ReaderAt)
	cp.Body = ioutil.NopCloser(
		io.NewSectionReader(r, 0, req.ContentLength))

	return cp
}
