package formula

import (
	"strings"
)

// Rewrite gives each reference of f to change, and writes back those it
// changed; the rest of f is kept as written.
func Rewrite(f string, change func(r *Ref) bool) (string, error) {
	tokens, err := Tokens(f)
	if err != nil {
		return f, err
	}
	var b strings.Builder
	last := 0
	for _, t := range tokens {
		if t.Kind != Reference || !change(t.Ref) {
			continue
		}
		b.WriteString(f[last:t.Pos])
		b.WriteString(t.Ref.String())
		last = t.Pos + len(t.Text)
	}
	if last == 0 {
		return f, nil
	}
	b.WriteString(f[last:])
	return b.String(), nil
}

// Translate is f copied dr rows down and dc columns right: its relative
// references move, those that leave the sheet become #REF!.
func Translate(f string, dr, dc int) (string, error) {
	if dr == 0 && dc == 0 {
		return f, nil
	}
	return Rewrite(f, func(r *Ref) bool {
		if r.Invalid {
			return false
		}
		a, ok := r.Area.translate(dr, dc)
		if !ok {
			r.Invalid = true
			return true
		}
		changed := a != r.Area
		r.Area = a
		return changed
	})
}

// translate is the area copied dr rows down and dc columns right, false
// when it leaves the sheet.
func (a Area) translate(dr, dc int) (Area, bool) {
	ok := true
	if !a.Cols {
		ok = move(&a.R1, a.AbsR1, dr, MaxRows) && move(&a.R2, a.AbsR2, dr, MaxRows)
	}
	if !a.Rows {
		ok = ok && move(&a.C1, a.AbsC1, dc, MaxCols) && move(&a.C2, a.AbsC2, dc, MaxCols)
	}
	return a.normal(), ok
}

func move(i *int, abs bool, d, limit int) bool {
	if abs {
		return true
	}
	*i += d
	return *i >= 1 && *i <= limit
}

// Shift is f once n rows (or columns, when rows is false) are inserted at
// at on the sheet named sheet, or removed when n is negative. own tells
// whether f lives on that sheet, where its references name no sheet.
// References to removed cells become #REF!.
func Shift(f, sheet string, own, rows bool, at, n int) (string, error) {
	return Rewrite(f, func(r *Ref) bool {
		if r.Invalid || r.Book != "" || r.LastSheet != "" {
			return false
		}
		if r.Sheet == "" && !own || r.Sheet != "" && !strings.EqualFold(r.Sheet, sheet) {
			return false
		}
		a := r.Area
		var ok bool
		if rows {
			if a.Cols {
				return false
			}
			a.R1, a.R2, ok = shiftSpan(a.R1, a.R2, at, n, MaxRows)
		} else {
			if a.Rows {
				return false
			}
			a.C1, a.C2, ok = shiftSpan(a.C1, a.C2, at, n, MaxCols)
		}
		if !ok {
			r.Invalid = true
			return true
		}
		if a == r.Area {
			return false
		}
		r.Area = a
		return true
	})
}

// shiftSpan moves the span lo to hi as inserting n at at, or removing -n,
// does; false when nothing is left of it.
func shiftSpan(lo, hi, at, n, limit int) (int, int, bool) {
	if n > 0 {
		if lo >= at {
			lo += n
		}
		if hi >= at {
			hi = min(hi+n, limit)
		}
		return lo, hi, lo <= limit
	}
	end := at - n
	switch {
	case lo >= end:
		lo += n
	case lo >= at:
		lo = at
	}
	switch {
	case hi >= end:
		hi += n
	case hi >= at:
		hi = at - 1
	}
	return lo, hi, lo <= hi
}

// Rename is f with its references to the sheet old pointing to new.
func Rename(f, old, new string) (string, error) {
	return Rewrite(f, func(r *Ref) bool {
		if r.Book != "" || !strings.EqualFold(r.Sheet, old) && !strings.EqualFold(r.LastSheet, old) {
			return false
		}
		if strings.EqualFold(r.Sheet, old) {
			r.Sheet = new
		}
		if strings.EqualFold(r.LastSheet, old) {
			r.LastSheet = new
		}
		return true
	})
}

// Drop is f once the sheet named sheet is deleted: its references to it
// become #REF!.
func Drop(f, sheet string) (string, error) {
	return Rewrite(f, func(r *Ref) bool {
		if r.Book != "" || !strings.EqualFold(r.Sheet, sheet) && !strings.EqualFold(r.LastSheet, sheet) {
			return false
		}
		*r = Ref{Invalid: true}
		return true
	})
}

// newer are the prefixes files give the functions newer than their format.
var newer = map[string]string{"FILTER": "_xlfn._xlws.", "SORT": "_xlfn._xlws."}

func init() {
	for _, name := range []string{"AGGREGATE", "ANCHORARRAY", "BETA.DIST", "BINOM.DIST", "CEILING.MATH",
		"CEILING.PRECISE", "CHISQ.DIST", "CHISQ.DIST.RT", "CHISQ.TEST", "COMBINA", "CONCAT", "CONFIDENCE.NORM",
		"CONFIDENCE.T", "COVARIANCE.P", "COVARIANCE.S", "DAYS", "EXPON.DIST", "F.DIST", "F.DIST.RT", "FLOOR.MATH",
		"FLOOR.PRECISE", "FORECAST.LINEAR", "GAMMA.DIST", "IFNA", "IFS", "ISO.CEILING", "ISOWEEKNUM",
		"LOGNORM.DIST", "MAXIFS", "MINIFS", "MODE.SNGL", "MUNIT", "NORM.DIST", "NORM.INV", "NORM.S.DIST",
		"NORM.S.INV", "NUMBERVALUE", "PERCENTILE.EXC", "PERCENTILE.INC", "PERCENTRANK.INC", "POISSON.DIST",
		"QUARTILE.EXC", "QUARTILE.INC", "RANDARRAY", "RANK.AVG", "RANK.EQ", "SEQUENCE", "SINGLE", "SORTBY",
		"STDEV.P", "STDEV.S", "SWITCH", "T.DIST", "T.DIST.2T", "T.DIST.RT", "T.INV", "T.INV.2T", "T.TEST",
		"TEXTAFTER", "TEXTBEFORE", "TEXTJOIN", "UNICHAR", "UNICODE", "UNIQUE", "VAR.P", "VAR.S", "XLOOKUP",
		"XMATCH", "XOR"} {
		newer[name] = "_xlfn."
	}
}

// Stored is f as files store it: the functions newer than the format
// with their prefix, _xlfn.SEQUENCE, and A1# as _xlfn.ANCHORARRAY(A1).
func Stored(f string) string {
	if !strings.ContainsAny(f, "(#") {
		return f
	}
	tokens, err := Tokens(f)
	if err != nil {
		return f
	}
	var b strings.Builder
	for i, t := range tokens {
		spill := i+1 < len(tokens) && tokens[i+1].Kind == Operator && tokens[i+1].Text == "#"
		switch {
		case t.Kind == Function && strings.ToUpper(t.Text) == FunctionName(t.Text):
			b.WriteString(newer[strings.ToUpper(t.Text[:len(t.Text)-1])])
		case (t.Kind == Reference || t.Kind == Name) && spill:
			b.WriteString("_xlfn.ANCHORARRAY(" + t.Text + ")")
			continue
		case t.Kind == Operator && t.Text == "#" && i > 0 && (tokens[i-1].Kind == Reference || tokens[i-1].Kind == Name):
			continue
		}
		b.WriteString(t.Text)
	}
	return b.String()
}
