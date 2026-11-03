package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/trame/ot"
)

// Batch is edits applied together, or not at all.
//
// Each edit finds its block in the document as the edits before it left
// it. In Word they are revisions signed by the agent unless Direct.
type Batch struct {
	// OnBehalfOf is the user the agent acts for.
	OnBehalfOf string `json:"onBehalfOf"`
	// Version is the one the edits were prepared on: the batch fails if a
	// block they touch changed since. Empty is the current one.
	Version string `json:"version,omitempty"`
	Direct  bool   `json:"direct,omitempty"`
	Edits   []Edit `json:"edits"`
}

// Edit is one change of a batch, by Op:
//
//   - replace: Find, the Nth time it is in Block, becomes Text; all the
//     text of the block when Find is empty;
//   - insert: the paragraphs Text writes in Markdown go after Block, at the
//     start of the document when Block is empty;
//   - delete: Block goes; a table only Direct;
//   - format: Find in Block takes Format: b, i, u, strike… "1" to set, ""
//     to clear;
//   - comment: Text comments Find in Block;
//   - reply: Text answers the comment Comment;
//   - accept, reject: Revision, an id Revisions gives.
//
// PowerPoint will take the same edits on the paragraphs of its shapes, and
// Excel replace on cells named as blocks.
type Edit struct {
	Op       string            `json:"op"`
	Block    string            `json:"block,omitempty"`
	Find     string            `json:"find,omitempty"`
	Nth      int               `json:"nth,omitempty"`
	Text     string            `json:"text,omitempty"`
	Format   map[string]string `json:"format,omitempty"`
	Comment  string            `json:"comment,omitempty"`
	Revision string            `json:"revision,omitempty"`
}

// Record is a batch the agent applied: the version it made, the user it
// was for and the date of its revisions.
type Record struct {
	Version    string    `json:"version"`
	OnBehalfOf string    `json:"onBehalfOf"`
	Date       time.Time `json:"date"`
	Edits      []Edit    `json:"edits"`
}

// Preview is what a batch would change, the blocks it touches in
// CriticMarkup, on the document at Version. The ids of the blocks it adds
// are only those they would likely get.
type Preview struct {
	Version string `json:"version"`
	Markup  string `json:"markup"`
}

// Preview tells what the batch would do, changing nothing.
func (a *Agent) Preview(b Batch) (Preview, error) {
	var out Preview
	err := a.word(func(s *state) error {
		_, work, err := a.plan(b, a.date())
		if err != nil {
			return err
		}
		r := newReader(work, Critic)
		r.only = work.version
		var text strings.Builder
		r.blocks("body", &text)
		s.eachParagraph("body", func(id string, _ *ot.Node, p ot.Delta, _ int, _ bool) {
			if _, ok := work.where[id]; !ok {
				text.WriteString("[" + id + "] {--" + plain(p) + "--}\n")
			}
		})
		out = Preview{a.version(), text.String()}
		return nil
	})
	return out, err
}

// Apply makes the batch, once the host's Allow agrees, and returns what it
// recorded.
func (a *Agent) Apply(ctx context.Context, b Batch) (Record, error) {
	if !a.isWord() {
		return Record{}, ErrUnsupported
	}
	if a.opt.Allow != nil {
		if err := a.opt.Allow(ctx, b); err != nil {
			return Record{}, fmt.Errorf("%w: %w", ErrDenied, err)
		}
	}
	a.sending.Lock()
	defer a.sending.Unlock()
	if err := context.Cause(ctx); err != nil {
		return Record{}, err
	}
	a.mu.Lock()
	date := a.date()
	e, _, err := a.plan(b, date)
	if err == nil {
		err = a.err
	}
	if err != nil {
		a.mu.Unlock()
		return Record{}, err
	}
	f, err := a.submit(e)
	if err != nil {
		return Record{}, err
	}
	a.mu.Lock()
	rec := Record{a.versionOf(f.version), b.OnBehalfOf, date, slices.Clone(b.Edits)}
	a.journal = append(a.journal, rec)
	a.mu.Unlock()
	if c, ok := cursorAfter(f.applied); ok {
		_ = a.show(c)
	}
	return rec, nil
}

// UndoMine rejects the revisions the agent made for user, and only those.
func (a *Agent) UndoMine(ctx context.Context, user string) (Record, error) {
	for {
		var b Batch
		err := a.word(func(s *state) error {
			b = Batch{OnBehalfOf: user, Version: a.version()}
			revs := s.revisions()
			for i := len(revs) - 1; i >= 0; i-- {
				if r := a.describe(revs[i]); r.Author == a.peer.Name && r.OnBehalfOf == user {
					b.Edits = append(b.Edits, Edit{Op: "reject", Revision: r.ID})
				}
			}
			return nil
		})
		if err != nil || len(b.Edits) == 0 {
			return Record{}, err
		}
		rec, err := a.Apply(ctx, b)
		if !errors.Is(err, ErrConflict) {
			return rec, err
		}
	}
}

// Journal is what the agent applied, to give the next session's Options.
func (a *Agent) Journal() []Record {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.journal)
}

// Focus shows the agent's cursor over a paragraph.
func (a *Agent) Focus(block string) error {
	var c Cursor
	err := a.word(func(s *state) error {
		n, i, start, ok := s.paragraph(block)
		if !ok {
			return gone(block)
		}
		c = Cursor{n.ID, start, start + flowLen(n.Text.Paragraphs()[i]) - 1}
		return nil
	})
	if err != nil {
		return err
	}
	return a.show(c)
}

// date is the date of the next batch's revisions: one per batch, for the
// revisions to tell which batch made them.
func (a *Agent) date() time.Time {
	d := a.opt.Now().UTC().Truncate(time.Second)
	if n := len(a.journal); n > 0 && !d.After(a.journal[n-1].Date) {
		d = a.journal[n-1].Date.Add(time.Second)
	}
	return d
}

// behalfOf is the user of the batch of that date.
func (a *Agent) behalfOf(date time.Time) string {
	for _, r := range a.journal {
		if r.Date.Equal(date) {
			return r.OnBehalfOf
		}
	}
	return ""
}

func gone(id string) error {
	return fmt.Errorf("%w: %s (deleted or rewritten since it was read)", ErrGone, id)
}

// plan turns a batch into the edit that makes it on the agent's document,
// and the document it would make.
func (a *Agent) plan(b Batch, date time.Time) (ot.Edit, *state, error) {
	if strings.TrimSpace(b.OnBehalfOf) == "" {
		return nil, nil, fmt.Errorf("%w: no user to act on behalf of", ErrInvalid)
	}
	base, err := a.since(b.Version)
	if err != nil {
		return nil, nil, err
	}
	p := &planner{orig: a.s, s: a.s.clone(), base: base, name: a.peer.Name, date: date.Format(time.RFC3339), direct: b.Direct}
	v := a.s.version + 1
	var out ot.Edit
	for i, ed := range b.Edits {
		e, err := p.edit(ed)
		if err == nil {
			if err = p.s.apply(e, v); err != nil {
				err = fmt.Errorf("%w: %w", ErrInvalid, err)
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("edit %d: %w", i+1, err)
		}
		out = append(out, e...)
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	p.s.version = v
	return out, p.s, nil
}

// planner resolves the edits of a batch one after the other on s, a copy
// of the document orig at the version base.
type planner struct {
	orig, s *state
	base    uint64
	name    string
	date    string
	direct  bool
}

func (p *planner) edit(e Edit) (ot.Edit, error) {
	switch e.Op {
	case "replace":
		return p.replace(e)
	case "insert":
		return p.insert(e)
	case "delete":
		return p.delete(e)
	case "format":
		return p.format(e)
	case "comment":
		return p.comment(e)
	case "reply":
		return p.reply(e)
	case "accept", "reject":
		return p.revision(e)
	}
	return nil, fmt.Errorf("%w: unknown op %q", ErrInvalid, e.Op)
}

// block checks that a block exists and did not change since the batch's
// version.
func (p *planner) block(id string) error {
	if _, ok := p.s.changed(id); !ok {
		return gone(id)
	}
	if mod, ok := p.orig.changed(id); ok && mod > p.base {
		return fmt.Errorf("%w: %s", ErrConflict, id)
	}
	return nil
}

func (p *planner) para(id string) (n *ot.Node, start int, para ot.Delta, err error) {
	if err := p.block(id); err != nil {
		return nil, 0, nil, err
	}
	n, i, start, ok := p.s.paragraph(id)
	if !ok {
		return nil, 0, nil, fmt.Errorf("%w: %s is not a paragraph", ErrInvalid, id)
	}
	return n, start, n.Text.Paragraphs()[i], nil
}

// find is where an edit's text is in the flow of its block.
func (p *planner) find(e Edit) (n *ot.Node, from, to int, err error) {
	n, start, para, err := p.para(e.Block)
	if err != nil {
		return nil, 0, 0, err
	}
	from, to, ok := spanOf(para, start).find(e.Find, e.Nth)
	if !ok {
		return nil, 0, 0, fmt.Errorf("%w: %q in %s", ErrNotFound, e.Find, e.Block)
	}
	return n, from, to, nil
}

// tracked are the attributes of a revision of that kind, none when direct.
func (p *planner) tracked(kind string) ot.Attrs {
	if p.direct {
		return nil
	}
	return ot.Attrs{kind: p.name, kind + "d": p.date}
}

// strike deletes a character: as a revision, unless direct or one the
// agent inserted.
func (p *planner) strike(b *builder, r rune, attrs ot.Attrs) {
	n := utf16.RuneLen(r)
	switch {
	case !visible(r, attrs):
		b.retain(n, nil)
	case p.direct || attrs["ins"] == p.name && r != '\n':
		b.remove(n)
	default:
		b.retain(n, p.tracked("del"))
	}
}

func (p *planner) replace(e Edit) (ot.Edit, error) {
	if strings.Contains(e.Text, "\n") {
		return nil, fmt.Errorf("%w: replace stays in its paragraph, insert adds paragraphs", ErrInvalid)
	}
	n, from, to, err := p.find(e)
	if err != nil {
		return nil, err
	}
	flow := n.Text.Delta()
	var b builder
	b.to(from)
	style, found := ot.Attrs(nil), false
	each(flow, from, to, func(r rune, attrs ot.Attrs) {
		if !found && visible(r, attrs) {
			style, found = attrs, true
		}
		p.strike(&b, r, attrs)
	})
	if !found && from > 0 {
		each(flow, from-1, from, func(r rune, attrs ot.Attrs) {
			if r != '\n' {
				style = attrs
			}
		})
	}
	b.insert(e.Text, with(runStyle(style), p.tracked("ins")))
	return b.edit(n.ID), nil
}

// formats are the attributes format sets.
var formats = map[string]bool{"b": true, "i": true, "u": true, "strike": true, "dstrike": true, "caps": true, "smallCaps": true,
	"hl": true, "color": true, "sz": true, "font": true, "va": true, "shd": true}

func (p *planner) format(e Edit) (ot.Edit, error) {
	if len(e.Format) == 0 {
		return nil, fmt.Errorf("%w: no format", ErrInvalid)
	}
	for k := range e.Format {
		if !formats[k] {
			return nil, fmt.Errorf("%w: unknown format %q", ErrInvalid, k)
		}
	}
	n, from, to, err := p.find(e)
	if err != nil {
		return nil, err
	}
	set := ot.Attrs(e.Format)
	var b builder
	b.to(from)
	var again ot.Delta
	each(n.Text.Delta(), from, to, func(r rune, attrs ot.Attrs) {
		switch {
		case !visible(r, attrs) || formatted(attrs, set):
			b.retain(utf16.RuneLen(r), nil)
		case p.direct:
			b.retain(utf16.RuneLen(r), set)
		default:
			p.strike(&b, r, attrs)
			again = again.Push(ot.Op{Insert: string(r), Attrs: with(runStyle(attrs), set, p.tracked("ins"))})
		}
	})
	for _, o := range again {
		b.insert(o.Insert, o.Attrs)
	}
	return b.edit(n.ID), nil
}

func formatted(attrs, set ot.Attrs) bool {
	for k, v := range set {
		if attrs[k] != v {
			return false
		}
	}
	return true
}

func (p *planner) delete(e Edit) (ot.Edit, error) {
	if err := p.block(e.Block); err != nil {
		return nil, err
	}
	if node, ok := p.s.nodes[e.Block]; ok {
		if !p.direct {
			return nil, fmt.Errorf("%w: a table is deleted directly, not as a revision", ErrInvalid)
		}
		return ot.Edit{{Op: ot.OpDel, ID: node}}, nil
	}
	n, start, para, err := p.para(e.Block)
	if err != nil {
		return nil, err
	}
	end := start + flowLen(para)
	if p.direct {
		return p.s.removeRange(n, start, end), nil
	}
	var b builder
	b.to(start)
	each(n.Text.Delta(), start, end, func(r rune, attrs ot.Attrs) {
		p.strike(&b, r, attrs)
	})
	return b.edit(n.ID), nil
}

func (p *planner) insert(e Edit) (ot.Edit, error) {
	paras := parseMarkdown(e.Text)
	if len(paras) == 0 {
		return nil, fmt.Errorf("%w: no paragraph to insert", ErrInvalid)
	}
	styles := newReader(p.s, Markdown).styles
	ins := p.tracked("ins")
	var text ot.Delta
	for _, m := range paras {
		for _, r := range m.runs {
			text = text.Push(ot.Op{Insert: r.text, Attrs: with(r.attrs, ins)})
		}
		text = text.Push(ot.Op{Insert: "\n", Attrs: with(m.mark(styles), ins)})
	}
	n, at, err := p.after(e.Block)
	if err != nil {
		return nil, err
	}
	if n != nil {
		var b builder
		b.to(at)
		for _, o := range text {
			b.insert(o.Insert, o.Attrs)
		}
		return b.edit(n.ID), nil
	}
	parent, before, next := p.place(e.Block)
	return ot.Edit{{Op: ot.OpNew, ID: rand.Text(), Type: "text", Parent: parent, Key: between(before, next), Text: text}}, nil
}

// after is where paragraphs inserted after a block go: an offset of a flow,
// or no flow when a new one has to hold them.
func (p *planner) after(id string) (*ot.Node, int, error) {
	if id != "" {
		if err := p.block(id); err != nil {
			return nil, 0, err
		}
		if n, i, start, ok := p.s.paragraph(id); ok {
			paras := n.Text.Paragraphs()
			if i < len(paras)-1 {
				return n, start + flowLen(paras[i]), nil
			}
		}
	}
	_, _, next := p.place(id)
	if next != nil && next.Text != nil {
		return next, 0, nil
	}
	return nil, 0, nil
}

// place is the parent of a block and its siblings around where blocks
// after it go.
func (p *planner) place(id string) (parent string, before, next *ot.Node) {
	parent = "body"
	if id != "" {
		node := p.s.where[id]
		if t, ok := p.s.nodes[id]; ok {
			node = t
		}
		before = p.s.tree.Node(node)
		parent = before.Parent
	}
	kids := p.s.tree.Children(parent)
	i := 0
	if before != nil {
		i = slices.Index(kids, before) + 1
	}
	if i < len(kids) {
		next = kids[i]
	}
	return parent, before, next
}

func between(before, next *ot.Node) string {
	a, b := "", ""
	if before != nil {
		a = before.Key
	}
	if next != nil {
		b = next.Key
	}
	if b != "" && a >= b {
		return a
	}
	return ot.KeyBetween(a, b)
}

func (p *planner) comment(e Edit) (ot.Edit, error) {
	if strings.TrimSpace(e.Text) == "" {
		return nil, fmt.Errorf("%w: empty comment", ErrInvalid)
	}
	n, from, to, err := p.find(e)
	if err != nil {
		return nil, err
	}
	id := rand.Text()
	var b builder
	b.to(from)
	b.insert(docx.Object, ot.Attrs{"cs": id})
	b.to(to)
	b.insert(docx.Object, ot.Attrs{"ce": id})
	b.insert(docx.Object, ot.Attrs{"comment": id})
	return append(p.newComment(id, "", e.Text), b.edit(n.ID)...), nil
}

func (p *planner) reply(e Edit) (ot.Edit, error) {
	if strings.TrimSpace(e.Text) == "" {
		return nil, fmt.Errorf("%w: empty comment", ErrInvalid)
	}
	c := p.s.tree.Node(e.Comment)
	if c == nil || c.Type != "comment" {
		return nil, gone(e.Comment)
	}
	parent := c.ID
	if top := str(c.Attrs, "parent"); top != "" {
		parent = top
	}
	return p.newComment(rand.Text(), parent, e.Text), nil
}

func (p *planner) newComment(id, parent, text string) ot.Edit {
	attrs := ot.Values{"author": quote(p.name), "date": quote(p.date), "initials": quote(initials(p.name))}
	if parent != "" {
		attrs["parent"] = quote(parent)
	}
	last := ""
	if kids := p.s.tree.Children("doc"); len(kids) > 0 {
		last = kids[len(kids)-1].Key
	}
	return ot.Edit{
		{Op: ot.OpNew, ID: id, Type: "comment", Parent: "doc", Key: ot.KeyBetween(last, ""), Attrs: attrs},
		{Op: ot.OpNew, ID: rand.Text(), Type: "text", Parent: id, Key: ot.KeyBetween("", ""), Text: ot.Delta{{Insert: strings.TrimSpace(text) + "\n"}}},
	}
}

func quote(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		if r := []rune(w)[0]; !unicode.IsControl(r) && len(out) < 9 {
			out = append(out, unicode.ToUpper(r))
		}
	}
	return string(out)
}

func (p *planner) revision(e Edit) (ot.Edit, error) {
	block, _, ok := strings.Cut(e.Revision, "/")
	if !ok {
		return nil, fmt.Errorf("%w: revision %q", ErrInvalid, e.Revision)
	}
	if err := p.block(block); err != nil {
		return nil, err
	}
	revs := p.s.revisions()
	i := slices.IndexFunc(revs, func(r revision) bool { return r.id == e.Revision })
	if i < 0 {
		return nil, fmt.Errorf("%w: revision %s (accepted, rejected or rewritten)", ErrGone, e.Revision)
	}
	r := revs[i]
	n := p.s.tree.Node(r.node)
	if (e.Op == "accept") == (r.kind == "ins") {
		var b builder
		b.to(r.from)
		b.retain(r.to-r.from, ot.Attrs{r.kind: "", r.kind + "d": ""})
		return b.edit(n.ID), nil
	}
	return p.s.removeRange(n, r.from, r.to), nil
}

// unrevised clears the revisions of what it is set on.
var unrevised = ot.Attrs{"ins": "", "insd": "", "del": "", "deld": ""}

// removeRange deletes text of a flow, keeping it ended by a mark: the last
// mark goes only with the flow, which goes only when another flow ends what
// holds it.
func (s *state) removeRange(n *ot.Node, from, to int) ot.Edit {
	flow := n.Text.Delta()
	size := flowLen(flow)
	kids := s.tree.Children(n.Parent)
	before, prev := charAt(flow, from-1)
	var b builder
	switch {
	case to < size:
		b.to(from)
		b.remove(to - from)
	case from == 0 && kids[len(kids)-1] != n && kids[len(kids)-1].Text != nil:
		return ot.Edit{{Op: ot.OpDel, ID: n.ID}}
	case before == '\n':
		_, last := charAt(flow, size-1)
		become := ot.Attrs{}
		for k := range last {
			become[k] = ""
		}
		for k, v := range prev {
			become[k] = v
		}
		b.to(from - 1)
		b.remove(size - from)
		b.retain(1, become)
	default:
		b.to(from)
		b.remove(size - 1 - from)
		b.retain(1, unrevised)
	}
	return b.edit(n.ID)
}

func charAt(flow ot.Delta, pos int) (r rune, attrs ot.Attrs) {
	each(flow, pos, pos+1, func(c rune, a ot.Attrs) { r, attrs = c, a })
	return r, attrs
}

// each calls f with the characters of a flow from offset from to to.
func each(flow ot.Delta, from, to int, f func(r rune, attrs ot.Attrs)) {
	pos := 0
	for _, o := range flow {
		if pos >= to {
			return
		}
		if pos+o.Len() <= from {
			pos += o.Len()
			continue
		}
		for _, r := range o.Insert {
			if pos >= from && pos < to {
				f(r, o.Attrs)
			}
			pos += utf16.RuneLen(r)
		}
	}
}

// runKeys are the attributes of text that text typed after it takes.
var runKeys = map[string]bool{"b": true, "i": true, "u": true, "strike": true, "dstrike": true, "caps": true, "smallCaps": true,
	"vanish": true, "sz": true, "font": true, "color": true, "hl": true, "shd": true, "va": true, "r": true}

func runStyle(attrs ot.Attrs) ot.Attrs {
	out := ot.Attrs{}
	for k, v := range attrs {
		if runKeys[k] {
			out[k] = v
		}
	}
	return out
}

// with merges attributes, those cleared left out.
func with(list ...ot.Attrs) ot.Attrs {
	out := ot.Attrs{}
	for _, attrs := range list {
		for k, v := range attrs {
			out[k] = v
		}
	}
	for k, v := range out {
		if v == "" {
			delete(out, k)
		}
	}
	return out
}

// builder writes a delta from the start of a flow to its end.
type builder struct {
	d   ot.Delta
	pos int
}

func (b *builder) to(pos int) {
	b.retain(pos-b.pos, nil)
}

func (b *builder) retain(n int, attrs ot.Attrs) {
	b.d = b.d.Push(ot.Op{Retain: n, Attrs: attrs})
	b.pos += n
}

func (b *builder) insert(text string, attrs ot.Attrs) {
	b.d = b.d.Push(ot.Op{Insert: text, Attrs: attrs})
}

func (b *builder) remove(n int) {
	b.d = b.d.Push(ot.Op{Delete: n})
	b.pos += n
}

func (b *builder) edit(id string) ot.Edit {
	d := b.d
	for len(d) > 0 && d[len(d)-1].Retain > 0 && d[len(d)-1].Attrs == nil {
		d = d[:len(d)-1]
	}
	if len(d) == 0 {
		return nil
	}
	return ot.Edit{{Op: ot.OpTxt, ID: id, Text: d}}
}
