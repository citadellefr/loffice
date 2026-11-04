// Package formula reads, rewrites and calculates Excel formulas, as a
// workbook stores them: in A1 notation, with English function names and
// commas between arguments.
package formula

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is the kind of a token.
type Kind uint8

const (
	Number Kind = iota + 1
	String
	Bool
	Error
	// Reference is a cell, an area, whole rows or columns, possibly on
	// other sheets; its Ref is set.
	Reference
	// Name is a defined name, or a table with its structured reference
	// ("Sales[Amount]").
	Name
	// Function is a function name, the "(" that follows it included.
	Function
	Operator
	Open
	Close
	Comma
	Semicolon
	OpenArray
	CloseArray
	Space
)

// Token is a piece of a formula: Text is the formula from Pos on.
type Token struct {
	Kind Kind
	Text string
	Pos  int
	Ref  *Ref
}

var ErrSyntax = errors.New("formula: syntax error")

// Errors a formula may produce or hold.
var errorCodes = []string{"#NULL!", "#DIV/0!", "#VALUE!", "#REF!", "#NAME?", "#NUM!", "#N/A", "#GETTING_DATA", "#SPILL!", "#CALC!", "#FIELD!", "#BLOCKED!", "#CONNECT!", "#BUSY!", "#UNKNOWN!"}

// IsError tells whether s is an error value, as a cell holds it.
func IsError(s string) bool {
	return slices.Contains(errorCodes, s)
}

// Tokens splits a formula, written without its "=", into tokens that
// cover it whole.
func Tokens(f string) ([]Token, error) {
	var out []Token
	for i := 0; i < len(f); {
		t, err := next(f, i)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		i = t.Pos + len(t.Text)
	}
	return out, nil
}

func next(f string, i int) (Token, error) {
	c := f[i]
	tok := func(k Kind, n int) (Token, error) { return Token{Kind: k, Text: f[i : i+n], Pos: i}, nil }
	switch {
	case c == ' ' || c == '\n' || c == '\r' || c == '\t':
		n := 1
		for i+n < len(f) && strings.IndexByte(" \n\r\t", f[i+n]) >= 0 {
			n++
		}
		return tok(Space, n)
	case c == '"':
		n := 1
		for {
			j := strings.IndexByte(f[i+n:], '"')
			if j < 0 {
				return Token{}, ErrSyntax
			}
			n += j + 1
			if i+n < len(f) && f[i+n] == '"' {
				n++
				continue
			}
			return tok(String, n)
		}
	case c == '#':
		for _, e := range errorCodes {
			if hasPrefixFold(f[i:], e) {
				if ref, n, ok := readRef(f, i); ok {
					return Token{Kind: Reference, Text: f[i : i+n], Pos: i, Ref: ref}, nil
				}
				return tok(Error, len(e))
			}
		}
		// the spill operator: A1#
		return tok(Operator, 1)
	case c >= '0' && c <= '9' || c == '.' && i+1 < len(f) && f[i+1] >= '0' && f[i+1] <= '9':
		if ref, n, ok := readRef(f, i); ok {
			return Token{Kind: Reference, Text: f[i : i+n], Pos: i, Ref: ref}, nil
		}
		return tok(Number, numberLen(f[i:]))
	case c == '(':
		return tok(Open, 1)
	case c == ')':
		return tok(Close, 1)
	case c == ',':
		return tok(Comma, 1)
	case c == ';':
		return tok(Semicolon, 1)
	case c == '{':
		return tok(OpenArray, 1)
	case c == '}':
		return tok(CloseArray, 1)
	case c == '<' || c == '>':
		if i+1 < len(f) && (f[i+1] == '=' || c == '<' && f[i+1] == '>') {
			return tok(Operator, 2)
		}
		return tok(Operator, 1)
	case strings.IndexByte("+-*/^&=%:@!", c) >= 0:
		return tok(Operator, 1)
	}
	if ref, n, ok := readRef(f, i); ok {
		return Token{Kind: Reference, Text: f[i : i+n], Pos: i, Ref: ref}, nil
	}
	// a structured reference of the table holding the formula: [@Col]
	if c == '[' {
		n, ok := bracketLen(f[i:])
		if !ok {
			return Token{}, ErrSyntax
		}
		return tok(Name, n)
	}
	n := identLen(f[i:])
	if n == 0 {
		return Token{}, ErrSyntax
	}
	// a sheet-qualified name: Sheet1!Total, 'My sheet'!Total
	if c == '\'' || i+n < len(f) && f[i+n] == '!' {
		p, ok := prefixLen(f[i:])
		if !ok {
			return Token{}, ErrSyntax
		}
		m := identLen(f[i+p:])
		if m == 0 {
			return Token{}, ErrSyntax
		}
		return tok(Name, p+m)
	}
	word := f[i : i+n]
	switch {
	case i+n < len(f) && f[i+n] == '(':
		return tok(Function, n+1)
	case i+n < len(f) && f[i+n] == '[':
		m, ok := bracketLen(f[i+n:])
		if !ok {
			return Token{}, ErrSyntax
		}
		return tok(Name, n+m)
	case strings.EqualFold(word, "TRUE") || strings.EqualFold(word, "FALSE"):
		return tok(Bool, n)
	}
	return tok(Name, n)
}

// hasPrefixFold tells whether s starts with prefix, whatever the case.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func numberLen(s string) int {
	n := 0
	for n < len(s) && (s[n] >= '0' && s[n] <= '9' || s[n] == '.') {
		n++
	}
	if n < len(s) && (s[n] == 'e' || s[n] == 'E') {
		m := n + 1
		if m < len(s) && (s[m] == '+' || s[m] == '-') {
			m++
		}
		if m < len(s) && s[m] >= '0' && s[m] <= '9' {
			for m < len(s) && s[m] >= '0' && s[m] <= '9' {
				m++
			}
			n = m
		}
	}
	return n
}

// identLen is the length of the name s starts with: letters, digits, "_",
// "." "\" and "?".
func identLen(s string) int {
	n := 0
	for n < len(s) {
		r, size := utf8.DecodeRuneInString(s[n:])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '\\' || r == '?' || r > 0x7F && !unicode.IsSpace(r)) {
			break
		}
		n += size
	}
	return n
}

// bracketLen is the length of the structured reference s starts with,
// "[" to its matching "]".
func bracketLen(s string) (int, bool) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			i++ // an escaped character
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

// prefixLen is the length of the sheet prefix s starts with, "!"
// included: Sheet1!, 'My sheet'!, Sheet1:Sheet3!, [1]Sheet1!.
func prefixLen(s string) (int, bool) {
	n := 0
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return 0, false
		}
		n = end + 1
	}
	if n < len(s) && s[n] == '\'' {
		for j := n + 1; j < len(s); j++ {
			if s[j] != '\'' {
				continue
			}
			if j+1 < len(s) && s[j+1] == '\'' {
				j++
				continue
			}
			if j+1 < len(s) && s[j+1] == '!' {
				return j + 2, true
			}
			return 0, false
		}
		return 0, false
	}
	for {
		m := identLen(s[n:])
		if m == 0 {
			return 0, false
		}
		n += m
		if n < len(s) && s[n] == '!' {
			return n + 1, true
		}
		if n < len(s) && s[n] == ':' {
			n++
			continue
		}
		return 0, false
	}
}
