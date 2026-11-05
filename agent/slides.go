package agent

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/citadellefr/trame/ot"
)

// SlideInfo is a slide of a presentation, Index counted from 1.
type SlideInfo struct {
	ID     string `json:"id"`
	Index  int    `json:"index"`
	Layout string `json:"layout,omitempty"`
	Title  string `json:"title,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// Layout is a slide layout a new slide can take.
type Layout struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Shape is a shape of a slide and its text, a paragraph a line; Shapes
// are those of a group.
type Shape struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Name        string  `json:"name,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
	Box         *Box    `json:"box,omitempty"`
	Text        string  `json:"text,omitempty"`
	Shapes      []Shape `json:"shapes,omitempty"`
}

// SlideComment is a comment of a slide.
type SlideComment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// Slide is a slide with its shapes, notes and comments.
type Slide struct {
	SlideInfo
	Shapes   []Shape        `json:"shapes"`
	Notes    string         `json:"notes,omitempty"`
	Comments []SlideComment `json:"comments,omitempty"`
}

// emuPerPoint is how many EMU, the unit of the file, make a point, and
// maxPoints the most a coordinate goes.
const (
	emuPerPoint = 12700
	maxPoints   = math.MaxInt32 / emuPerPoint
)

// shapeTypes are the nodes a slide or a group holds as shapes.
var shapeTypes = map[string]bool{"sp": true, "pic": true, "cxn": true, "grp": true, "frame": true, "alt": true, "other": true}

var (
	slideOnly = map[string]bool{"slide": true}
	textOnly  = map[string]bool{"sp": true}
	deletable = func() map[string]bool { m := maps.Clone(shapeTypes); m["slide"] = true; return m }()
)

// Slides are the slides of a presentation and the layouts new ones take.
func (a *Agent) Slides() (version string, slides []SlideInfo, layouts []Layout, err error) {
	err = a.on(a.isSlides, func(s *state) error {
		version = a.version()
		for i, n := range s.tree.Children("deck") {
			slides = append(slides, s.slideInfo(n, i+1))
		}
		for _, l := range s.layouts() {
			layouts = append(layouts, Layout{l.ID, str(l.Attrs, "name")})
		}
		return nil
	})
	return version, slides, layouts, err
}

// Slide is the slide id with all it holds.
func (a *Agent) Slide(id string) (version string, slide Slide, err error) {
	err = a.on(a.isSlides, func(s *state) error {
		i := slices.IndexFunc(s.tree.Children("deck"), func(n *ot.Node) bool { return n.ID == id })
		if i < 0 {
			return gone(id)
		}
		version = a.version()
		slide = Slide{SlideInfo: s.slideInfo(s.tree.Node(id), i+1), Shapes: s.shapes(id)}
		if slide.Shapes == nil {
			slide.Shapes = []Shape{}
		}
		for _, n := range s.tree.Children(id) {
			switch n.Type {
			case "notes":
				slide.Notes = lines(n)
			case "comment":
				slide.Comments = append(slide.Comments, SlideComment{n.ID, str(n.Attrs, "author"), lines(n)})
			}
		}
		return nil
	})
	return version, slide, err
}

func (s *state) layouts() []*ot.Node {
	var out []*ot.Node
	for _, m := range s.tree.Children("") {
		if m.Type != "master" {
			continue
		}
		for _, l := range s.tree.Children(m.ID) {
			if l.Type == "layout" {
				out = append(out, l)
			}
		}
	}
	return out
}

func (s *state) slideInfo(n *ot.Node, index int) SlideInfo {
	info := SlideInfo{ID: n.ID, Index: index, Hidden: string(n.Attrs["hidden"]) == "true"}
	if l := s.tree.Node(str(n.Attrs, "layout")); l != nil {
		info.Layout = str(l.Attrs, "name")
	}
	for _, k := range s.tree.Children(n.ID) {
		if ph, _ := placeholder(k); ph == "title" || ph == "ctrTitle" {
			info.Title = strings.ReplaceAll(lines(k), "\n", " ")
			break
		}
	}
	return info
}

// placeholder is the type of the placeholder a shape stands for, and
// whether it is one.
func placeholder(n *ot.Node) (string, bool) {
	raw, ok := n.Attrs["ph"]
	var ph struct{ Type string }
	_ = json.Unmarshal(raw, &ph)
	return ph.Type, ok
}

func (s *state) shapes(parent string) []Shape {
	var out []Shape
	for _, n := range s.tree.Children(parent) {
		if !shapeTypes[n.Type] {
			continue
		}
		sh := Shape{ID: n.ID, Type: n.Type, Name: str(n.Attrs, "name"), Box: boxOf(n), Text: lines(n)}
		if ph, ok := placeholder(n); ok {
			sh.Placeholder = ph
			if ph == "" {
				sh.Placeholder = "obj"
			}
		}
		switch n.Type {
		case "frame":
			sh.Text = s.table(n)
		case "grp":
			sh.Shapes = s.shapes(n.ID)
		}
		out = append(out, sh)
	}
	return out
}

// table is the text of a table, a row a line, its cells apart.
func (s *state) table(frame *ot.Node) string {
	var rows []string
	for _, tr := range s.tree.Children(frame.ID) {
		var cells []string
		for _, tc := range s.tree.Children(tr.ID) {
			cells = append(cells, strings.ReplaceAll(lines(tc), "\n", " "))
		}
		rows = append(rows, strings.Join(cells, " | "))
	}
	return strings.Join(rows, "\n")
}

// boxOf is where a shape is, in points.
func boxOf(n *ot.Node) *Box {
	var b Box
	if json.Unmarshal(n.Attrs["xfrm"], &b) != nil {
		return nil
	}
	pt := func(emu float64) float64 { return math.Round(emu / emuPerPoint) }
	return &Box{pt(b.X), pt(b.Y), pt(b.W), pt(b.H)}
}

// lines is the text of a node, a paragraph a line.
func lines(n *ot.Node) string {
	if n.Text == nil {
		return ""
	}
	var out []string
	for _, p := range n.Text.Paragraphs() {
		out = append(out, plain(p))
	}
	return strings.Join(out, "\n")
}

func (p *planner) slideEdit(e Edit) (ot.Edit, error) {
	switch e.Op {
	case "text":
		return p.setText(e)
	case "shape":
		return p.addShape(e)
	case "slide":
		return p.addSlide(e)
	case "delete":
		return p.removeNode(e)
	case "notes":
		return p.setNotes(e)
	case "comment":
		return p.slideComment(e)
	}
	return nil, fmt.Errorf("%w: unknown op %q", ErrInvalid, e.Op)
}

// node finds a block of one of the types.
func (p *planner) node(id string, types map[string]bool) (*ot.Node, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: no block", ErrInvalid)
	}
	n := p.s.tree.Node(id)
	switch {
	case n == nil:
		return nil, gone(id)
	case !types[n.Type]:
		return nil, fmt.Errorf("%w: %s is a %s", ErrInvalid, id, n.Type)
	}
	return n, nil
}

// unchanged checks that a node did not change since the batch's version.
func (p *planner) unchanged(n *ot.Node) error {
	if p.orig.tree.Node(n.ID) != nil && p.orig.mod[n.ID] > p.base {
		return fmt.Errorf("%w: %s", ErrConflict, n.ID)
	}
	return nil
}

func (p *planner) setText(e Edit) (ot.Edit, error) {
	n, err := p.node(e.Block, textOnly)
	if err != nil {
		return nil, err
	}
	return p.rewrite(n, e.Text)
}

// rewrite replaces all the text of a node, which takes the style of the
// first run and the first paragraph of the old; the last paragraph keeps
// its mark, which a flow cannot lose.
func (p *planner) rewrite(n *ot.Node, text string) (ot.Edit, error) {
	if err := p.unchanged(n); err != nil {
		return nil, err
	}
	if n.Text == nil {
		return nil, fmt.Errorf("%w: %s holds no text", ErrInvalid, n.ID)
	}
	flow := n.Text.Delta()
	run, mark := styles(flow)
	var b builder
	paras := strings.Split(text, "\n")
	for i, line := range paras {
		b.insert(line, run)
		if i < len(paras)-1 {
			b.insert("\n", mark)
		}
	}
	b.remove(flowLen(flow) - 1)
	return b.edit(n.ID), nil
}

// styles are the attributes a run and a paragraph mark take from the
// first of a flow: not what makes a run a field, a link or an object.
func styles(flow ot.Delta) (run, mark ot.Attrs) {
	foundRun, foundMark := false, false
	each(flow, 0, flowLen(flow), func(r rune, attrs ot.Attrs) {
		switch {
		case r == '\n' && !foundMark:
			mark, foundMark = attrs, true
		case r != '\n' && !foundRun && attrs["fld"] == "" && attrs["o"] == "":
			run, foundRun = attrs, true
		}
	})
	run = maps.Clone(run)
	delete(run, "link")
	return run, mark
}

func (p *planner) addShape(e Edit) (ot.Edit, error) {
	slide, err := p.node(e.Block, slideOnly)
	if err != nil {
		return nil, err
	}
	if e.Box == nil || e.Box.W <= 0 || e.Box.H <= 0 {
		return nil, fmt.Errorf("%w: a shape needs a box with a width and a height", ErrInvalid)
	}
	for _, v := range []float64{e.Box.X, e.Box.Y, e.Box.W, e.Box.H} {
		if !(math.Abs(v) <= maxPoints) {
			return nil, fmt.Errorf("%w: a box is within %d points", ErrInvalid, maxPoints)
		}
	}
	emu := func(pt float64) int64 { return int64(math.Round(pt * emuPerPoint)) }
	var last *ot.Node
	shapes := 0
	for _, k := range p.s.tree.Children(slide.ID) {
		if shapeTypes[k.Type] {
			last = k
			shapes++
		}
	}
	var text ot.Delta
	for _, line := range strings.Split(e.Text, "\n") {
		text = text.Push(ot.Op{Insert: line}).Push(ot.Op{Insert: "\n"})
	}
	return ot.Edit{{Op: ot.OpNew, ID: rand.Text(), Type: "sp", Parent: slide.ID, Key: between(last, nil), Text: text, Attrs: ot.Values{
		"name": quote(fmt.Sprintf("Text %d", shapes+1)),
		"geom": raw(map[string]string{"prst": "rect"}),
		"xfrm": raw(map[string]int64{"x": emu(e.Box.X), "y": emu(e.Box.Y), "w": emu(e.Box.W), "h": emu(e.Box.H)}),
		"fill": raw(map[string]bool{"none": true}),
		"body": raw(map[string]string{"wrap": "square", "fit": "shape", "rtlCol": "0"}),
	}}}, nil
}

func (p *planner) addSlide(e Edit) (ot.Edit, error) {
	layout, err := p.layout(e.Layout)
	if err != nil {
		return nil, err
	}
	kids := p.s.tree.Children("deck")
	var before, next *ot.Node
	if e.Block == "" {
		if len(kids) > 0 {
			before = kids[len(kids)-1]
		}
	} else {
		if before, err = p.node(e.Block, slideOnly); err != nil {
			return nil, err
		}
		if i := slices.Index(kids, before); i+1 < len(kids) {
			next = kids[i+1]
		}
	}
	id := rand.Text()
	out := ot.Edit{{Op: ot.OpNew, ID: id, Type: "slide", Parent: "deck", Key: between(before, next), Attrs: ot.Values{"layout": quote(layout.ID)}}}
	key := ""
	for _, ph := range p.s.tree.Children(layout.ID) {
		kind, ok := placeholder(ph)
		if !ok || slices.Contains([]string{"dt", "ftr", "sldNum", "hdr"}, kind) {
			continue
		}
		var all map[string]json.RawMessage
		_ = json.Unmarshal(ph.Attrs["ph"], &all)
		kept := map[string]json.RawMessage{}
		for _, k := range []string{"type", "idx", "orient", "sz"} {
			if v, ok := all[k]; ok {
				kept[k] = v
			}
		}
		attrs := ot.Values{"ph": raw(kept)}
		if name, ok := ph.Attrs["name"]; ok {
			attrs["name"] = name
		}
		key = ot.KeyBetween(key, "")
		out = append(out, ot.Change{Op: ot.OpNew, ID: rand.Text(), Type: "sp", Parent: id, Key: key, Attrs: attrs, Text: ot.Delta{{Insert: "\n"}}})
	}
	return out, nil
}

// layout finds a layout by its id or its name.
func (p *planner) layout(ref string) (*ot.Node, error) {
	if ref == "" {
		return nil, fmt.Errorf("%w: no layout", ErrInvalid)
	}
	var named *ot.Node
	for _, l := range p.s.layouts() {
		if l.ID == ref {
			return l, nil
		}
		if named == nil && strings.EqualFold(str(l.Attrs, "name"), ref) {
			named = l
		}
	}
	if named == nil {
		return nil, fmt.Errorf("%w: no layout %q", ErrInvalid, ref)
	}
	return named, nil
}

func (p *planner) removeNode(e Edit) (ot.Edit, error) {
	n, err := p.node(e.Block, deletable)
	if err != nil {
		return nil, err
	}
	if err := p.unchanged(n); err != nil {
		return nil, err
	}
	return ot.Edit{{Op: ot.OpDel, ID: n.ID}}, nil
}

func (p *planner) setNotes(e Edit) (ot.Edit, error) {
	slide, err := p.node(e.Block, slideOnly)
	if err != nil {
		return nil, err
	}
	for _, k := range p.s.tree.Children(slide.ID) {
		if k.Type == "notes" {
			return p.rewrite(k, e.Text)
		}
	}
	return nil, fmt.Errorf("%w: slide %s has no notes to change", ErrInvalid, slide.ID)
}

func (p *planner) slideComment(e Edit) (ot.Edit, error) {
	slide, err := p.node(e.Block, slideOnly)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(e.Text) == "" {
		return nil, fmt.Errorf("%w: empty comment", ErrInvalid)
	}
	last, started := "zzz", 0
	for _, k := range p.s.tree.Children(slide.ID) {
		if k.Type == "comment" {
			last = k.Key
			if str(k.Attrs, "parent") == "" {
				started++
			}
		}
	}
	step := 96 * (started%12 + 1)
	return ot.Edit{{Op: ot.OpNew, ID: rand.Text(), Type: "comment", Parent: slide.ID, Key: ot.KeyBetween(last, ""), Text: ot.Delta{{Insert: strings.TrimSpace(e.Text) + "\n"}}, Attrs: ot.Values{
		"author":   quote(p.name),
		"initials": quote(initials(p.name)),
		"date":     quote(p.date),
		"x":        raw(step),
		"y":        raw(step),
	}}}, nil
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// unrestorable tells what a change that takes back the agent's would make
// again that the Check lets nobody make: the notes of a slide, the comment
// of someone else, or the charts of a sheet. It is "" for the others.
func (a *Agent) unrestorable(c ot.Change) string {
	if c.Op == ot.OpSet && c.Attrs["charts"] != nil {
		return "the charts of " + a.s.name(c.ID)
	}
	if c.Op != ot.OpNew {
		return ""
	}
	switch author := str(c.Attrs, "author"); {
	case c.Type == "notes":
		return "the notes of slide " + c.Parent
	case c.Type == "comment" && author != a.peer.Name:
		return fmt.Sprintf("the comment of %s on slide %s", author, c.Parent)
	}
	return ""
}

// previewSlides tells what the edit does to the nodes it touches, going
// from the document before to the one after: the text each had in
// {--…--} and has in {++…++}.
func previewSlides(before, after *state, e ot.Edit) string {
	var ids []string
	for _, c := range e {
		if !slices.Contains(ids, c.ID) {
			ids = append(ids, c.ID)
		}
	}
	var b strings.Builder
	for _, id := range ids {
		old, now := before.tree.Node(id), after.tree.Node(id)
		switch {
		case old == nil && now == nil:
		case old == nil:
			if before.tree.Node(now.Parent) == nil {
				continue
			}
			fmt.Fprintf(&b, "{++%s++}\n", entry(after.name(id), lines(now)))
		case now == nil:
			if after.tree.Node(old.Parent) == nil {
				continue
			}
			fmt.Fprintf(&b, "{--%s--}\n", entry(before.name(id), lines(old)))
		default:
			fmt.Fprintf(&b, "%s: {--%s--}{++%s++}\n", after.name(id), lines(old), lines(now))
		}
	}
	return b.String()
}

func entry(name, text string) string {
	if text == "" {
		return name
	}
	return name + ": " + text
}
