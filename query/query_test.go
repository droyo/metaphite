package query

import (
	"errors"
	"strings"
	"testing"
)

type test struct {
	in       string
	lexOut   []token
	parseOut Query
}

type token struct {
	t int
	v string
}

var ttPositive = []test{
	{
		in:       "myhost.loadavg.05",
		parseOut: Query{Expr: Metric("myhost.loadavg.05")},
		lexOut: []token{
			token{WORD, "myhost"},
			token{'.', ""},
			token{WORD, "loadavg"},
			token{'.', ""},
			token{WORD, "05"},
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
		lexOut: []token{
			token{WORD, "aliasByNode"},
			token{'(', ""},
			token{WORD, "myhost"},
			token{'.', ""},
			token{WORD, "loadavg"},
			token{'.', ""},
			token{WORD, "05"},
			token{',', ""},
			token{WORD, "1"},
			token{')', ""},
		},
	},
	{
		in: `alias(aws-east*.totals.{queues,exchanges,}, "All the \"best\"")`,
		parseOut: Query{
			Expr: Func{
				Name: "alias",
				Args: []Expr{
					Metric("aws-east*.totals.{queues,exchanges,}"),
					Value("All the \\\"best\\\""),
				},
			},
		},
		lexOut: []token{
			token{WORD, "alias"},
			token{'(', ""},
			token{WORD, "aws-east*"},
			token{'.', ""},
			token{WORD, "totals"},
			token{'.', ""},
			token{'{', ""},
			token{WORD, "queues"},
			token{',', ""},
			token{WORD, "exchanges"},
			token{',', ""},
			token{'}', ""},
			token{',', ""},
			token{STRING, `All the \"best\"`},
			token{')', ""},
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
		lexOut: []token{
			token{WORD, "averageSeriesWithWildcards"},
			token{'(', ""},
			token{WORD, "host"},
			token{'.', ""},
			token{WORD, "cpu-[0-7]"},
			token{'.', ""},
			token{WORD, "cpu-"},
			token{'{', ""},
			token{WORD, "user"},
			token{',', ""},
			token{WORD, "system"},
			token{'}', ""},
			token{'.', ""},
			token{WORD, "value"},
			token{',', ""},
			token{WORD, "1"},
			token{')', ""},
		},
	},
}

func tokenize(s string) ([]token, error) {
	var (
		acc  []token
		lex  = lexer{target: s}
		lval = new(yySymType)
	)
	for {
		t := lex.Lex(lval)
		v := lval.str

		if t == 0 {
			break
		}
		if t < 0 {
			return acc, errors.New(strings.Join(lex.err, "\n"))
		}
		acc = append(acc, token{t: t, v: string(v)})
	}
	return acc, nil
}

func TestLexer(t *testing.T) {
	for _, tt := range ttPositive {
		tok, err := tokenize(tt.in)
		if err != nil {
			t.Error(err)
		}
		if len(tok) != len(tt.lexOut) {
			t.Errorf("%s: got \n%v, expected \n%v", tt.in, tok, tt.lexOut)
		}
		for i := range tok {
			if tok[i] != tt.lexOut[i] {
				t.Errorf("got \n%v, exptected \n%v", tok, tt.lexOut)
				return
			}
		}
		t.Logf("%s -> %v", tt.in, tok)
	}
}

func TestParser(t *testing.T) {
	yyErrorVerbose = true
	//yyDebug = 3
	for _, tt := range ttPositive {
		lex := lexer{target: tt.in}
		result := yyParse(&lex)
		if err := lex.Err(); err != nil {
			t.Errorf("%s: %v", tt.in, err)
		} else if result != 0 {
			t.Errorf("parse %q failed but no error", tt.in)
		} else if !lex.result.equal(tt.parseOut) {
			t.Errorf("parse %q: got \n%#v, expected \n%#v", tt.in, lex.result, tt.parseOut)
		} else {
			t.Logf("%s -> \n%#v", tt.in, lex.result)
		}
		//println()
	}
}
