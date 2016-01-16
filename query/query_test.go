package query

import (
	"errors"
	"testing"
)

type test struct {
	in       string
	lexOut   []item
	parseOut Query
}

var ttPositive = []test{
	{
		in:       "myhost.loadavg.05",
		parseOut: Query{Expr: Metric("myhost.loadavg.05")},
		lexOut: []item{
			item{pMETRIC, "myhost.loadavg.05"},
		},
	},
	{
		in: "aliasByNode(myhost.loadavg.05, 1)",
		parseOut: Query{
			Expr: Func{
				Name: "aliasByNode",
				Args: []Expr{
					Metric("myhost.loadavg.05"),
					Value("1"),
				},
			},
		},
		lexOut: []item{
			item{pWORD, "aliasByNode"},
			item{'(', "("},
			item{pMETRIC, "myhost.loadavg.05"},
			item{',', ","},
			item{pNUMBER, "1"},
			item{')', ")"},
		},
	},
	{
		in: `alias(aws-east*.totals.{queues,exchanges,}, "All the \"best\"")`,
		parseOut: Query{
			Expr: Func{
				Name: "alias",
				Args: []Expr{
					Metric("aws-east*.totals.{queues,exchanges,}"),
					Value(`"All the \"best\""`),
				},
			},
		},
		lexOut: []item{
			item{pWORD, "alias"},
			item{'(', "("},
			item{pMETRIC, "aws-east*.totals.{queues,exchanges,}"},
			item{',', ","},
			item{pSTRING, `"All the \"best\""`},
			item{')', ")"},
		},
	},
	{
		in: "averageSeriesWithWildcards(host.cpu-[0-7].cpu-{user,system}.value, 1)",
		parseOut: Query{
			Expr: Func{
				Name: "averageSeriesWithWildcards",
				Args: []Expr{
					Metric("host.cpu-[0-7].cpu-{user,system}.value"),
					Value("1"),
				},
			},
		},
		lexOut: []item{
			item{pWORD, "averageSeriesWithWildcards"},
			item{'(', "("},
			item{pMETRIC, "host.cpu-[0-7].cpu-{user,system}.value"},
			item{',', ","},
			item{pNUMBER, "1"},
			item{')', ")"},
		},
	},
}

func tokenize(s string) ([]item, error) {
	var (
		acc []item
		lex = lex(s)
	)
	for v := range lex.items {
		if v.typ == pERROR {
			return acc, errors.New(v.val)
		}
		acc = append(acc, v)
	}
	return acc, nil
}

func TestLexer(t *testing.T) {
Loop:
	for _, tt := range ttPositive {
		tok, err := tokenize(tt.in)
		if err != nil {
			t.Error(err)
		}
		if len(tok) != len(tt.lexOut) {
			t.Errorf("%s: got \n%v, expected \n%v", tt.in, tok, tt.lexOut)
			continue
		}
		for i := range tok {
			if tok[i] != tt.lexOut[i] {
				t.Errorf("got \n%v, exptected \n%v", tok, tt.lexOut)
				continue Loop
			}
		}
		t.Logf("%s -> \n%v", tt.in, tok)
	}
}

func TestParser(t *testing.T) {
	yyErrorVerbose = true
	//yyDebug = 3
	for _, tt := range ttPositive {
		lex := lex(tt.in)
		result := yyParse(lex)
		if err := lex.Err(); err != nil {
			t.Errorf("%s: %v", tt.in, err)
		} else if result != 0 {
			t.Errorf("parse %q failed but no error", tt.in)
		} else if lex.result.Expr == nil {
			t.Errorf("parse %q nil but no error", tt.in)
		} else if !lex.result.equal(tt.parseOut) {
			t.Errorf("parse %q: got \n%#v, expected \n%#v", tt.in, lex.result, tt.parseOut)
		} else {
			t.Logf("%s -> \n%#v", tt.in, lex.result)
		}
		//println()
	}
}

var ttBrace = [][]Metric{
	[]Metric{
		"servers.{prod,stage}-mysql[1-3].mysql.connections",
		"servers.prod-mysql[1-3].mysql.connections",
		"servers.stage-mysql[1-3].mysql.connections",
	},
	[]Metric{
		"{prod,stage}.ci-server1.loadavg.{01,05}",
		"prod.ci-server1.loadavg.01",
		"prod.ci-server1.loadavg.05",
		"stage.ci-server1.loadavg.01",
		"stage.ci-server1.loadavg.05",
	},
}

func TestBraceExpand(t *testing.T) {
	for _, vv := range ttBrace {
		pat, want, got := vv[0], vv[1:], vv[0].braceExpand(0, nil)
		if len(got) != len(want) {
			t.Errorf("\n%q, got \n%s, expected \n%s", pat, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("\n%q, got \n%s, expected \n%s", pat, got[i], want[i])
			}
		}
		t.Logf("\n%q -> \n%s", pat, got)
	}
}

var ttMatch = []struct {
	pat Metric
	val string
	ok  bool
}{
	{"servers.host*", "servers.host1", true},
	{"servers.host[1-3]", "servers.host2", true},
	{"servers.h*st3", "servers.hoooost3", true},
	{"servers.{h,m,k}ost3", "servers.host3", true},
}

func TestMatch(t *testing.T) {
	for _, tt := range ttMatch {
		if ok := tt.pat.Match(tt.val); ok != tt.ok {
			t.Error("match(%q,%q) = %v, expected %v", tt.pat, tt.val, ok, tt.ok)
		} else {
			t.Logf("match(%q,%q) = %v", tt.pat, tt.val, tt.ok)
		}
	}
}

var ttFlatten = []struct {
	query string
	list  []Metric
}{
	{
		"sumSeries(ganglia{a,}.by-function.server1.*.cpu.load5, ganglia.by-function.server{1,2}.*.cpu.load{62,m})",
		[]Metric{
			"ganglia{a,}.by-function.server1.*.cpu.load5",
			"ganglia.by-function.server{1,2}.*.cpu.load{62,m}",
		},
	},
}

func TestFlatten(t *testing.T) {
	for _, tt := range ttFlatten {
		lex := lex(tt.query)
		result := yyParse(lex)
		if err := lex.Err(); err != nil {
			t.Errorf("%s: %v", tt.query, err)
			continue
		}
		if result != 0 {
			t.Errorf("parse %q failed but no error", tt.query)
			continue
		}
		if lex.result.Expr == nil {
			t.Errorf("parse %q nil but no error", tt.query)
			continue
		}
		mp := lex.result.Metrics()
		m := make([]Metric, 0, len(mp))
		for _, p := range mp {
			m = append(m, *p)
		}
		if len(m) != len(tt.list) {
			t.Errorf("%q\n%#v != \n%#v in \n%#v", tt.query, m, tt.list, lex.result)
			continue
		}
		match := true
		for i, v := range m {
			match = match && v == tt.list[i]
		}
		if !match {
			t.Errorf("%q\n%#v != \n%#v in \n%#v", tt.query, m, tt.list, lex.result)
		} else {
			t.Logf("%q -> \n%q", tt.query, m)
		}
	}
}
