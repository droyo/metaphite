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
	quoteChar         rune
	err               []string
	result            Query
}

var eof rune = -1

type lexState int

type charClass int

const (
	charAlpha charClass = iota
	charDigit
	charQuote
	charPunct
	charSpace
	charEscape
	charEOF
)

// We use a FSM for the lexical analysis.
// Row = Current state, Col = character class.
var fsm = [...][...]func(*lexer, rune) error{
	stateBegin: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).word,
		charDigit:  (*lexer).number,
		charQuote:  (*lexer).quoted,
		charPunct:  (*lexer).constant,
		charSpace:  (*lexer).noop,
		charEscape: (*lexer).constant,
		charEOF:    (*lexer).success,
	},
	stateQuote: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).noop,
		charDigit:  (*lexer).noop,
		charQuote:  (*lexer).endquote,
		charPunct:  (*lexer).noop,
		charSpace:  (*lexer).noop,
		charEscape: (*lexer).escape,
		charEOF:    (*lexer).failWith("unterminated string literal"),
	},
	stateBackslash: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).endEscape,
		charDigit:  (*lexer).endEscape,
		charQuote:  (*lexer).endEscape,
		charPunct:  (*lexer).endEscape,
		charSpace:  (*lexer).endEscape,
		charEscape: (*lexer).endEscape,
		charEOF:    (*lexer).failWith("EOF after backslash"),
	},
	stateWord: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).noop,
		charDigit:  (*lexer).noop,
		charQuote:  (*lexer).unexpected,
		charPunct:  (*lexer).endWord,
		charSpace:  (*lexer).endWord,
		charEscape: (*lexer).constant,
		charEOF:    (*lexer).endWord,
	},
	stateNumber: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).word,
		charDigit:  (*lexer).noop,
		charQuote:  (*lexer).unexpected,
		charPunct:  (*lexer).decimal,
		charSpace:  (*lexer).endNumber,
		charEscape: (*lexer).constant,
		charEOF:    (*lexer).endNumber,
	},
	stateDecimal: [...]func(*lexer, rune) error{
		charAlpha:  (*lexer).unexpected,
		charDigit:  (*lexer).noop,
		charQuote:  (*lexer).unexpected,
		charPunct:  (*lexer).endDecimal,
		charSpace:  (*lexer).endDecimal,
		charEscape: (*lexer).unexpected,
		charEOF:    (*lexer).endDecimal,
	},
}

const (
	stateBegin lexState = iota
	stateSingleQuote
	stateDoubleQuote
	stateBackslash
	stateWord
	stateNumber
	stateDecimal
	stateEnd
)

func (l *lexer) dot() string {
	return l.target[l.start:l.pos]
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
				lval.str = ""
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
				lval.str = l.target[l.start:l.pos_1]
				l.pop()
				l.start = l.pos
				return STRING
			}
		case stateDoubleQuote:
			if c == eof {
				l.errorf("unterminated string")
				return -1
			} else if c == '\\' {
				l.push(stateBackslash)
			} else if c == '"' {
				l.start++
				lval.str = l.target[l.start:l.pos_1]
				l.pop()
				l.start = l.pos
				return STRING
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
				lval.str = l.dot()
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
				lval.str = l.dot()
				l.start = l.pos
				return NUMBER
			}
		case stateDecimal:
			if unicode.IsDigit(c) {
				// noop
			} else {
				l.pop()
				l.unstep()
				lval.str = l.dot()
				l.start = l.pos
				return NUMBER
			}
		default:
			panic(fmt.Errorf("unknown lexer state %d", l.state()))
		}
	}
	return 0
}
