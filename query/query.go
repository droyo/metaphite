// Package query parses Graphite queries into an
// abstract syntax tree.
package query

//go:generate -command yacc go tool yacc
//go:generate yacc -o expr.go expr.y

//&target=averageSeries(company.server*.applicationInstance.requestsHandled)

// String produces the string representation of a (possibly modified)
// query. The return value is not url-encoded.
func (q *Query) String() string {
	return q.orig
}

// Metrics produces a list of metrics referenced in a query.
// The pointers returned by Metrics may be modified to
// change an expression.

// A Query is a parsed graphite target query, and may consist
// of a simple metric name (or glob), or a function call.
type Query struct {
	Expr
	orig string
}

// An Expr represents a graphite query subexpression.
type Expr interface {
	isExpr()
}

// A Func represents a function call.
type Func struct {
	Name string // The name of the function
	Args []Expr // zero or more arguments
}

// A Metric is the name of a graphite metric, a list of words separated
// by dots.
type Metric string

// A Value is a literal number, or a quoted string literal, which may
// contain arbitrary utf8-encoded characters. Numbers are represented
// as strings to avoid any loss in precision to repeated floating-point
// conversions.
type Value string

// If you find the empty methods odd, see exprNode in
// https://golang.org/src/go/ast/ast.go , or the article
// "Sum types in Go": http://www.jerf.org/iri/post/2917

func (Func) isExpr()   {}
func (Metric) isExpr() {}
func (Value) isExpr()  {}
