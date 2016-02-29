package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/droyo/metaphite/internal/mock"
)

func testMux(t *testing.T) *Mux {
	tr := new(http.Transport)
	tr.RegisterProtocol("dev", mock.NewServer())
	tr.RegisterProtocol("stage", mock.NewServer())
	tr.RegisterProtocol("prod", mock.NewServer())
	mux, err := NewMux(tr, map[string]string{
		"dev":   "dev:///",
		"stage": "stage:///",
		"prod":  "prod:///",
	})
	if err != nil {
		t.Fatal(err)
	}
	return mux
}

func testRequest(mux *Mux, query string) *httptest.ResponseRecorder {
	r, err := http.NewRequest("GET", query, nil)
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestProxyOne(t *testing.T) {
	mux := testMux(t)
	rsp := testRequest(mux, "/render?target=stage.entries")
	t.Log(rsp.Body.String())
}
