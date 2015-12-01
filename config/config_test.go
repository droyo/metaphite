package config

import (
	"net/url"
	"strings"
	"testing"
)

var input = `{
	"mappings": {
		"west": "https://graphite-west.example.net/",
		"east": "https://graphite-east.example.net/",
		"stage": "https://graphite-stage.example.net/"
	}
}`

var tt = []struct {
	from, to string
}{
	{
		"https://a/render?target=west.servers.host1.loadavg.05",
		"https://graphite-west.example.net/render?target=servers.host1.loadavg.05",
	},
	{
		"https://a/render?target=east.servers.host1.loadavg.05",
		"https://graphite-east.example.net/render?target=servers.host1.loadavg.05",
	},
	{
		"https://a/render?target=stage.servers.host1.loadavg.05",
		"https://graphite-stage.example.net/render?target=servers.host1.loadavg.05",
	},
}

func parseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestParse(t *testing.T) {
	if _, err := Parse(strings.NewReader(input)); err != nil {
		t.Error(err)
	}
}

func TestMapURL(t *testing.T) {
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tt {
		in := parseURL(test.from)
		out, err := cfg.MapURL(in)
		if err != nil {
			t.Errorf("map %s failed: %v", in, err)
			continue
		}
		if out.String() != test.to {
			t.Errorf("%s → %s, expected %s", in, out.String(), test.to)
		}
		t.Logf("%s → %s", in, out.String())
	}
}
