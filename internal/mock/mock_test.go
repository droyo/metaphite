package mock

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMock(t *testing.T) {
	server := NewServer()
	tr := new(http.Transport)
	tr.RegisterProtocol("foo", server)
	client := &http.Client{
		Transport: tr,
	}
	testRequest(t, client, "foo:///metrics/find?query=servers.*")
	testRequest(t, client, "foo:///metrics/expand?query=collectd.*")
	testRequest(t, client, "foo:///render?target=entries")
}

func testRequest(t *testing.T, client *http.Client, urlStr string) {
	rsp, err := client.Get(urlStr)
	if err != nil {
		t.Error(err)
		return
	}
	var result interface{}
	if err := json.NewDecoder(rsp.Body).Decode(&result); err != nil {
		t.Error(err)
	} else {
		t.Logf("read %v", result)
	}
}
