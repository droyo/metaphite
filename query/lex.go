package query

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenizer for graphite queries
type lexer struct {
	target            string
	start, pos, pos_1 int
	stateStack        []lexState
	err               []string
	result            Query
}

var eof rune = -1

type lexState int

const (
	stateBegin lexState = iota
	stateSingleQuote
	stateDoubleQuote
	stateBackslash
	stateWord
	stateNumber
	stateDecimal
)

func (l *lexer) dot() Value {
	return Value(l.target[l.start:l.pos])
}

func (l *lexer) Err() error {
	if len(l.err) > 0 {
		return errors.New(strings.Join(l.err, "\n"))
	}
	return nil
}

func (l *lexer) errorf(format string, v ...interface{}) {
	l.err = append(l.err, fmt.Sprintf(format, v...))
}

func (l *lexer) state() lexState {
	if len(l.stateStack) > 0 {
		return l.stateStack[len(l.stateStack)-1]
	}
	return stateBegin
}

func (l *lexer) unstep() {
	if l.pos == l.pos_1 {
		panic("error: unstep() without step()")
	}
	l.pos = l.pos_1
}

func (l *lexer) step(c *rune) bool {
	if l.pos > len(l.target) {
		return false
	}
	l.pos_1 = l.pos
	if l.pos == len(l.target) {
		*c = eof
		l.pos++
		return true
	}
	r, n := utf8.DecodeRuneInString(l.target[l.pos:])
	l.pos += n
	*c = r
	return true
}

func (l *lexer) push(state lexState) {
	l.stateStack = append(l.stateStack, state)
}

func (l *lexer) pop() lexState {
	s := l.state()
	if len(l.stateStack) > 0 {
		l.stateStack = l.stateStack[:len(l.stateStack)-1]
	}
	return s
}

// See https://github.com/graphite-project/graphite-web/blob/master/webapp/graphite/render/grammar.py
func punct(c rune) bool {
	return c == ',' || c == '(' || c == ')' || c == '.' || c == '{' || c == '}'
}

func (l *lexer) Error(e string) {
	l.err = append(l.err, e)
}

func (l *lexer) Lex(lval *yySymType) int {
	var c rune
	for l.step(&c) {
		switch l.state() {
		case stateBegin:
			if c == eof {
				return 0
			} else if c == '\\' {
				l.push(stateBackslash)
			} else if punct(c) {
				l.start = l.pos
				lval.val = ""
				return int(c)
			} else if unicode.IsSpace(c) {
				l.start = l.pos
			} else if c == '-' {
				l.push(stateNumber)
			} else if unicode.IsDigit(c) {
				l.push(stateNumber)
			} else if unicode.IsLetter(c) {
				l.push(stateWord)
			} else if c == '_' || c == '[' || c == ']' {
				l.push(stateWord)
			} else if c == '\'' {
				l.push(stateSingleQuote)
			} else if c == '"' {
				l.push(stateDoubleQuote)
			} else {
				l.errorf("unexpected char '%c'", c)
				return -1
			}
		case stateSingleQuote:
			if c == eof {
				l.errorf("unterminated string")
				return -1
			} else if c == '\\' {
				l.push(stateBackslash)
			} else if c == '\'' {
				l.start++
				lval.val = Value(l.target[l.start:l.pos_1])
				l.pop()
				l.start = l.pos
				return VALUE
			}
		case stateDoubleQuote:
			if c == eof {
				l.errorf("unterminated string")
				return -1
			} else if c == '\\' {
				l.push(stateBackslash)
			} else if c == '"' {
				l.start++
				lval.val = Value(l.target[l.start:l.pos_1])
				l.pop()
				l.start = l.pos
				return VALUE
			}
		case stateBackslash:
			if c == eof {
				l.errorf("eof after backslash")
				return -1
			}
			l.pop()
		case stateWord:
			if unicode.IsLetter(c) {
				// noop
			} else if unicode.IsDigit(c) {
				// noop
			} else if c == '_' {
				// noop
			} else if c == '-' {
				// noop
			} else if c == '[' {
				// noop
			} else if c == ']' {
				// noop
			} else if c == '*' {
				// noop
			} else {
				l.pop()
				l.unstep()
				lval.val = l.dot()
				l.start = l.pos
				return WORD
			}
		case stateNumber:
			if unicode.IsDigit(c) {
				// noop
			} else if c == '.' {
				l.pop()
				l.push(stateDecimal)
			} else {
				l.pop()
				l.unstep()
				lval.val = l.dot()
				l.start = l.pos
				return WORD
			}
		case stateDecimal:
			if unicode.IsDigit(c) {
				// noop
			} else {
				l.pop()
				l.unstep()
				lval.val = l.dot()
				l.start = l.pos
				return VALUE
			}
		default:
			panic(fmt.Errorf("unknown lexer state %d", l.state()))
		}
	}
	return 0
}
