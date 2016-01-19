// Package query parses Graphite queries into an
// abstract syntax tree.
package query

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

//go:generate -command yacc go tool yacc
//go:generate yacc -o expr.go expr.y

//&target=averageSeries(company.server*.applicationInstance.requestsHandled)

// Parse parses a graphite query. The various expressions
// in a query can be accessed and modified through the methods
// on the returned Query value.
func Parse(query string) (*Query, error) {
	l := lex(query)
	defer l.drain()

	result := yyParse(l)
	if err := l.Err(); err != nil {
		return nil, err
	}

	if result != 0 {
		return nil, errors.New("parse error")
	}

	return l.result, nil
}

// String produces the string representation of a (possibly modified)
// query. The return value is not url-encoded.
func (q *Query) String() string {
	var buf bytes.Buffer
	marshalExpr(&buf, q, 0)
	return buf.String()
}

func marshalExpr(w io.Writer, e Expr, depth int) {
	const maxDepth = 200
	if depth > maxDepth {
		return
	}

	switch e := e.(type) {
	case *Query:
		marshalExpr(w, e.Expr, depth+1)
	case *Func:
		fmt.Fprint(w, e.Name, "(")
		for i, v := range e.Args {
			expr := v
			marshalExpr(w, expr, depth+1)
			if i < len(e.Args)-1 {
				fmt.Fprint(w, ", ")
			}
		}
		fmt.Fprint(w, ")")
	case *Value:
		fmt.Fprint(w, *e)
	case *Metric:
		fmt.Fprint(w, *e)
	}
}

// Metrics produces a list of metrics referenced in a query.
// The pointers returned by Metrics may be modified to
// change an expression.

// A Query is a parsed graphite target query, and may consist
// of a single metric name (or glob), or a function call.
type Query struct {
	Expr
}

func (x *Query) equal(y Expr) bool {
	if yq, ok := y.(*Query); ok && yq != nil {
		return x.Expr.equal(yq.Expr)
	}
	return false
}

// walk calls fn on each expression in a Query in
// depth-first order
func (q *Query) walk(fn func(Expr)) {
	walk(q.Expr, fn, 0)
}

func walk(e Expr, fn func(Expr), depth int) {
	const maxDepth = 200
	if depth > maxDepth {
		return
	}
	switch v := e.(type) {
	case *Func:
		fn(v)
		for _, vv := range v.Args {
			walk(vv, fn, depth+1)
		}
	case *Query:
		walk(v.Expr, fn, depth+1)
	case *Value:
		fn(v)
	case *Metric:
		fn(v)
	}
}

// Metrics returns a slice of pointers to all metric names
// referenced in a query. The Metrics may be mutated
// through the pointer values to affect the output of the
// Query's String method.
func (q *Query) Metrics() []*Metric {
	var result []*Metric
	q.walk(func(expr Expr) {
		if m, ok := expr.(*Metric); ok {
			result = append(result, m)
		}
	})
	return result
}

// An Expr represents a graphite query subexpression.
type Expr interface {
	equal(e Expr) bool
}

// A Func represents a function call.
type Func struct {
	Name string // The name of the function
	Args []Expr // zero or more arguments
}

func (f *Func) String() string { return f.Name }
func (xfn *Func) equal(y Expr) bool {
	yfn, ok := y.(*Func)
	if !ok || yfn == nil {
		return false
	}
	if xfn.Name != yfn.Name {
		return false
	}
	if len(xfn.Args) != len(yfn.Args) {
		return false
	}
	for i, v := range xfn.Args {
		if !v.equal(yfn.Args[i]) {
			return false
		}
	}
	return true
}

// A Metric is the name of a graphite metric, a list of words separated
// by dots. If a Metric contains a glob pattern, it can be expanded
// to multiple metrics using the Expand method.
type Metric string

func (x *Metric) equal(y Expr) bool {
	if m, ok := y.(*Metric); ok && m != nil {
		return *x == *m
	}
	return false
}

func (m *Metric) String() string { return string(*m) }

// Split splits m immediately following the first dot
func (m Metric) Split() (first, rest Metric) {
	first = m
	dot := strings.Index(string(m), ".")
	if dot >= 0 {
		first = m[:dot]
		rest = m[dot+1:]
	}
	return first, rest
}

// If a Metric contains any brace expansions,
// Expand expands them and returns a slice
// of Metrics for each expansion. Otherwise,
// Expand returns a single-element slice containing
// the original Metric.
func (m Metric) Expand() []Metric {
	return m.braceExpand(0, nil)
}

// Match returns true if the metric is equal to or matches name
func (m Metric) Match(name string) bool {
	for _, pat := range m.Expand() {
		if pat.match(name) {
			return true
		}
	}
	return false
}

// match returns true if the Metric pat matches s.
func (pat Metric) match(s string) bool {
	ok, err := path.Match(string(pat), s)
	if err != nil {
		return false
	}
	return ok
}

// braceExpand expands all brace-delimited lists in a Metric
// and produces a list of simple Metrics.
func (m Metric) braceExpand(depth int, addto []Metric) []Metric {
	const maxPatterns = 100
	var (
		escape, inbrace bool
		start           int
		prefix, suffix  Metric
		segments        []Metric
	)
	if len(m) == 0 {
		return addto
	}
	if len(addto) == 0 {
		addto = append(addto, "")
	}
	if depth > maxPatterns {
		goto done
	}

Loop:
	for i, v := range m {
		if escape {
			escape = false
			continue
		}
		switch v {
		case '\\':
			escape = true
		case '{':
			if inbrace {
				// foo.{{bar,baz} is invalid
				return nil
			}
			inbrace = true
			prefix = m[:i]
			start = i + 1
		case ',':
			if !inbrace {
				break
			}
			segments = append(segments, prefix+m[start:i])
			start = i + 1
		case '}':
			inbrace = false
			segments = append(segments, prefix+m[start:i])
			suffix = m[i+1:]
			break Loop
		}
	}
done:
	if inbrace {
		// unterminated brace expansion.
		return nil
	}
	if len(segments) == 0 {
		// no brace expansion in this fragment
		for i := range addto {
			addto[i] += m
		}
		return addto
	}
	result := make([]Metric, 0, len(segments)*len(addto))
	for _, pfx := range addto {
		for _, seg := range segments {
			result = append(result, pfx+seg)
		}
	}
	return suffix.braceExpand(depth+1, result)
}

// A Value is a literal number, or a quoted string literal, which may
// contain arbitrary utf8-encoded characters. Numbers are represented
// as strings to avoid any loss in precision to repeated floating-point
// conversions.
type Value string

func (v *Value) String() string { return string(*v) }

func (x *Value) equal(y Expr) bool {
	if v, ok := y.(*Value); ok && v != nil {
		return *x == *v
	}
	return false
}
