package backend

import (
	"encoding/json"
	"io"
	"net/http"
)

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func httperror(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func copyReq(r *http.Request) *http.Request {
	cp := new(http.Request)
	*cp = *r
	return cp
}
