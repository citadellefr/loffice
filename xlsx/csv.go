package xlsx

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/charset"
	"github.com/citadellefr/loffice/ot"
)

// CSV is a file of values separated by semicolons, commas or tabs, read as
// a workbook of one sheet, "S1", whose book has the attribute "csv". Its
// fields are read as the French version of Excel reads them, except that
// numbers written with leading zeros stay text: postcodes and phone
// numbers are not lost.
//
// It is written back as it was read: separator, quotes, encoding, line
// endings and the "sep=" line Excel understands. Only values are written,
// in their number format. A cell whose field would not be written back as
// it was read keeps it in "src", written again as long as the cell's value
// and format are those it gives: a file saved unedited is the same.
type CSV struct {
	doc    *Document
	sep    byte
	locale *formula.Locale
	now    time.Time
	bom    bool
	ansi   bool // read as Windows-1252
	// newline is the line break of the file; lines with another keep it
	// in the field "eol" of their row, as those with more fields than
	// cells keep their number in "n".
	newline string
	final   bool // the last line ends with a line break
	header  string
	// width is the number of fields of every line, when all have as many.
	width int
	// quoteAll is set when every field holding text was quoted, and
	// quoteEmpty when every empty one was.
	quoteAll, quoteEmpty bool
	// blank are the lines without cells at the end.
	blank int
	// formats are the number formats read, by code.
	formats map[string]*formula.Format
}

var (
	ErrCSVSheets = errors.New("xlsx: a CSV file holds a single sheet")
	ErrCSVSize   = errors.New("xlsx: the CSV file does not fit in a sheet")
)

const csvSheet = "S1"

var bom = []byte{0xEF, 0xBB, 0xBF}

type csvField struct {
	raw, text string
	quoted    bool
}

type csvLine struct {
	fields []csvField
	eol    string
}

// OpenCSV reads a CSV file, its sheet named after the file as Excel does.
func OpenCSV(data []byte, name string) (*CSV, *ot.Tree, error) {
	c := &CSV{now: time.Now(), newline: "\r\n", formats: map[string]*formula.Format{}}
	data, c.bom = bytes.CutPrefix(data, bom)
	text := string(data)
	if !c.bom && !utf8.Valid(data) {
		c.ansi, text = true, charset.Decode(data)
	}
	if len(text) >= 5 && strings.EqualFold(text[:4], "sep=") {
		line, rest, _ := strings.Cut(text, "\n")
		c.header, c.sep, text = line, line[4], rest
	} else {
		c.sep = separator(text)
	}
	c.locale = formula.French
	if c.sep == ',' {
		l := *formula.French
		l.Decimal = "."
		c.locale = &l
	}

	lines := c.records(text)
	if len(lines) > formula.MaxRows {
		return nil, nil, ErrCSVSize
	}
	if len(lines) > 0 {
		if eol := lines[0].eol; eol != "" {
			c.newline = eol
		}
		c.final = lines[len(lines)-1].eol != ""
	}
	c.quoteAll, c.quoteEmpty = true, true
	texts, empties := 0, 0
	for i, line := range lines {
		if len(line.fields) > formula.MaxCols {
			return nil, nil, ErrCSVSize
		}
		if i == 0 {
			c.width = len(line.fields)
		} else if len(line.fields) != c.width {
			c.width = 0
		}
		for _, f := range line.fields {
			if f.text == "" {
				empties++
				c.quoteEmpty = c.quoteEmpty && f.quoted
			} else {
				texts++
				c.quoteAll = c.quoteAll && f.quoted
			}
		}
	}
	c.quoteAll = c.quoteAll && texts > 0
	c.quoteEmpty = c.quoteEmpty && empties > 0

	formats := map[string]string{}
	nodes := ot.Edit{{Op: ot.OpNew, ID: "book", Type: "book", Key: "V", Attrs: ot.Values{"csv": json.RawMessage("true")}}}
	cells := []ot.Cell{}
	var b []byte
	for i, line := range lines {
		last := 0
		for j, f := range line.fields {
			v, code := c.value(f.text)
			src := ""
			if c.quote(c.write(v, code), v.Type == formula.TypeText) != f.raw {
				src = f.raw
			}
			if v.Type == formula.TypeBlank && src == "" {
				continue
			}
			var xf string
			if code != "" {
				if xf = formats[code]; xf == "" {
					xf = xfID(len(formats))
					formats[code] = xf
					nodes = append(nodes, ot.Change{Op: ot.OpNew, ID: xf, Type: "xf", Key: "V", Attrs: ot.Values{"style": mustJSON(Style{Format: code})}})
				}
			}
			b = csvCell(b[:0], v, xf, src)
			cells = append(cells, ot.Cell{Row: i + 1, Col: j + 1, Fields: bytes.Clone(b)})
			last = j + 1
		}
		row := map[string]any{}
		if n := len(line.fields); c.width == 0 && n > max(last, 1) {
			row["n"] = n
		}
		if line.eol != "" && line.eol != c.newline {
			row["eol"] = line.eol
		}
		if len(row) > 0 {
			cells = append(cells, ot.Cell{Row: i + 1, Col: 0, Fields: mustJSON(row)})
		}
		if last == 0 {
			c.blank++
		} else {
			c.blank = 0
		}
	}
	slices.SortFunc(cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	sheet := strings.TrimSuffix(path.Base(name), path.Ext(name))
	sheet = sheetName(sheet, 0, map[string]bool{})
	nodes = append(nodes, ot.Change{Op: ot.OpNew, ID: csvSheet, Type: "sheet", Parent: "book", Key: "V",
		Attrs: ot.Values{"name": mustJSON(sheet)}, Cells: cells})
	tree, err := ot.NewTree(nodes)
	if err != nil {
		return nil, nil, err
	}
	c.doc = &Document{loaded: tree.Clone(), csv: true}
	return c, tree, nil
}

// separator is the one the first line has most of, a semicolon when it
// has none, as the French version of Excel writes.
func separator(text string) byte {
	counts := map[byte]int{}
	quoted := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '"' {
			quoted = !quoted
		}
		if !quoted && (c == '\n' || c == '\r') {
			break
		}
		if !quoted && (c == ';' || c == '\t' || c == ',') {
			counts[c]++
		}
	}
	best := byte(';')
	for _, c := range []byte{'\t', ','} {
		if counts[c] > counts[best] {
			best = c
		}
	}
	return best
}

// records are the lines of the file, cut into fields, with their line
// breaks. A quoted field may hold separators, line breaks and doubled
// quotes; what follows its closing quote is kept, as Excel does.
func (c *CSV) records(text string) []csvLine {
	var lines []csvLine
	i := 0
	for i < len(text) {
		var line csvLine
		for {
			start := i
			f := csvField{}
			if i < len(text) && text[i] == '"' {
				var b strings.Builder
				for i++; i < len(text); i++ {
					if text[i] == '"' {
						if i+1 < len(text) && text[i+1] == '"' {
							b.WriteByte('"')
							i++
							continue
						}
						i++
						break
					}
					b.WriteByte(text[i])
				}
				for ; i < len(text) && text[i] != c.sep && text[i] != '\n' && text[i] != '\r'; i++ {
					b.WriteByte(text[i])
				}
				f.text, f.quoted = b.String(), true
			} else {
				for i < len(text) && text[i] != c.sep && text[i] != '\n' && text[i] != '\r' {
					i++
				}
				f.text = text[start:i]
			}
			f.raw = text[start:i]
			line.fields = append(line.fields, f)
			if i < len(text) && text[i] == c.sep {
				i++
				continue
			}
			break
		}
		start := i
		if i < len(text) && text[i] == '\r' {
			i++
		}
		if i < len(text) && text[i] == '\n' {
			i++
		}
		line.eol = text[start:i]
		lines = append(lines, line)
	}
	return lines
}

// csvCell writes the fields of a cell, keys in order.
func csvCell(b []byte, v formula.Value, xf, src string) []byte {
	b = append(b, '{')
	if v.Type == formula.TypeError {
		b = append(b, `"e":`...)
		b = appendString(b, v.Str)
	}
	if xf != "" {
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, `"s":`...)
		b = appendString(b, xf)
	}
	if src != "" {
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, `"src":`...)
		b = appendString(b, src)
	}
	if v.Type != formula.TypeError && v.Type != formula.TypeBlank {
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, `"v":`...)
		switch v.Type {
		case formula.TypeNumber:
			b = strconv.AppendFloat(b, v.Num, 'g', -1, 64)
		case formula.TypeBool:
			b = strconv.AppendBool(b, v.Num != 0)
		default:
			b = appendString(b, v.Str)
		}
	}
	return append(b, '}')
}

// frenchErrors are the errors as the French version writes them.
var frenchErrors = map[string]string{"#NULL!": "#NUL!", "#VALUE!": "#VALEUR!", "#NAME?": "#NOM?", "#NUM!": "#NOMBRE!"}

// words are the booleans and errors, as read in either language.
var words = map[string]formula.Value{
	"VRAI": formula.Boolean(true), "TRUE": formula.Boolean(true), "FAUX": formula.Boolean(false), "FALSE": formula.Boolean(false),
	"#NULL!": formula.Err("#NULL!"), "#NUL!": formula.Err("#NULL!"), "#DIV/0!": formula.Err("#DIV/0!"),
	"#VALUE!": formula.Err("#VALUE!"), "#VALEUR!": formula.Err("#VALUE!"), "#REF!": formula.Err("#REF!"),
	"#NAME?": formula.Err("#NAME?"), "#NOM?": formula.Err("#NAME?"), "#NUM!": formula.Err("#NUM!"),
	"#NOMBRE!": formula.Err("#NUM!"), "#N/A": formula.Err("#N/A"),
}

// value reads a field as the French version of Excel reads what is typed
// into a cell, and gives the number format that shows it as written.
// Formulas stay text: a file is data, not code to run.
func (c *CSV) value(s string) (formula.Value, string) {
	if s == "" {
		return formula.Value{}, ""
	}
	t := strings.TrimSpace(s)
	if len(t) <= len("#NOMBRE!") {
		if v, ok := words[strings.ToUpper(t)]; ok {
			return v, ""
		}
	}
	if p, ok := strings.CutSuffix(t, "%"); ok {
		if n, ok := c.number(strings.TrimSpace(p)); ok {
			code := "0%"
			if d := c.decimals(t); d > 0 {
				code = "0." + strings.Repeat("0", d) + "%"
			}
			return formula.Num(n / 100), code
		}
	}
	if strings.HasPrefix(t, "€") || strings.HasSuffix(t, "€") {
		if n, ok := c.number(strings.TrimSpace(strings.ReplaceAll(t, "€", ""))); ok {
			return formula.Num(n), `#,##0.00 "€"`
		}
	}
	if n, ok := c.number(t); ok {
		if grouped(t) {
			return formula.Num(n), "#,##0"
		}
		return formula.Num(n), ""
	}
	if n, code, ok := c.date(t); ok {
		return formula.Num(n), code
	}
	return formula.Str(s), ""
}

var spaces = strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "")

// number reads a number written in the file's locale, digits grouped by
// spaces or not; one written with leading zeros is not.
func (c *CSV) number(t string) (float64, bool) {
	if t == "" || !strings.ContainsRune("+-.,0123456789", rune(t[0])) {
		return 0, false
	}
	s := t
	if strings.ContainsAny(s, " \u00a0\u202f") {
		s = spaces.Replace(s)
	}
	if c.locale.Decimal == "," {
		if strings.Contains(s, ".") {
			return 0, false
		}
		s = strings.ReplaceAll(s, ",", ".")
	} else {
		s = strings.ReplaceAll(s, ",", "")
	}
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	digits := func() int {
		start := i
		for i < len(s) && '0' <= s[i] && s[i] <= '9' {
			i++
		}
		return i - start
	}
	whole := i
	n := digits()
	if n > 1 && s[whole] == '0' {
		return 0, false
	}
	if i < len(s) && s[i] == '.' {
		i++
		n += digits()
	}
	if n == 0 {
		return 0, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '-' || s[i] == '+') {
			i++
		}
		if digits() == 0 {
			return 0, false
		}
	}
	if i != len(s) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && !math.IsInf(v, 0)
}

// grouped tells whether a number was written with its thousands grouped.
func grouped(t string) bool {
	r := []rune(t)
	for i := 1; i+3 < len(r); i++ {
		if (r[i] == ' ' || r[i] == '\u00a0' || r[i] == '\u202f') && isDigit(r[i-1]) && isDigit(r[i+1]) && isDigit(r[i+2]) && isDigit(r[i+3]) {
			return true
		}
	}
	return false
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

func (c *CSV) decimals(t string) int {
	_, after, ok := strings.Cut(t, c.locale.Decimal)
	if !ok {
		return 0
	}
	n := 0
	for n < len(after) && '0' <= after[n] && after[n] <= '9' {
		n++
	}
	return n
}

var (
	dayPattern  = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})(?:[/-](\d{2,4}))?(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?$`)
	timePattern = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
)

// date reads a date, a time or both, day first, and gives the format that
// shows them as written.
func (c *CSV) date(t string) (float64, string, bool) {
	if m := dayPattern.FindStringSubmatch(t); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y := c.now.Year()
		code := "d-mmm"
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
			if len(m[3]) == 2 {
				if y < 30 {
					y += 2000
				} else {
					y += 1900
				}
			}
			code = "dd/mm/yyyy"
		}
		day := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		if mo < 1 || mo > 12 || day.Day() != d {
			return 0, "", false
		}
		n := formula.Serial(day, false)
		if m[4] != "" {
			tm, ok := timeOf(m[4], m[5], m[6])
			if !ok {
				return 0, "", false
			}
			n += tm
			code = "dd/mm/yyyy hh:mm"
		}
		return n, code, true
	}
	if m := timePattern.FindStringSubmatch(t); m != nil {
		tm, ok := timeOf(m[1], m[2], m[3])
		if !ok {
			return 0, "", false
		}
		if m[3] == "" {
			return tm, "hh:mm", true
		}
		return tm, "hh:mm:ss", true
	}
	return 0, "", false
}

func timeOf(hours, minutes, seconds string) (float64, bool) {
	h, _ := strconv.Atoi(hours)
	m, _ := strconv.Atoi(minutes)
	s, _ := strconv.Atoi(seconds)
	if h > 23 || m > 59 || s > 59 {
		return 0, false
	}
	return float64(h*3600+m*60+s) / 86400, true
}

// write is a value as the file holds it: in its number format, in the
// file's locale.
func (c *CSV) write(v formula.Value, code string) string {
	switch v.Type {
	case formula.TypeNumber:
		if code == "" {
			code = "General"
		}
		f := c.formats[code]
		if f == nil {
			f = formula.ParseFormat(code)
			c.formats[code] = f
		}
		return c.locale.Format(v.Num, f, false)
	case formula.TypeBool:
		if v.Num != 0 {
			return c.locale.True
		}
		return c.locale.False
	case formula.TypeError:
		if f, ok := frenchErrors[v.Str]; ok {
			return f
		}
	}
	return v.Str
}

// quote writes a field, quoted when it has to be or when the file quotes
// all of its text.
func (c *CSV) quote(s string, text bool) string {
	if s == "" {
		if c.quoteEmpty {
			return `""`
		}
		return ""
	}
	if c.quoteAll && text || strings.ContainsAny(s, "\"\r\n") || strings.IndexByte(s, c.sep) >= 0 {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// Check tells whether an edit only changes what a CSV file can hold as a
// workbook: its one sheet, and formats.
func (c *CSV) Check(tree *ot.Tree, e ot.Edit) error {
	for _, ch := range e {
		if ch.Op == ot.OpNew && ch.Type == "sheet" || ch.Op == ot.OpDel && ch.ID == csvSheet {
			return ErrCSVSheets
		}
	}
	return c.doc.Check(tree, e)
}

// Save writes the sheet back as a CSV file.
func (c *CSV) Save(tree *ot.Tree) ([]byte, error) {
	sheet := tree.Node(csvSheet)
	if sheet == nil || sheet.Grid == nil {
		return nil, ErrCSVSheets
	}
	formats := map[string]string{}
	for _, n := range tree.Children("") {
		if n.Type == "xf" {
			var st Style
			_ = json.Unmarshal(n.Attrs["style"], &st)
			formats[n.ID] = st.Format
		}
	}
	rows, cols := 0, 0
	sheet.Grid.Each(1, formula.MaxRows, func(row, col int, _ json.RawMessage) bool {
		if col > 0 {
			rows, cols = row, max(cols, col)
		}
		return true
	})
	empty := c.quote("", false)
	var b strings.Builder
	if c.header != "" {
		b.WriteString(c.header)
		b.WriteByte('\n')
	}
	lines := rows + c.blank
	for r := 1; r <= lines; r++ {
		var fields []string
		var line struct {
			N   int    `json:"n"`
			EOL string `json:"eol"`
		}
		sheet.Grid.EachIn(r, r, 0, formula.MaxCols, func(_, col int, raw json.RawMessage) bool {
			if col == 0 {
				_ = json.Unmarshal(raw, &line)
				return true
			}
			for len(fields) < col-1 {
				fields = append(fields, empty)
			}
			fields = append(fields, c.field(raw, formats))
			return true
		})
		for len(fields) > 0 && fields[len(fields)-1] == empty {
			fields = fields[:len(fields)-1]
		}
		n := line.N
		if c.width > 0 {
			n = max(c.width, cols)
		}
		for len(fields) < n {
			fields = append(fields, empty)
		}
		b.WriteString(strings.Join(fields, string(c.sep)))
		switch {
		case line.EOL != "":
			b.WriteString(line.EOL)
		case r < lines || c.final:
			b.WriteString(c.newline)
		}
	}
	text := b.String()
	var out []byte
	if c.ansi {
		// the narrow space that groups thousands, which Windows-1252 lacks
		if data, ok := charset.Encode(strings.ReplaceAll(text, "\u202f", "\u00a0")); ok {
			return data, nil
		}
		out = append(out, bom...)
	} else if c.bom {
		out = append(out, bom...)
	}
	return append(out, text...), nil
}

// field is the text of a cell: the one it was read from while its value
// and format are those that text gives.
func (c *CSV) field(raw json.RawMessage, formats map[string]string) string {
	var f struct {
		V   json.RawMessage `json:"v"`
		E   string          `json:"e"`
		S   string          `json:"s"`
		Src string          `json:"src"`
	}
	_ = json.Unmarshal(raw, &f)
	v := valueOf(raw)
	code := formats[f.S]
	if f.Src != "" {
		if src := c.records(f.Src); len(src) == 1 && len(src[0].fields) == 1 && src[0].fields[0].raw == f.Src {
			if w, wcode := c.value(src[0].fields[0].text); w.Type == v.Type && w.Num == v.Num && w.Str == v.Str && wcode == code {
				return f.Src
			}
		}
	}
	if v.Type == formula.TypeBlank {
		return c.quote("", false)
	}
	return c.quote(c.write(v, code), v.Type == formula.TypeText)
}
