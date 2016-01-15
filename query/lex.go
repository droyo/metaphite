package query

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Based on the talk "Lexical Scanning in Go" by Rob Pike.
// http://talks.golang.org/2011/lex.slide

// character sets
const (
	charAlpha      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	charAlphanum   = charAlpha + charNumeric
	charDelim      = "(),"
	charGlob       = "[]{}*"
	charDot        = "."
	charIdentifier = charAlphanum + "-_"
	charNumeric    = "0123456789"
	charQuote      = `"'`
	charWhitespace = " \t\r\n\v\f"
)

func is(r int, sets ...string) bool {
	for _, v := range sets {
		if strings.ContainsRune(v, rune(r)) {
			return true
		}
	}
	return false
}

const eof = -1

type item struct {
	typ int // see %token decl in expr.y
	val string
}

type stateFn func(*lexer) stateFn

type lexer struct {
	input      string    // the input string
	start, pos int       // start, end+1 of current item
	width      int       // length of previous utf8 codepoint
	items      chan item // scanned lexemes go here
	err        []string  // errors from yacc
	result     Query     // yacc puts our result here
}

func lex(input string) *lexer {
	l := lexer{
		input: input,
		items: make(chan item),
	}
	go l.run()
	return &l
}

// implement the yyLex interface
func (l *lexer) Error(e string) { l.err = append(l.err, e) }
func (l *lexer) Lex(lval *yySymType) int {
	tok, ok := <-l.items
	if !ok {
		// eof reached
		return 0
	}
	lval.str = tok.val
	if tok.typ == pERROR {
		return 1
	}
	return tok.typ
}

func (l *lexer) Err() error {
	if len(l.err) > 0 {
		return errors.New(strings.Join(l.err, "\n\t"))
	}
	return nil
}

func (l *lexer) dot() string  { return l.input[l.start:l.pos] }
func (l *lexer) rest() string { return l.input[l.pos:] }
func (l *lexer) ignore()      { l.start = l.pos }
func (l *lexer) backup()      { l.pos -= l.width }
func (l *lexer) peek() int    { defer l.backup(); return l.next() }
func (l *lexer) emit(t int)   { l.items <- item{t, l.dot()}; l.start = l.pos }

func (l *lexer) errorf(format string, v ...interface{}) stateFn {
	l.items <- item{pERROR, fmt.Sprintf(format, v...)}
	return nil
}

func (l *lexer) run() {
	for state := lexClear; state != nil; state = state(l) {
	}
	close(l.items)
}

// Any call to lex() should be paired with a deferred
// call to l.drain(). This ensures that the lexing goroutine
// finishes.
func (l *lexer) drain() {
	for range l.items {
	}
}

// consumes the next character in the input
func (l *lexer) next() int {
	var r rune
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	r, l.width = utf8.DecodeRuneInString(l.rest())
	l.pos += l.width
	return int(r)
}

// consume a rune from the valid set
func (l *lexer) accept(valid ...string) bool {
	for _, v := range valid {
		if strings.ContainsRune(v, rune(l.next())) {
			return true
		}
		l.backup()
	}
	return false
}

// consume a run of consecutive runes from the valid set
func (l *lexer) acceptRun(valid ...string) {
Loop:
	for {
		r := rune(l.next())
		for _, v := range valid {
			if strings.ContainsRune(v, r) {
				continue Loop
			}
		}
		break Loop
	}
	l.backup()
}

// starting state, scan for any expression
func lexClear(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case is(r, charNumeric, "+-"):
			l.backup()
			return lexNumber
		case is(r, charWhitespace):
			l.ignore()
		case is(r, charIdentifier):
			l.backup()
			return lexName
		case is(r, charGlob):
			l.backup()
			return lexMetric
		case is(r, charDelim):
			l.emit(r)
			return lexClear
		case is(r, charQuote):
			// note that quote tokens contain their
			// surrounding tokens. this allows us to
			// preserve the formatting of string literals
			// exactly (this is a proxy, after all).
			l.backup()
			return lexQuote
		case r == eof:
			return nil
		default:
			return l.errorf("unexpected character '%c' (%d)", r)
		}
	}
	panic("not reached")
}

// read a (possibly negative, possibly) number.
// graphite only accepts decimal numbers. No
// scientific notation or imaginary numbers are
// allowed. But note that something like
//
// 	305.mymetric.count
//
// could be a valid name for a metric.
func lexNumber(l *lexer) stateFn {
	l.accept("+-")
	l.acceptRun(charNumeric)
	if l.accept(".") {
		l.acceptRun(charNumeric)
	}
	if l.accept(charWhitespace, charDelim) {
		l.backup()
		l.emit(pNUMBER)
		return lexClear
	}
	if l.accept(charAlphanum, charGlob, ".") {
		l.backup()
		return lexMetric
	}
	return l.errorf("unexpected character '%c' in number", l.peek())
}

// read a simple word, such as a function name
func lexName(l *lexer) stateFn {
	l.acceptRun(charIdentifier)
	if l.accept(charWhitespace, charDelim) {
		l.backup()
		l.emit(pWORD)
		return lexClear
	}
	if l.accept(charGlob, charDot) {
		return lexMetric
	}
	return l.errorf("unexpected character '%c' in word", l.peek())
}

// read a metric name, which is a series of words
// connected by dots. metrics may contain complex
// patterns, for instance
//
// 	servers.{prod,dev,stage}-sql[1-4].loadavg.*
//
// two additional states ensure the braces and brackets
// are balanced.
func lexMetric(l *lexer) stateFn {
	l.acceptRun(charIdentifier, "*.")
	if l.accept("{") {
		return lexCurlyBrace
	} else if l.accept("[") {
		return lexSquareBracket
	} else if l.accept(charWhitespace, charDelim) {
		l.backup()
		l.emit(pMETRIC)
		return lexClear
	} else if l.peek() == eof {
		l.emit(pMETRIC)
		return lexClear
	}
	return l.errorf("unexpected character '%c' in metric", l.peek())
}

// consume a glob expression of the form {x,y,z} (do not emit it)
// The opening '{' is already consumed. '}' characters may be
// escaped with a backslash.
func lexCurlyBrace(l *lexer) stateFn {
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof {
				break
			}
			fallthrough
		case eof:
			return l.errorf("unterminated brace list")
		case '}':
			return lexMetric
		}
	}
}

// consume, do not emit, a glob expression of the form [chars]
// The opening '[' is already consumed. characters may be
// escaped with a backslash.
func lexSquareBracket(l *lexer) stateFn {
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof {
				break
			}
			fallthrough
		case eof:
			return l.errorf("unterminated '[' glob")
		case ']':
			return lexMetric
		}
	}
}

// consume a quoted string. quotation marks within the
// string may be escaped with a backslash.
func lexQuote(l *lexer) stateFn {
	quoteChar := l.next()
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof {
				break
			}
			fallthrough
		case eof:
			return l.errorf("unterminated string")
		case quoteChar:
			break Loop
		}
	}
	l.emit(pSTRING)
	return lexClear
}
