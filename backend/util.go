package backend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func httperror(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func copyReq(r *http.Request) *http.Request {
	cp := new(http.Request)
	form := make(url.Values, len(r.Form))
	for key, val := range r.Form {
		form[key] = val
	}
	*cp = *r
	cp.Form = form
	return cp
}
