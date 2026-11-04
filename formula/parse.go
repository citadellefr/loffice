package formula

import (
	"errors"
	"strconv"
	"strings"
)

// Expr is a formula read into a tree.
type Expr interface{ expr() }

type (
	numberExpr float64
	textExpr   string
	boolExpr   bool
	errorExpr  string
	refExpr    struct{ ref *Ref }
	// nameExpr is a defined name, possibly of another sheet.
	nameExpr struct{ sheet, name string }
	// structExpr is a structured reference to a table.
	structExpr struct{ s Structured }
	// unaryExpr is -x, +x, or @x, the implicit intersection.
	unaryExpr struct {
		op byte
		x  Expr
	}
	percentExpr struct{ x Expr }
	// spillExpr is x#, the array a dynamic formula spills.
	spillExpr struct{ x Expr }
	// binaryExpr is an operator between two operands: arithmetic,
	// comparison, "&", and the reference operators ":", " " and ",".
	binaryExpr struct {
		op   string
		x, y Expr
	}
	// callExpr is a function, named in upper case without the prefix of
	// functions newer than the file format ("_xlfn.").
	callExpr struct {
		name string
		args []Expr
	}
	// invokeExpr calls the LAMBDA a function gives: LAMBDA(x,x+1)(2).
	invokeExpr struct {
		fn   Expr
		args []Expr
	}
	arrayExpr struct {
		rows, cols int
		cells      []Expr
	}
	// missingExpr is an argument left out: IF(A1,,2).
	missingExpr struct{}
	// boundExpr is a reference whose sheets are known, nil when they are
	// not.
	boundExpr struct{ refs []Area3 }
)

func (numberExpr) expr()  {}
func (textExpr) expr()    {}
func (boolExpr) expr()    {}
func (errorExpr) expr()   {}
func (refExpr) expr()     {}
func (nameExpr) expr()    {}
func (structExpr) expr()  {}
func (unaryExpr) expr()   {}
func (percentExpr) expr() {}
func (spillExpr) expr()   {}
func (binaryExpr) expr()  {}
func (callExpr) expr()    {}
func (invokeExpr) expr()  {}
func (arrayExpr) expr()   {}
func (missingExpr) expr() {}
func (boundExpr) expr()   {}

// token is a token that is not a space, knowing whether one came before.
type token struct {
	Token
	spaced bool
}

type parser struct {
	tokens []token
	i      int
	depth  int
}

// MaxDepth is how deep a formula may nest functions, parentheses and
// unary operators: the 64 levels of functions Excel accepts.
const MaxDepth = 64

// ErrDepth is the error of a formula nested deeper than MaxDepth.
var ErrDepth = errors.New("formula: nested too deeply")

// enter goes one level deeper, and leave comes back.
func (p *parser) enter() error {
	if p.depth == MaxDepth {
		return ErrDepth
	}
	p.depth++
	return nil
}

func (p *parser) leave() { p.depth-- }

// Parse reads a formula, written without its "=".
func Parse(f string) (Expr, error) {
	all, err := Tokens(f)
	if err != nil {
		return nil, err
	}
	p := &parser{}
	spaced := false
	for _, t := range all {
		if t.Kind == Space {
			spaced = true
			continue
		}
		p.tokens = append(p.tokens, token{t, spaced})
		spaced = false
	}
	if len(p.tokens) == 0 {
		return nil, ErrSyntax
	}
	e, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.tokens) {
		return nil, ErrSyntax
	}
	return e, nil
}

func (p *parser) peek() *token {
	if p.i < len(p.tokens) {
		return &p.tokens[p.i]
	}
	return nil
}

func (p *parser) op(ops ...string) (string, bool) {
	t := p.peek()
	if t == nil || t.Kind != Operator {
		return "", false
	}
	for _, o := range ops {
		if t.Text == o {
			return o, true
		}
	}
	return "", false
}

// levels are the binary operators, from the loosest.
var levels = [][]string{{"=", "<>", "<", ">", "<=", ">="}, {"&"}, {"+", "-"}, {"*", "/"}, {"^"}}

func (p *parser) expr(level int) (Expr, error) {
	if level == len(levels) {
		return p.percent()
	}
	x, err := p.expr(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.op(levels[level]...)
		if !ok {
			return x, nil
		}
		p.i++
		y, err := p.expr(level + 1)
		if err != nil {
			return nil, err
		}
		x = binaryExpr{op, x, y}
	}
}

func (p *parser) percent() (Expr, error) {
	x, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.op("%"); !ok {
			return x, nil
		}
		p.i++
		x = percentExpr{x}
	}
}

func (p *parser) unary() (Expr, error) {
	if op, ok := p.op("-", "+", "@"); ok {
		p.i++
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return unaryExpr{op[0], x}, nil
	}
	return p.intersection()
}

// intersection is references a space separates: the cells they share.
func (p *parser) intersection() (Expr, error) {
	x, err := p.rangeOp()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || !t.spaced || !refLike(x) || t.Kind != Reference && t.Kind != Name && t.Kind != Function && t.Kind != Open {
			return x, nil
		}
		y, err := p.rangeOp()
		if err != nil {
			return nil, err
		}
		x = binaryExpr{" ", x, y}
	}
}

// refLike tells whether an expression may stand for a reference.
func refLike(e Expr) bool {
	switch e := e.(type) {
	case refExpr, nameExpr, structExpr, callExpr, invokeExpr:
		return true
	case binaryExpr:
		return e.op == ":" || e.op == " " || e.op == ","
	}
	return false
}

func (p *parser) rangeOp() (Expr, error) {
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.op(":"); !ok {
			break
		}
		p.i++
		y, err := p.primary()
		if err != nil {
			return nil, err
		}
		x = binaryExpr{":", x, y}
	}
	if _, ok := p.op("#"); ok {
		p.i++
		x = spillExpr{x}
	}
	return x, nil
}

func (p *parser) primary() (Expr, error) {
	t := p.peek()
	if t == nil {
		return nil, ErrSyntax
	}
	p.i++
	switch t.Kind {
	case Number:
		n, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			return nil, ErrSyntax
		}
		return numberExpr(n), nil
	case String:
		return textExpr(strings.ReplaceAll(t.Text[1:len(t.Text)-1], `""`, `"`)), nil
	case Bool:
		return boolExpr(strings.EqualFold(t.Text, "TRUE")), nil
	case Error:
		return errorExpr(strings.ToUpper(t.Text)), nil
	case Reference:
		return refExpr{t.Ref}, nil
	case Name:
		if s, ok := ParseStructured(t.Text); ok {
			return structExpr{s}, nil
		}
		sheet, name := "", t.Text
		if i := strings.LastIndexByte(name, '!'); i >= 0 {
			_, sheet, _ = splitPrefix(name[:i])
			name = name[i+1:]
		} else {
			name = trimParam(name)
		}
		return nameExpr{sheet, name}, nil
	case Function:
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		args, err := p.args()
		if err != nil {
			return nil, err
		}
		var x Expr = callExpr{FunctionName(t.Text[:len(t.Text)-1]), args}
		for t := p.peek(); t != nil && t.Kind == Open && !t.spaced; t = p.peek() {
			p.i++
			if args, err = p.args(); err != nil {
				return nil, err
			}
			x = invokeExpr{x, args}
		}
		return x, nil
	case Open:
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer p.leave()
		x, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		for {
			if t := p.peek(); t != nil && t.Kind == Comma {
				p.i++
				y, err := p.expr(0)
				if err != nil {
					return nil, err
				}
				x = binaryExpr{",", x, y}
				continue
			}
			break
		}
		if t := p.peek(); t == nil || t.Kind != Close {
			return nil, ErrSyntax
		}
		p.i++
		return x, nil
	case OpenArray:
		return p.array()
	}
	return nil, ErrSyntax
}

// paramPrefix is the prefix files give the names LET and LAMBDA declare.
const paramPrefix = "_xlpm."

// FunctionName is the name of a function as a formula calls it, without
// the prefixes of the file format.
func FunctionName(s string) string {
	s = strings.ToUpper(s)
	for _, prefix := range []string{"_XLFN._XLWS.", "_XLFN.", "_XLWS.", "_XLPM."} {
		s = strings.TrimPrefix(s, prefix)
	}
	return s
}

// args reads the arguments of a call, after its "(" and up to its ")".
func (p *parser) args() ([]Expr, error) {
	if t := p.peek(); t != nil && t.Kind == Close {
		p.i++
		return nil, nil
	}
	var args []Expr
	for {
		t := p.peek()
		if t == nil {
			return nil, ErrSyntax
		}
		if t.Kind == Comma || t.Kind == Close {
			args = append(args, missingExpr{})
		} else {
			x, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, x)
		}
		t = p.peek()
		switch {
		case t == nil:
			return nil, ErrSyntax
		case t.Kind == Comma:
			p.i++
		case t.Kind == Close:
			p.i++
			return args, nil
		default:
			return nil, ErrSyntax
		}
	}
}

// array reads the constant of an array: {1,2;3,4}.
func (p *parser) array() (Expr, error) {
	a := arrayExpr{rows: 1}
	col := 0
	for {
		neg := false
		if _, ok := p.op("-"); ok {
			neg = true
			p.i++
		}
		e, err := p.primary()
		if err != nil {
			return nil, err
		}
		switch x := e.(type) {
		case numberExpr:
			if neg {
				e = -x
			}
		case textExpr, boolExpr, errorExpr:
			if neg {
				return nil, ErrSyntax
			}
		default:
			return nil, ErrSyntax
		}
		a.cells = append(a.cells, e)
		col++
		t := p.peek()
		if t == nil {
			return nil, ErrSyntax
		}
		p.i++
		switch t.Kind {
		case Comma:
		case Semicolon:
			if a.cols == 0 {
				a.cols = col
			} else if col != a.cols {
				return nil, ErrSyntax
			}
			a.rows++
			col = 0
		case CloseArray:
			if a.cols == 0 {
				a.cols = col
			} else if col != a.cols {
				return nil, ErrSyntax
			}
			return a, nil
		default:
			return nil, ErrSyntax
		}
	}
}
