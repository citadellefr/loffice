package pagination

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/opc"
)

const (
	relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relExtended       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"
)

// Doc is what pagination needs from a docx.
type Doc struct {
	Body []Block
	// Final is the section of the last blocks; earlier sections end at the
	// paragraph that carries them.
	Final *Section
	// WordPages is the page count Word saved in docProps/app.xml, 0 if none.
	WordPages int
	// FromWord tells whether Word saved the document, which makes WordPages
	// and the page breaks it recorded a reference.
	FromWord   bool
	DefaultTab float64
	// Unsupported lists the features met that the prototype does not lay
	// out, which may explain a difference with Word.
	Unsupported map[string]bool
	// Paragraphs counts paragraphs, those of tables included.
	Paragraphs int
}

type Block any

type Para struct {
	Index int
	Style string
	Props paraProps
	Items []Item
	// Mark is the run style of the paragraph mark, which gives an empty
	// paragraph its height.
	Mark    *RunStyle
	Section *Section
	// WordBreaks are the offsets at which Word's last layout broke a page
	// inside or before this paragraph.
	WordBreaks []int
	label      int // length of the numbering label
}

type paraProps struct {
	Before, After       float64 // pt
	Line                float64 // a factor for LineRule "auto", else pt
	LineRule            string
	Left, Right, First  float64 // pt; First is negative for a hanging indent
	KeepNext, KeepLines bool
	BreakBefore, Widow  bool
	Contextual          bool
	AutoBefore          bool
	AutoAfter           bool
	Tabs                []float64
}

type ItemKind uint8

const (
	TextItem ItemKind = iota
	TabItem
	LineBreak
	PageBreak
	ColumnBreak
	ObjectItem
)

type Item struct {
	Kind ItemKind
	Text string
	Run  *RunStyle
	W, H float64 // an inline object, in pt
}

type RunStyle struct {
	Font    *Font
	Size    float64 // pt
	Spacing float64 // pt added after every character
}

type Table struct {
	Rows []Row
}

type Row struct {
	Cells []Cell
	// Spacing is the space between cells, and between them and the table's
	// edges, in pt.
	Spacing   float64
	Height    float64
	Exact     bool
	CantSplit bool
}

type Cell struct {
	Width       float64 // pt, text area
	Top, Bottom float64
	// BorderTop and BorderBottom are the widths of its borders, in pt.
	BorderTop, BorderBottom float64
	Blocks                  []Block
	MergedFromAbove         bool
}

type Section struct {
	Width, Height            float64
	Top, Bottom, Left, Right float64
	HeaderDist, FooterDist   float64
	Type                     string
	TitlePage                bool
	Header, FirstHeader      []Block
	Footer, FirstFooter      []Block
	raw                      *node
}

// props is a stack of pPr or rPr elements, highest priority first.
type props []*node

func (p props) elem(name string) *node {
	for _, n := range p {
		if c := n.child(name); c != nil {
			return c
		}
	}
	return nil
}

func (p props) attr(elem, attr string) (float64, bool) {
	for _, n := range p {
		if v, ok := n.child(elem).num(attr); ok {
			return v, true
		}
	}
	return 0, false
}

func (p props) on(elem string) bool {
	c := p.elem(elem)
	return c != nil && c.on()
}

type loader struct {
	pkg          *opc.Package
	doc          *Doc
	fonts        *FontSet
	styles       map[string]*node
	defaultPara  string
	defaultTable string
	gutterAtTop  bool
	docDefaults  *node
	theme        map[string]string
	numbering    map[string]*node // numId → abstractNum
	fields       []bool           // open complex fields, true while in their instructions
	paraCount    int
}

func Load(data []byte, fonts *FontSet) (*Doc, error) {
	pkg, err := opc.Open(data, opc.Limits{})
	if err != nil {
		return nil, err
	}
	l := &loader{pkg: pkg, fonts: fonts, doc: &Doc{DefaultTab: 36, Unsupported: map[string]bool{}},
		styles: map[string]*node{}, theme: map[string]string{}, numbering: map[string]*node{}}
	main, err := l.target("", relOfficeDocument)
	if err != nil {
		return nil, err
	}
	if app, err := l.target("", relExtended); err == nil {
		if n, err := l.parse(app); err == nil {
			l.doc.WordPages, _ = strconv.Atoi(strings.TrimSpace(n.find("Properties", "Pages").content()))
			l.doc.FromWord = strings.Contains(n.find("Properties", "Application").content(), "Microsoft")
		}
	}
	rels, err := pkg.Relationships(main)
	if err != nil {
		return nil, err
	}
	parts := map[string]string{}
	for _, r := range rels {
		if t, err := opc.Resolve(main, r.Target); err == nil && !r.External {
			parts[path.Base(r.Type)] = t
			parts[r.ID] = t
		}
	}
	l.loadStyles(parts["styles"])
	l.loadTheme(parts["theme"])
	l.loadNumbering(parts["numbering"])
	l.loadSettings(parts["settings"])
	root, err := l.parse(main)
	if err != nil {
		return nil, err
	}
	body := root.find("document", "body")
	if body == nil {
		return nil, fmt.Errorf("no body")
	}
	l.doc.Body = l.blocks(body, nil)
	l.doc.Paragraphs = l.paraCount
	l.doc.Final = l.section(body.child("sectPr"), parts)
	for _, b := range l.doc.Body {
		if p, ok := b.(*Para); ok && p.Section != nil {
			p.Section = l.section(p.Section.raw, parts)
		}
	}
	return l.doc, nil
}

func (l *loader) target(source, relType string) (string, error) {
	rels, err := l.pkg.Relationships(source)
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if r.Type == relType && !r.External {
			return opc.Resolve(source, r.Target)
		}
	}
	return "", fmt.Errorf("no %s", path.Base(relType))
}

func (l *loader) parse(name string) (*node, error) {
	if name == "" {
		return nil, fmt.Errorf("no part")
	}
	data, err := l.pkg.Read(name)
	if err != nil {
		return nil, err
	}
	return parseXML(data)
}

func (l *loader) loadStyles(name string) {
	root, err := l.parse(name)
	if err != nil {
		return
	}
	styles := root.child("styles")
	l.docDefaults = styles.child("docDefaults")
	for _, s := range styles.children() {
		if s.name != "style" {
			continue
		}
		id := s.attr("styleId")
		l.styles[id] = s
		if d := s.attr("default"); d == "1" || d == "true" || d == "on" {
			switch s.attr("type") {
			case "paragraph":
				l.defaultPara = id
			case "table":
				l.defaultTable = id
			}
		}
	}
}

func (l *loader) loadTheme(name string) {
	root, err := l.parse(name)
	if err != nil {
		return
	}
	scheme := root.find("theme", "themeElements", "fontScheme")
	l.theme["minor"] = scheme.find("minorFont", "latin").attr("typeface")
	l.theme["major"] = scheme.find("majorFont", "latin").attr("typeface")
}

func (l *loader) loadNumbering(name string) {
	root, err := l.parse(name)
	if err != nil {
		return
	}
	abstract := map[string]*node{}
	for _, n := range root.child("numbering").children() {
		switch n.name {
		case "abstractNum":
			abstract[n.attr("abstractNumId")] = n
		case "num":
			l.numbering[n.attr("numId")] = n
		}
	}
	for id, n := range l.numbering {
		l.numbering[id] = abstract[n.child("abstractNumId").attr("val")]
	}
}

func (l *loader) loadSettings(name string) {
	root, err := l.parse(name)
	if err != nil {
		return
	}
	settings := root.child("settings")
	if v, ok := settings.child("defaultTabStop").num("val"); ok && v > 0 {
		l.doc.DefaultTab = v / 20
	}
	l.gutterAtTop = settings.child("gutterAtTop") != nil && settings.child("gutterAtTop").on()
}

// styleChain lists the pPr or rPr of a style and those it is based on.
func (l *loader) styleChain(id, elem string) props {
	var out props
	for seen := 0; id != "" && seen < 20; seen++ {
		s := l.styles[id]
		if s == nil {
			break
		}
		if n := s.child(elem); n != nil {
			out = append(out, n)
		}
		id = s.child("basedOn").attr("val")
	}
	return out
}

// blocks reads body content; in a table, tableStyle names its style.
func (l *loader) blocks(parent *node, tableStyle *string) []Block {
	var out []Block
	for _, n := range parent.children() {
		switch n.name {
		case "p":
			out = append(out, l.para(n, tableStyle))
		case "tbl":
			out = append(out, l.table(n))
		case "sdt":
			out = append(out, l.blocks(n.child("sdtContent"), tableStyle)...)
		case "customXml":
			out = append(out, l.blocks(n, tableStyle)...)
		case "AlternateContent":
			out = append(out, l.blocks(n.child("Choice"), tableStyle)...)
		}
	}
	return out
}

// paraProps stacks the paragraph properties by priority: direct, numbering,
// style, then the table style above the default paragraph style, as Word
// does.
func (l *loader) paraProps(p *node, tableStyle *string) (props, props, string) {
	pPr := p.child("pPr")
	style := pPr.child("pStyle").attr("val")
	if l.styles[style] == nil {
		style = l.defaultPara
	}
	var pp, rp props
	if pPr != nil {
		pp = append(pp, pPr)
	}
	chain := func(elem string) props {
		own, deflt := l.split(style, elem)
		var out props
		out = append(out, own...)
		if tableStyle != nil {
			out = append(out, l.styleChain(*tableStyle, elem)...)
		}
		return append(out, deflt...)
	}
	styleP := chain("pPr")
	num := append(pp, styleP...).elem("numPr")
	if lvl := l.level(num); lvl != nil {
		if n := lvl.child("pPr"); n != nil {
			pp = append(pp, n)
		}
	}
	pp = append(pp, styleP...)
	rp = chain("rPr")
	if d := l.docDefaults.find("pPrDefault", "pPr"); d != nil {
		pp = append(pp, d)
	}
	if d := l.docDefaults.find("rPrDefault", "rPr"); d != nil {
		rp = append(rp, d)
	}
	return pp, rp, style
}

// split separates the chain of a style from that of the default paragraph
// style it rests on.
func (l *loader) split(style, elem string) (own, deflt props) {
	all := l.styleChain(style, elem)
	base := l.styleChain(l.defaultPara, elem)
	if len(base) <= len(all) && slices.Equal(all[len(all)-len(base):], base) {
		return all[:len(all)-len(base)], base
	}
	return all, nil
}

func (l *loader) level(numPr *node) *node {
	id := numPr.child("numId").attr("val")
	abstract := l.numbering[id]
	if abstract == nil || id == "0" {
		return nil
	}
	ilvl := numPr.child("ilvl").attr("val")
	if ilvl == "" {
		ilvl = "0"
	}
	for _, lvl := range abstract.children() {
		if lvl.name == "lvl" && lvl.attr("ilvl") == ilvl {
			return lvl
		}
	}
	return nil
}

func (l *loader) para(n *node, tableStyle *string) *Para {
	pp, rp, style := l.paraProps(n, tableStyle)
	p := &Para{Index: l.paraCount, Style: style, Props: resolvePara(pp)}
	l.paraCount++
	markProps := rp
	if m := n.find("pPr", "rPr"); m != nil {
		markProps = append(props{m}, rp...)
	}
	p.Mark = l.runStyle(markProps)
	if lvl := l.level(pp.elem("numPr")); lvl != nil {
		label := levelNumber.ReplaceAllString(lvl.child("lvlText").attr("val"), "1")
		lp := markProps
		if r := lvl.child("rPr"); r != nil {
			lp = append(props{r}, lp...)
		}
		p.Items = append(p.Items, Item{Kind: TextItem, Text: label, Run: l.runStyle(lp)})
		switch lvl.child("suff").attr("val") {
		case "space":
			p.Items = append(p.Items, Item{Kind: TextItem, Text: " ", Run: p.Mark})
		case "nothing":
		default:
			p.Items = append(p.Items, Item{Kind: TabItem, Run: p.Mark})
		}
	}
	p.label = offset(p.Items)
	if s := n.find("pPr", "sectPr"); s != nil {
		p.Section = &Section{raw: s}
	}
	l.inline(p, n, rp)
	return p
}

func resolvePara(p props) paraProps {
	r := paraProps{Line: 1, LineRule: "auto"}
	if v, ok := p.attr("spacing", "before"); ok {
		r.Before = v / 20
	}
	if v, ok := p.attr("spacing", "after"); ok {
		r.After = v / 20
	}
	// HTML-style automatic spacing, which Word renders as 14 pt
	if r.AutoBefore = autoSpacing(p, "beforeAutospacing"); r.AutoBefore {
		r.Before = 14
	}
	if r.AutoAfter = autoSpacing(p, "afterAutospacing"); r.AutoAfter {
		r.After = 14
	}
	for _, n := range p {
		c := n.child("spacing")
		if v, ok := c.num("line"); ok {
			r.LineRule = c.attr("lineRule")
			if r.LineRule == "" {
				r.LineRule = "auto"
			}
			if r.LineRule == "auto" {
				r.Line = v / 240
			} else {
				r.Line = v / 20
			}
			break
		}
	}
	indent := func(names ...string) (float64, bool) {
		for _, n := range p {
			for _, name := range names {
				if v, ok := n.child("ind").num(name); ok {
					return v / 20, true
				}
			}
		}
		return 0, false
	}
	r.Left, _ = indent("left", "start")
	r.Right, _ = indent("right", "end")
	for _, n := range p {
		ind := n.child("ind")
		if v, ok := ind.num("hanging"); ok {
			r.First = -v / 20
			break
		}
		if v, ok := ind.num("firstLine"); ok {
			r.First = v / 20
			break
		}
	}
	r.KeepNext = p.on("keepNext")
	r.KeepLines = p.on("keepLines")
	r.BreakBefore = p.on("pageBreakBefore")
	r.Widow = p.on("widowControl")
	r.Contextual = p.on("contextualSpacing")
	for _, n := range p {
		if tabs := n.child("tabs"); tabs != nil {
			for _, t := range tabs.children() {
				if v, ok := t.num("pos"); ok && t.attr("val") != "clear" {
					r.Tabs = append(r.Tabs, v/20)
				}
			}
			slices.Sort(r.Tabs)
			break
		}
	}
	return r
}

func autoSpacing(p props, attr string) bool {
	for _, n := range p {
		if v := n.child("spacing").attr(attr); v != "" {
			return v == "1" || v == "true" || v == "on"
		}
	}
	return false
}

func (l *loader) runStyle(p props) *RunStyle {
	size := 10.0
	if v, ok := p.attr("sz", "val"); ok {
		size = v / 2
	}
	if va := p.elem("vertAlign").attr("val"); va == "superscript" || va == "subscript" {
		size *= 2.0 / 3
	}
	spacing, _ := p.attr("spacing", "val")
	name := "Times New Roman"
	for _, n := range p {
		f := n.child("rFonts")
		if f == nil {
			continue
		}
		if t := f.attr("asciiTheme"); t != "" {
			if strings.HasPrefix(t, "major") {
				name = l.theme["major"]
			} else {
				name = l.theme["minor"]
			}
			break
		}
		if a := f.attr("ascii"); a != "" {
			name = a
			break
		}
	}
	font, ok := l.fonts.Get(name, p.on("b"), p.on("i"))
	if !ok {
		l.doc.Unsupported["font "+name] = true
	}
	return &RunStyle{Font: font, Size: size, Spacing: spacing / 20}
}

// inline reads the content of a paragraph, or of an element that holds runs.
func (l *loader) inline(p *Para, parent *node, paraRun props) {
	for _, n := range parent.children() {
		switch n.name {
		case "r":
			l.run(p, n, paraRun)
		case "hyperlink", "smartTag", "ins", "customXml", "moveTo", "dir", "bdo":
			l.inline(p, n, paraRun)
		case "sdt":
			l.inline(p, n.child("sdtContent"), paraRun)
		case "fldSimple":
			l.inline(p, n, paraRun)
		case "AlternateContent":
			l.inline(p, n.child("Choice"), paraRun)
		case "del", "moveFrom":
			l.doc.Unsupported["tracked changes"] = true
		case "oMath", "oMathPara":
			l.doc.Unsupported["equations"] = true
		}
	}
}

func (l *loader) visible() bool {
	return !slices.Contains(l.fields, true)
}

func (l *loader) run(p *Para, r *node, paraRun props) {
	rp := paraRun
	if style := r.find("rPr", "rStyle").attr("val"); style != "" {
		rp = append(l.styleChain(style, "rPr"), rp...)
	}
	if rPr := r.child("rPr"); rPr != nil {
		rp = append(props{rPr}, rp...)
	}
	if rp.on("vanish") || rp.on("webHidden") {
		return
	}
	var style *RunStyle
	get := func() *RunStyle {
		if style == nil {
			style = l.runStyle(rp)
		}
		return style
	}
	for _, n := range r.children() {
		switch n.name {
		case "fldChar":
			switch n.attr("fldCharType") {
			case "begin":
				l.fields = append(l.fields, true)
			case "separate":
				if len(l.fields) > 0 {
					l.fields[len(l.fields)-1] = false
				}
			case "end":
				if len(l.fields) > 0 {
					l.fields = l.fields[:len(l.fields)-1]
				}
			}
		case "lastRenderedPageBreak":
			// a break before the text is one before the numbering label too
			at := offset(p.Items)
			if at <= p.label {
				at = 0
			}
			p.WordBreaks = append(p.WordBreaks, at)
		}
		if !l.visible() {
			continue
		}
		switch n.name {
		case "t":
			p.Items = append(p.Items, Item{Kind: TextItem, Text: n.text, Run: get()})
		case "tab", "ptab":
			p.Items = append(p.Items, Item{Kind: TabItem, Run: get()})
		case "cr":
			p.Items = append(p.Items, Item{Kind: LineBreak, Run: get()})
		case "br":
			switch n.attr("type") {
			case "page":
				p.Items = append(p.Items, Item{Kind: PageBreak, Run: get()})
			case "column":
				p.Items = append(p.Items, Item{Kind: ColumnBreak, Run: get()})
			default:
				p.Items = append(p.Items, Item{Kind: LineBreak, Run: get()})
			}
		case "noBreakHyphen":
			p.Items = append(p.Items, Item{Kind: TextItem, Text: "‑", Run: get()})
		case "sym":
			l.doc.Unsupported["symbols"] = true
			p.Items = append(p.Items, Item{Kind: TextItem, Text: "□", Run: get()})
		case "footnoteReference", "endnoteReference":
			l.doc.Unsupported["notes"] = true
			p.Items = append(p.Items, Item{Kind: TextItem, Text: "1", Run: get()})
		case "drawing":
			l.drawing(p, n)
		case "pict", "object":
			l.doc.Unsupported["VML"] = true
			if w, h, ok := vmlSize(n); ok {
				p.Items = append(p.Items, Item{Kind: ObjectItem, W: w, H: h})
			}
		case "AlternateContent":
			if c := n.child("Choice"); c != nil {
				inner := &node{name: "r", kids: c.kids}
				if rPr := r.child("rPr"); rPr != nil {
					inner.kids = append([]*node{rPr}, c.kids...)
				}
				l.run(p, inner, paraRun)
			}
		}
	}
}

// offset is the position after the items, in characters.
func offset(items []Item) int {
	n := 0
	for _, it := range items {
		if it.Kind == TextItem {
			n += len([]rune(it.Text))
		} else {
			n++
		}
	}
	return n
}

func (l *loader) drawing(p *Para, n *node) {
	if in := n.child("inline"); in != nil {
		cx, _ := in.child("extent").num("cx")
		cy, _ := in.child("extent").num("cy")
		p.Items = append(p.Items, Item{Kind: ObjectItem, W: cx / 12700, H: cy / 12700})
		return
	}
	if a := n.child("anchor"); a != nil {
		for _, k := range a.children() {
			switch k.name {
			case "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom":
				l.doc.Unsupported["floating objects"] = true
			}
		}
	}
}

var levelNumber = regexp.MustCompile(`%\d`)

var vmlDimension = regexp.MustCompile(`(width|height):\s*([\d.]+)(pt|in|px)?`)

// vmlSize reads the size of an inline VML shape from its style.
func vmlSize(n *node) (w, h float64, ok bool) {
	var shape *node
	var walk func(*node)
	walk = func(n *node) {
		for _, k := range n.children() {
			if shape != nil {
				return
			}
			if k.name == "shape" || k.name == "rect" || k.name == "group" {
				shape = k
				return
			}
			walk(k)
		}
	}
	walk(n)
	style := shape.attr("style")
	if strings.Contains(style, "position:absolute") {
		return 0, 0, false
	}
	for _, m := range vmlDimension.FindAllStringSubmatch(style, -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		switch m[3] {
		case "in":
			v *= 72
		case "px":
			v *= 0.75
		}
		if m[1] == "width" {
			w = v
		} else {
			h = v
		}
	}
	return w, h, w > 0 && h > 0
}

func (l *loader) table(n *node) *Table {
	tblPr := n.child("tblPr")
	if tblPr.child("tblpPr") != nil {
		l.doc.Unsupported["floating tables"] = true
	}
	style := tblPr.child("tblStyle").attr("val")
	if l.styles[style] == nil {
		style = l.defaultTable
	}
	tbl := append(props{tblPr}, l.styleChain(style, "tblPr")...)
	margin := func(own *node, side string, deflt float64) float64 {
		if v, ok := own.child(side).num("w"); ok {
			return v / 20
		}
		for _, t := range tbl {
			if v, ok := t.find("tblCellMar", side).num("w"); ok {
				return v / 20
			}
		}
		return deflt
	}
	border := func(own *node, side string) float64 {
		b := own.child(side)
		for _, t := range tbl {
			if b != nil {
				break
			}
			b = t.find("tblBorders", side)
		}
		if b == nil || b.attr("val") == "nil" || b.attr("val") == "none" {
			return 0
		}
		v, _ := b.num("sz")
		return v / 8
	}
	spacing, _ := tbl.attr("tblCellSpacing", "w")
	var grid []float64
	for _, c := range n.child("tblGrid").children() {
		if v, ok := c.num("w"); ok && c.name == "gridCol" {
			grid = append(grid, v/20)
		}
	}
	var trs []*node
	for _, tr := range n.children() {
		if tr.name == "tr" {
			trs = append(trs, tr)
		}
	}
	t := &Table{}
	for r, tr := range trs {
		trPr := tr.child("trPr")
		row := Row{CantSplit: trPr.child("cantSplit") != nil && trPr.child("cantSplit").on(), Spacing: spacing / 20}
		if h, ok := trPr.child("trHeight").num("val"); ok {
			row.Height = h / 20
			row.Exact = trPr.child("trHeight").attr("hRule") == "exact"
		}
		col := 0
		if v, ok := trPr.child("gridBefore").num("val"); ok {
			col = int(v)
		}
		for _, tc := range tr.children() {
			if tc.name == "sdt" {
				tc = tc.find("sdtContent", "tc")
			}
			if tc == nil || tc.name != "tc" {
				continue
			}
			tcPr := tc.child("tcPr")
			span := 1
			if v, ok := tcPr.child("gridSpan").num("val"); ok && v > 1 {
				span = int(v)
			}
			width := 0.0
			for i := col; i < col+span && i < len(grid); i++ {
				width += grid[i]
			}
			if width == 0 {
				if v, ok := tcPr.child("tcW").num("w"); ok && tcPr.child("tcW").attr("type") == "dxa" {
					width = v / 20
				}
			}
			col += span
			mar := tcPr.child("tcMar")
			left := margin(mar, "left", 5.4) + margin(mar, "start", 0)
			right := margin(mar, "right", 5.4) + margin(mar, "end", 0)
			top, bottom := "insideH", "insideH"
			if r == 0 || spacing > 0 {
				top = "top"
			}
			if r == len(trs)-1 || spacing > 0 {
				bottom = "bottom"
			}
			borders := tcPr.child("tcBorders")
			vm := tcPr.child("vMerge")
			cell := Cell{
				Width:           max(width-left-right, 1),
				Top:             margin(mar, "top", 0),
				Bottom:          margin(mar, "bottom", 0),
				BorderTop:       border(borders, top),
				BorderBottom:    border(borders, bottom),
				Blocks:          l.blocks(tc, &style),
				MergedFromAbove: vm != nil && vm.attr("val") != "restart",
			}
			if spacing == 0 && r > 0 {
				// a border between two rows is drawn once, below the first
				cell.BorderTop = 0
			}
			row.Cells = append(row.Cells, cell)
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

func (l *loader) section(s *node, parts map[string]string) *Section {
	sec := &Section{Width: 612, Height: 792, Top: 72, Bottom: 72, Left: 90, Right: 90, HeaderDist: 36, FooterDist: 36}
	if s == nil {
		return sec
	}
	set := func(dst *float64, n *node, attr string) {
		if v, ok := n.num(attr); ok {
			*dst = v / 20
		}
	}
	set(&sec.Width, s.child("pgSz"), "w")
	set(&sec.Height, s.child("pgSz"), "h")
	m := s.child("pgMar")
	set(&sec.Top, m, "top")
	set(&sec.Bottom, m, "bottom")
	set(&sec.Left, m, "left")
	set(&sec.Right, m, "right")
	set(&sec.HeaderDist, m, "header")
	set(&sec.FooterDist, m, "footer")
	gutter := 0.0
	set(&gutter, m, "gutter")
	if l.gutterAtTop {
		sec.Top += gutter
	} else {
		sec.Left += gutter
	}
	sec.Top, sec.Bottom = abs(sec.Top), abs(sec.Bottom)
	sec.Type = s.child("type").attr("val")
	sec.TitlePage = s.child("titlePg") != nil && s.child("titlePg").on()
	if v, ok := s.child("cols").num("num"); ok && v > 1 {
		l.doc.Unsupported["columns"] = true
	}
	if t := s.child("docGrid").attr("type"); t == "lines" || t == "linesAndChars" {
		l.doc.Unsupported["line grid"] = true
	}
	for _, ref := range s.children() {
		if ref.name != "headerReference" && ref.name != "footerReference" {
			continue
		}
		part := parts[ref.attr("id")]
		root, err := l.parse(part)
		if err != nil {
			continue
		}
		saved := l.fields
		l.fields = nil
		var blocks []Block
		for _, k := range root.children() {
			blocks = l.blocks(k, nil)
		}
		l.fields = saved
		switch ref.name + "/" + ref.attr("type") {
		case "headerReference/default":
			sec.Header = blocks
		case "headerReference/first":
			sec.FirstHeader = blocks
		case "footerReference/default":
			sec.Footer = blocks
		case "footerReference/first":
			sec.FirstFooter = blocks
		}
	}
	return sec
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
