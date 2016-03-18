package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/droyo/metaphite/internal/mock"
)

func newMux(t *testing.T) *Mux {
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

func testRequest(t *testing.T, query string) {
	mux := newMux(t)
	r, err := http.NewRequest("GET", query, nil)
	if err != nil {
		panic(err)
	}
	rsp := httptest.NewRecorder()
	mux.ServeHTTP(rsp, r)
	if rsp.Code != 200 {
		t.Errorf("request returned %d", rsp.Code)
	}
	t.Logf("%d - %s", rsp.Code, rsp.Body.String())
}

func TestProxyRender(t *testing.T) {
	testRequest(t, "/render?target=stage.entries")
}

func TestProxyMetricFind(t *testing.T) {
	testRequest(t, "/metrics?query=stage.collectd.*")
}

func TestMultiMetric(t *testing.T) {
	testRequest(t, "/metrics?query=*.collectd.*")
}

func TestMultiRender(t *testing.T) {
	testRequest(t, "/render?target=*.entries")
}
