// Package query parses Graphite queries into an
// abstract syntax tree.
package query

import "fmt"

//go:generate -command yacc go tool yacc
//go:generate yacc -o expr.go expr.y

//&target=averageSeries(company.server*.applicationInstance.requestsHandled)

// String produces the string representation of a (possibly modified)
// query. The return value is not url-encoded.
func (q *Query) String() string {
	return ""
}

// Metrics produces a list of metrics referenced in a query.
// The pointers returned by Metrics may be modified to
// change an expression.

// A Query is a parsed graphite target query, and may consist
// of a single metric name (or glob), or a function call.
type Query struct {
	Expr
}

// An Expr represents a graphite query subexpression.
type Expr interface {
	isExpr()
	equal(e Expr) bool
}

// A Func represents a function call.
type Func struct {
	Name string // The name of the function
	Args []Expr // zero or more arguments
}

func (xfn Func) equal(y Expr) bool {
	yfn, ok := y.(Func)
	if !ok {
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
// by dots.
type Metric string

func (x Metric) equal(y Expr) bool {
	if m, ok := y.(Metric); ok {
		return x == m
	}
	fmt.Printf("%q != %q (%[1]T != %T)\n", x, y)
	return false
}

// A Value is a literal number, or a quoted string literal, which may
// contain arbitrary utf8-encoded characters. Numbers are represented
// as strings to avoid any loss in precision to repeated floating-point
// conversions.
type Value string

func (x Value) equal(y Expr) bool {
	if m, ok := y.(Value); ok {
		return x == m
	}
	fmt.Printf("%q != %q (%[1]T != %T)\n", x, y)
	return false
}

// If you find the empty methods odd, see exprNode in
// https://golang.org/src/go/ast/ast.go , or the article
// "Sum types in Go": http://www.jerf.org/iri/post/2917

func (Func) isExpr()   {}
func (Metric) isExpr() {}
func (Value) isExpr()  {}
func (Query) isExpr()  {}

func (x Query) equal(y Query) bool {
	return x.Expr.equal(y.Expr)
}
