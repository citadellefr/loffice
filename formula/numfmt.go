package formula

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A Format is a number format code read: "0.00", "#,##0 €;[Red]-#,##0 €",
// "dd/mm/yyyy hh:mm". Its separators are those of the file, "." for the
// decimals and "," for the thousands; a Locale writes its own.
type Format struct {
	sections []*fmtSection
}

type fmtSection struct {
	tokens []fmtToken
	color  string
	// cond is a condition such as [>=100], which chooses the section.
	cond      string
	condValue float64
	date      bool
	text      bool // holds @
	general   bool
	percent   int
	// scale is the thousands a trailing comma divides by.
	scale     int
	thousands bool
	exp       bool
	fraction  bool
	// digits placeholders: of the integer part, the decimals, the
	// exponent, the numerator and denominator of a fraction
	intDigits, fracDigits int
	// denominator is a fixed one: # ?/8
	denominator int
	// elapsed is [h], [m] or [s]: hours, minutes or seconds in all
	elapsed bool
	// secDigits are the decimals of seconds: ss.00
	secDigits int
	ampm      bool
}

type tokenKind uint8

const (
	tokLiteral tokenKind = iota
	tokDigit             // 0 # ?
	tokDecimal
	tokExp // E+ E-
	tokSlash
	tokDate // y m d h s, AM/PM, elapsed
	tokText // @
	tokGeneral
)

type fmtToken struct {
	kind tokenKind
	text string
	// part tells which part a digit belongs to: 'i' integer, 'f' decimals,
	// 'e' exponent, 'n' numerator, 'd' denominator.
	part byte
}

// ParseFormat reads a number format code.
func ParseFormat(code string) *Format {
	f := &Format{}
	for _, s := range splitSections(code) {
		f.sections = append(f.sections, parseSection(s))
		if len(f.sections) == 4 {
			break
		}
	}
	if len(f.sections) == 0 {
		f.sections = []*fmtSection{{general: true, tokens: []fmtToken{{kind: tokGeneral}}}}
	}
	return f
}

// splitSections cuts a code at the semicolons outside quotes and brackets.
func splitSections(code string) []string {
	var out []string
	start := 0
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '"':
			if j := strings.IndexByte(code[i+1:], '"'); j >= 0 {
				i += j + 1
			} else {
				i = len(code)
			}
		case '\\', '_', '*':
			i++
		case '[':
			if j := strings.IndexByte(code[i:], ']'); j >= 0 {
				i += j
			}
		case ';':
			out = append(out, code[start:i])
			start = i + 1
		}
	}
	return append(out, code[start:])
}

var colorNames = []string{"black", "blue", "cyan", "green", "magenta", "red", "white", "yellow"}

func parseSection(code string) *fmtSection {
	s := &fmtSection{}
	lit := func(t string) { s.tokens = append(s.tokens, fmtToken{kind: tokLiteral, text: t}) }
	lower := strings.ToLower(code)
	for i := 0; i < len(code); {
		c := code[i]
		switch {
		case c == '"':
			j := strings.IndexByte(code[i+1:], '"')
			if j < 0 {
				lit(code[i+1:])
				i = len(code)
				continue
			}
			lit(code[i+1 : i+1+j])
			i += j + 2
		case c == '\\' && i+1 < len(code):
			_, n := utf8.DecodeRuneInString(code[i+1:])
			lit(code[i+1 : i+1+n])
			i += 1 + n
		case c == '_' && i+1 < len(code):
			_, n := utf8.DecodeRuneInString(code[i+1:])
			lit(" ")
			i += 1 + n
		case c == '*' && i+1 < len(code):
			_, n := utf8.DecodeRuneInString(code[i+1:])
			i += 1 + n
		case c == '[':
			j := strings.IndexByte(code[i:], ']')
			if j < 0 {
				i = len(code)
				continue
			}
			s.bracket(code[i+1:i+j], lit)
			i += j + 1
		case strings.HasPrefix(lower[i:], "general"):
			s.general = true
			s.tokens = append(s.tokens, fmtToken{kind: tokGeneral})
			i += 7
		case c == '0' || c == '#' || c == '?':
			s.tokens = append(s.tokens, fmtToken{kind: tokDigit, text: string(c)})
			i++
		case c == '.':
			if s.lastDate() {
				n := 0
				for i+1+n < len(code) && code[i+1+n] == '0' {
					n++
				}
				if n > 0 {
					s.secDigits = n
					s.tokens = append(s.tokens, fmtToken{kind: tokDate, text: "." + strings.Repeat("0", n)})
					i += 1 + n
					continue
				}
			}
			s.tokens = append(s.tokens, fmtToken{kind: tokDecimal})
			i++
		case c == ',':
			// a comma between digits groups thousands; after them it scales
			if s.lastDigit() && i+1 < len(code) && (code[i+1] == '0' || code[i+1] == '#' || code[i+1] == '?') {
				s.thousands = true
			} else if s.lastDigit() || s.scale > 0 && i > 0 && code[i-1] == ',' {
				s.scale++
			} else {
				lit(",")
			}
			i++
		case c == '%':
			s.percent++
			lit("%")
			i++
		case (c == 'E' || c == 'e') && i+1 < len(code) && (code[i+1] == '+' || code[i+1] == '-') && s.hasDigit():
			s.exp = true
			s.tokens = append(s.tokens, fmtToken{kind: tokExp, text: code[i : i+2]})
			i += 2
		case c == '/' && s.lastDigit():
			s.fraction = true
			s.tokens = append(s.tokens, fmtToken{kind: tokSlash})
			i++
			j := i
			for j < len(code) && code[j] >= '1' && code[j] <= '9' || j > i && j < len(code) && code[j] == '0' {
				j++
			}
			if j > i {
				s.denominator, _ = strconv.Atoi(code[i:j])
				s.tokens = append(s.tokens, fmtToken{kind: tokLiteral, text: code[i:j], part: 'D'})
				i = j
			}
		case c == '@':
			s.text = true
			s.tokens = append(s.tokens, fmtToken{kind: tokText})
			i++
		case strings.HasPrefix(lower[i:], "am/pm"):
			s.ampm, s.date = true, true
			s.tokens = append(s.tokens, fmtToken{kind: tokDate, text: "AM/PM"})
			i += 5
		case strings.HasPrefix(lower[i:], "a/p"):
			s.ampm, s.date = true, true
			s.tokens = append(s.tokens, fmtToken{kind: tokDate, text: "A/P"})
			i += 3
		case strings.ContainsRune("ymdhse", rune(c|0x20)) && !(c|0x20 == 'e' && !s.date):
			j := i
			for j < len(code) && code[j]|0x20 == c|0x20 {
				j++
			}
			s.date = true
			s.tokens = append(s.tokens, fmtToken{kind: tokDate, text: strings.ToLower(code[i:j])})
			i = j
		default:
			_, n := utf8.DecodeRuneInString(code[i:])
			lit(code[i : i+n])
			i += n
		}
	}
	s.parts()
	return s
}

// bracket reads what a section holds in brackets: a color, a condition,
// a currency, an elapsed time.
func (s *fmtSection) bracket(b string, lit func(string)) {
	lower := strings.ToLower(b)
	switch {
	case strings.HasPrefix(b, "$"):
		sym, _, _ := strings.Cut(b[1:], "-")
		if sym != "" {
			lit(sym)
		}
	case lower == "h" || lower == "hh" || lower == "m" || lower == "mm" || lower == "s" || lower == "ss":
		s.date, s.elapsed = true, true
		s.tokens = append(s.tokens, fmtToken{kind: tokDate, text: "[" + lower + "]"})
	case strings.IndexAny(b, "<>=") == 0:
		op := b[:1]
		if len(b) > 1 && strings.ContainsRune("<>=", rune(b[1])) {
			op = b[:2]
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(b[len(op):]), 64); err == nil {
			s.cond, s.condValue = op, v
		}
	case strings.HasPrefix(lower, "color"):
		s.color = lower
	default:
		for _, c := range colorNames {
			if lower == c {
				s.color = c
			}
		}
	}
}

func (s *fmtSection) lastDigit() bool {
	return len(s.tokens) > 0 && s.tokens[len(s.tokens)-1].kind == tokDigit
}

func (s *fmtSection) lastDate() bool {
	for i := len(s.tokens) - 1; i >= 0; i-- {
		switch s.tokens[i].kind {
		case tokDate:
			return strings.HasPrefix(s.tokens[i].text, "s") || s.tokens[i].text == "[s]" || s.tokens[i].text == "[ss]"
		case tokLiteral:
			continue
		}
		return false
	}
	return false
}

func (s *fmtSection) hasDigit() bool {
	for _, t := range s.tokens {
		if t.kind == tokDigit {
			return true
		}
	}
	return false
}

// parts tells each digit placeholder which part of the number it writes,
// and which "m" are minutes.
func (s *fmtSection) parts() {
	part := byte('i')
	for i := range s.tokens {
		t := &s.tokens[i]
		switch t.kind {
		case tokDecimal:
			if part == 'i' {
				part = 'f'
			}
		case tokExp:
			part = 'e'
		case tokSlash:
			part = 'd'
		case tokDigit:
			t.part = part
		}
	}
	if s.fraction {
		// the digits right before the slash are the numerator; an integer
		// part is separated from them by something else
		last := -1
		for i, t := range s.tokens {
			if t.kind == tokSlash {
				last = i
				break
			}
		}
		for i := last - 1; i >= 0 && s.tokens[i].kind == tokDigit; i-- {
			s.tokens[i].part = 'n'
		}
	}
	for _, t := range s.tokens {
		switch {
		case t.kind == tokDigit && t.part == 'i':
			s.intDigits++
		case t.kind == tokDigit && t.part == 'f':
			s.fracDigits++
		}
	}
	for i := range s.tokens {
		t := &s.tokens[i]
		if t.kind != tokDate || t.text[0] != 'm' || len(t.text) > 2 {
			continue
		}
		prev, next := s.nearDate(i, -1), s.nearDate(i, 1)
		if prev != "" && (prev[0] == 'h' || strings.HasPrefix(prev, "[h")) || next != "" && (next[0] == 's' || strings.HasPrefix(next, "[s")) {
			t.text = strings.ToUpper(t.text)
		}
	}
}

// nearDate is the nearest date token before or after the i-th.
func (s *fmtSection) nearDate(i, step int) string {
	for j := i + step; j >= 0 && j < len(s.tokens); j += step {
		if s.tokens[j].kind == tokDate && s.tokens[j].text[0] != '.' {
			return s.tokens[j].text
		}
	}
	return ""
}

// IsDate tells whether the format writes dates or times.
func (f *Format) IsDate() bool {
	return f.sections[0].date
}

// section is the section that writes n, and whether n is written without
// its sign: the second section writes negative numbers, the third zero.
// Conditions such as [>=100] choose among the first two otherwise.
func (f *Format) section(n float64) (*fmtSection, bool) {
	s := f.sections
	if s[0].cond != "" || len(s) > 1 && s[1].cond != "" {
		for _, sec := range s[:min(len(s), 2)] {
			if sec.cond != "" && cmpOp(sec.cond, compareFloat(n, sec.condValue)) {
				return sec, false
			}
		}
		for _, sec := range s[:min(len(s), 3)] {
			if sec.cond == "" && !sec.text {
				return sec, false
			}
		}
		return s[0], false
	}
	switch {
	case n < 0 && len(s) > 1 && !s[1].text:
		return s[1], true
	case n == 0 && len(s) > 2 && !s[2].text:
		return s[2], false
	}
	return s[0], false
}

// Format writes a number.
func (l *Locale) Format(n float64, f *Format, date1904 bool) string {
	s, unsigned := f.section(n)
	if unsigned {
		n = math.Abs(n)
	}
	switch {
	case s.general || s.text && !s.hasDigit():
		out := formatGeneral(n)
		out = strings.Replace(out, ".", l.Decimal, 1)
		return l.literals(s, out)
	case s.date:
		if n < 0 {
			return strings.Repeat("#", 11)
		}
		return l.formatDate(n, s, date1904)
	}
	return l.formatNumber(n, s, len(f.sections) == 1 || s == f.sections[0] && n >= 0)
}

// FormatText writes text: in the fourth section, or the first when it
// holds @; as it is otherwise.
func (l *Locale) FormatText(t string, f *Format) string {
	var s *fmtSection
	switch {
	case len(f.sections) == 4:
		s = f.sections[3]
	case f.sections[0].text:
		s = f.sections[0]
	default:
		return t
	}
	var b strings.Builder
	for _, tok := range s.tokens {
		switch tok.kind {
		case tokText:
			b.WriteString(t)
		case tokLiteral:
			b.WriteString(tok.text)
		}
	}
	return b.String()
}

// literals is text written around a number of General format.
func (l *Locale) literals(s *fmtSection, number string) string {
	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokGeneral, tokText:
			b.WriteString(number)
		case tokLiteral:
			b.WriteString(t.text)
		}
	}
	return b.String()
}

func (l *Locale) formatNumber(n float64, s *fmtSection, signed bool) string {
	for range s.percent {
		n *= 100
	}
	for range s.scale {
		n /= 1000
	}
	neg := n < 0 && signed
	n = math.Abs(n)

	var intStr, fracStr, expStr string
	var num, den int64
	switch {
	case s.exp:
		intStr, fracStr, expStr = scientific(n, max(s.intDigits, 1), s.fracDigits)
	case s.fraction:
		var whole float64
		whole, num, den = fraction(n, s)
		if whole > 0 {
			intStr = strconv.FormatFloat(whole, 'f', 0, 64)
		}
	default:
		r := strconv.FormatFloat(roundWith(n, s.fracDigits, math.Round), 'f', s.fracDigits, 64)
		intStr, fracStr, _ = strings.Cut(r, ".")
	}
	intStr = strings.TrimLeft(intStr, "0")
	if neg && strings.Trim(intStr+fracStr, "0") == "" && num == 0 {
		neg = false
	}

	ints := l.integer(s, intStr)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	fi, ii := 0, 0
	done := map[byte]bool{}
	for _, t := range s.tokens {
		switch t.kind {
		case tokLiteral:
			if t.part == 'D' {
				b.WriteString(strconv.FormatInt(den, 10))
			} else {
				b.WriteString(t.text)
			}
		case tokDecimal:
			b.WriteString(l.Decimal)
		case tokSlash:
			if !done['-'] {
				b.WriteByte('/')
			}
		case tokExp:
			b.WriteString(t.text[:1])
			if strings.HasPrefix(expStr, "-") {
				b.WriteByte('-')
			} else if t.text[1] == '+' {
				b.WriteByte('+')
			}
		case tokDigit:
			switch t.part {
			case 'i':
				b.WriteString(ints[ii])
				ii++
			case 'f':
				b.WriteString(decimal(s, fracStr, fi))
				fi++
			default:
				if done[t.part] {
					continue
				}
				done[t.part] = true
				switch t.part {
				case 'e':
					b.WriteString(padDigits(strings.TrimPrefix(expStr, "-"), s.placeholders('e'), true))
				case 'n':
					if num == 0 && intStr != "" {
						// a whole number: the fraction is left blank
						b.WriteString(strings.Repeat(" ", len(s.placeholders('n'))+1+len(s.placeholders('d'))))
						done['d'], done['-'] = true, true
						continue
					}
					b.WriteString(padDigits(strconv.FormatInt(num, 10), s.placeholders('n'), true))
				case 'd':
					b.WriteString(padDigits(strconv.FormatInt(den, 10), s.placeholders('d'), false))
				}
			}
		}
	}
	return b.String()
}

// placeholders are the digit placeholders of a part of the number.
func (s *fmtSection) placeholders(part byte) string {
	var b strings.Builder
	for _, t := range s.tokens {
		if t.kind == tokDigit && t.part == part {
			b.WriteString(t.text)
		}
	}
	return b.String()
}

// integer is what each placeholder of the integer part writes: its digit,
// padding for a digit the number does not have, and for the first one the
// digits beyond the placeholders too. Thousands are grouped among what
// all of them write.
func (l *Locale) integer(s *fmtSection, digits string) []string {
	ph := s.placeholders('i')
	k := len(ph)
	out := make([]string, k)
	for j := range k {
		pos := len(digits) - (k - j)
		switch {
		case j == 0 && pos > 0:
			out[j] = digits[:pos+1]
		case pos >= 0:
			out[j] = digits[pos : pos+1]
		case ph[j] == '0':
			out[j] = "0"
		case ph[j] == '?':
			out[j] = " "
		}
	}
	if !s.thousands {
		return out
	}
	// the digits written, counted from the right
	count := 0
	for j := range k {
		count += len(strings.TrimSpace(out[j]))
	}
	seen := 0
	for j := range k {
		var b strings.Builder
		for i := range len(out[j]) {
			b.WriteByte(out[j][i])
			if out[j][i] == ' ' {
				continue
			}
			seen++
			if left := count - seen; left > 0 && left%3 == 0 {
				b.WriteString(l.Groups[0])
			}
		}
		out[j] = b.String()
	}
	return out
}

// decimal is what the i-th placeholder of the decimals writes: "#" and
// "?" leave out the zeros that end the decimals, "?" as spaces.
func decimal(s *fmtSection, digits string, i int) string {
	ph := s.placeholders('f')
	d := byte('0')
	if i < len(digits) {
		d = digits[i]
	}
	if ph[i] == '0' || strings.Trim(digits[min(i, len(digits)):], "0") != "" {
		return string(d)
	}
	if ph[i] == '?' {
		return " "
	}
	return ""
}

// padDigits pads digits to their placeholders, on the left or the right:
// "0" with zeros, "?" with spaces, "#" with nothing.
func padDigits(digits, ph string, left bool) string {
	var pad strings.Builder
	for i := len(digits); i < len(ph); i++ {
		c := ph[len(ph)-1-i]
		if !left {
			c = ph[i]
		}
		switch c {
		case '0':
			pad.WriteByte('0')
		case '?':
			pad.WriteByte(' ')
		}
	}
	if left {
		return pad.String() + digits
	}
	return digits + pad.String()
}

// scientific writes n with intDigits before the decimal point and
// fracDigits after it, and its exponent.
func scientific(n float64, intDigits, fracDigits int) (string, string, string) {
	if n == 0 {
		return strings.Repeat("0", intDigits), strings.Repeat("0", fracDigits), "0"
	}
	exp := int(math.Floor(math.Log10(n)))
	// engineering notation: ##0.0E+0 keeps exponents multiple of 3
	if intDigits > 1 {
		exp -= ((exp % intDigits) + intDigits) % intDigits
	} else {
		exp -= intDigits - 1
	}
	m := n / math.Pow(10, float64(exp))
	r := strconv.FormatFloat(roundWith(m, fracDigits, math.Round), 'f', fracDigits, 64)
	if v, _ := strconv.ParseFloat(r, 64); v >= math.Pow(10, float64(intDigits)) {
		exp += max(intDigits, 1)
		m = n / math.Pow(10, float64(exp))
		r = strconv.FormatFloat(roundWith(m, fracDigits, math.Round), 'f', fracDigits, 64)
	}
	i, f, _ := strings.Cut(r, ".")
	return i, f, strconv.Itoa(exp)
}

// fraction is n as a whole number and a fraction whose denominator has
// as many digits as the format gives it, or the one it fixes.
func fraction(n float64, s *fmtSection) (float64, int64, int64) {
	whole := 0.0
	if s.intDigits > 0 {
		whole = math.Floor(n)
		n -= whole
	}
	if s.denominator > 0 {
		d := int64(s.denominator)
		num := int64(math.Round(n * float64(d)))
		if s.intDigits > 0 && num == d {
			return whole + 1, 0, d
		}
		return whole, num, d
	}
	digits := 0
	for _, t := range s.tokens {
		if t.kind == tokDigit && t.part == 'd' {
			digits++
		}
	}
	// Excel reads denominators of 5 digits at most
	limit := int64(math.Pow(10, float64(min(max(digits, 1), 5)))) - 1
	bestN, bestD, bestErr := int64(0), int64(1), math.Inf(1)
	for d := int64(1); d <= limit; d++ {
		num := int64(math.Round(n * float64(d)))
		if e := math.Abs(n - float64(num)/float64(d)); e < bestErr-1e-12 {
			bestN, bestD, bestErr = num, d, e
		}
	}
	if s.intDigits > 0 && bestN == bestD {
		return whole + 1, 0, 1
	}
	return whole, bestN, bestD
}

var (
	frenchShortMonths  = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}
	englishShortMonths = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	frenchDays         = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	englishDays        = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

func (l *Locale) formatDate(n float64, s *fmtSection, date1904 bool) string {
	// round to the precision shown: the second, or its decimals
	unit := 86400 * math.Pow(10, float64(s.secDigits))
	n = math.Round(n*unit) / unit
	day := dayOf(n, date1904)
	frac := n - math.Floor(n)
	secs := frac * 86400
	h := int(secs / 3600)
	mi := int(secs/60) % 60
	sec := math.Mod(secs, 60)
	french := l.Decimal == ","
	c := &Context{Date1904: date1904}
	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokLiteral:
			b.WriteString(t.text)
		case tokDecimal:
			b.WriteString(l.Decimal)
		case tokDate:
			switch t.text {
			case "y", "yy":
				b.WriteString(pad2(day.y % 100))
			case "yyy", "yyyy":
				b.WriteString(strconv.Itoa(day.y))
			case "m":
				b.WriteString(strconv.Itoa(day.m))
			case "mm":
				b.WriteString(pad2(day.m))
			case "mmm":
				if french {
					b.WriteString(frenchShortMonths[day.m-1])
				} else {
					b.WriteString(englishShortMonths[day.m-1])
				}
			case "mmmmm":
				if french {
					b.WriteString(strings.ToUpper(frenchMonths[day.m-1][:1]))
				} else {
					b.WriteString(englishShortMonths[day.m-1][:1])
				}
			case "d":
				b.WriteString(strconv.Itoa(day.d))
			case "dd":
				b.WriteString(pad2(day.d))
			case "ddd":
				w := c.weekday(n)
				if french {
					b.WriteString(frenchDays[w][:3] + ".")
				} else {
					b.WriteString(englishDays[w][:3])
				}
			case "h", "hh":
				hh := h
				if s.ampm {
					hh = h % 12
					if hh == 0 {
						hh = 12
					}
				}
				if t.text == "hh" {
					b.WriteString(pad2(hh))
				} else {
					b.WriteString(strconv.Itoa(hh))
				}
			case "M":
				b.WriteString(strconv.Itoa(mi))
			case "MM":
				b.WriteString(pad2(mi))
			case "s":
				b.WriteString(strconv.Itoa(int(sec)))
			case "ss":
				b.WriteString(pad2(int(sec)))
			case "[h]", "[hh]":
				b.WriteString(strconv.Itoa(int(math.Floor(n * 24))))
			case "[m]", "[mm]":
				b.WriteString(strconv.Itoa(int(math.Floor(n * 1440))))
			case "[s]", "[ss]":
				b.WriteString(strconv.Itoa(int(math.Floor(n * 86400))))
			case "AM/PM":
				if h < 12 {
					b.WriteString("AM")
				} else {
					b.WriteString("PM")
				}
			case "A/P":
				if h < 12 {
					b.WriteString("A")
				} else {
					b.WriteString("P")
				}
			default:
				switch {
				case strings.HasPrefix(t.text, "."):
					d := strconv.FormatFloat(sec-math.Floor(sec), 'f', s.secDigits, 64)
					b.WriteString(l.Decimal + d[strings.IndexByte(d, '.')+1:])
				case strings.HasPrefix(t.text, "mmmm"):
					if french {
						b.WriteString(frenchMonths[day.m-1])
					} else {
						b.WriteString(strings.ToUpper(englishMonths[day.m-1][:1]) + englishMonths[day.m-1][1:])
					}
				case strings.HasPrefix(t.text, "dddd"):
					w := c.weekday(n)
					if french {
						b.WriteString(frenchDays[w])
					} else {
						b.WriteString(englishDays[w])
					}
				case strings.HasPrefix(t.text, "yyy"):
					b.WriteString(strconv.Itoa(day.y))
				case strings.HasPrefix(t.text, "e"):
					b.WriteString(strconv.Itoa(day.y))
				case strings.HasPrefix(t.text, "MM"):
					b.WriteString(pad2(mi))
				case strings.HasPrefix(t.text, "hh"):
					b.WriteString(pad2(h))
				case strings.HasPrefix(t.text, "ss"):
					b.WriteString(pad2(int(sec)))
				case strings.HasPrefix(t.text, "dd"):
					b.WriteString(pad2(day.d))
				}
			}
		case tokDigit:
			b.WriteString(t.text)
		}
	}
	return b.String()
}

func pad2(n int) string {
	if n < 10 && n >= 0 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
