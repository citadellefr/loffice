package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/trame/ot"
)

// View is how a document reads.
type View int

const (
	// Markdown is the text as it would be with every revision accepted,
	// comments left out.
	Markdown View = iota
	// Critic shows the revisions and comments in CriticMarkup: {++added++},
	// {--deleted--}, {==commented==}{>>[id] author: comment<<}.
	Critic
)

// Heading is a title of the outline of a document.
type Heading struct {
	Block string `json:"block"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Match is text found in a block, Offset runes into its text as Markdown
// reads it without its markup.
type Match struct {
	Block   string `json:"block"`
	Offset  int    `json:"offset"`
	Text    string `json:"text"`
	Excerpt string `json:"excerpt"`
}

// Revision is a tracked change: text inserted or deleted by its author.
// OnBehalfOf is the user the agent made it for, for its own.
type Revision struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Author     string    `json:"author"`
	Date       time.Time `json:"date,omitzero"`
	Block      string    `json:"block"`
	Text       string    `json:"text"`
	OnBehalfOf string    `json:"onBehalfOf,omitempty"`
}

// Read is the document in a view, each block on lines of its own after its
// id in brackets, and its version.
func (a *Agent) Read(view View) (version, text string, err error) {
	err = a.word(func(s *state) error {
		version, text = a.version(), newReader(s, view).document()
		return nil
	})
	return version, text, err
}

// Outline are the headings of the document.
func (a *Agent) Outline() (version string, headings []Heading, err error) {
	err = a.word(func(s *state) error {
		version = a.version()
		r := newReader(s, Markdown)
		s.eachParagraph("body", func(id string, n *ot.Node, p ot.Delta, _ int, nested bool) {
			if level := r.level(mark(p)); level > 0 && !nested {
				headings = append(headings, Heading{id, level, plain(p)})
			}
		})
		return nil
	})
	return version, headings, err
}

// Search finds text, or a regular expression, in the blocks of the body.
func (a *Agent) Search(query string, regex bool) (version string, matches []Match, err error) {
	if !regex {
		query = regexp.QuoteMeta(query)
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	err = a.word(func(s *state) error {
		version = a.version()
		s.eachParagraph("body", func(id string, _ *ot.Node, p ot.Delta, _ int, _ bool) {
			text := plain(p)
			for _, m := range re.FindAllStringIndex(text, -1) {
				if m[0] == m[1] {
					continue
				}
				matches = append(matches, Match{id, runes(text[:m[0]]), text[m[0]:m[1]], excerpt(text, m[0], m[1])})
			}
		})
		return nil
	})
	return version, matches, err
}

// Revisions are the tracked changes of the body.
func (a *Agent) Revisions() (version string, revs []Revision, err error) {
	err = a.word(func(s *state) error {
		version = a.version()
		for _, r := range s.revisions() {
			revs = append(revs, a.describe(r))
		}
		return nil
	})
	return version, revs, err
}

func (a *Agent) describe(r revision) Revision {
	out := Revision{ID: r.id, Kind: r.kind, Author: r.author, Block: r.block, Text: r.text}
	out.Date, _ = time.Parse(time.RFC3339, r.date)
	if r.author == a.peer.Name {
		out.OnBehalfOf = docx.BehalfOf(a.s.tree, r.author, r.date)
	}
	return out
}

// word runs f on the document if it is a Word one.
func (a *Agent) word(f func(s *state) error) error {
	if !a.isWord() {
		return ErrUnsupported
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	return f(a.s)
}

func (a *Agent) isWord() bool {
	switch a.kind {
	case ".docx", ".docm", ".dotx", ".dotm":
		return true
	}
	return false
}

// eachParagraph calls f with the paragraphs under parent, in order, and
// whether they are in a table.
func (s *state) eachParagraph(parent string, f func(id string, n *ot.Node, p ot.Delta, start int, nested bool)) {
	var walk func(parent string, nested bool)
	walk = func(parent string, nested bool) {
		for _, n := range s.tree.Children(parent) {
			switch {
			case n.Text != nil:
				start := 0
				for i, p := range n.Text.Paragraphs() {
					f(s.paras[n.ID][i].id, n, p, start, nested)
					start += flowLen(p)
				}
			case n.Type == "comment":
			default:
				walk(n.ID, nested || n.Type == "tbl")
			}
		}
	}
	walk(parent, false)
}

// mark are the attributes of the mark of a paragraph.
func mark(p ot.Delta) ot.Attrs {
	return p[len(p)-1].Attrs
}

// content is a paragraph without its mark.
func content(p ot.Delta) ot.Delta {
	last := p[len(p)-1]
	out := slices.Clip(p[:len(p)-1])
	if text := strings.TrimSuffix(last.Insert, "\n"); text != "" {
		out = append(out, ot.Op{Insert: text, Attrs: last.Attrs})
	}
	return out
}

// visible tells whether a character shows in the text as Markdown reads
// it.
func visible(r rune, a ot.Attrs) bool {
	return a["del"] == "" && r != []rune(docx.Object)[0]
}

// plain is the text of a paragraph as Markdown reads it without markup.
func plain(p ot.Delta) string {
	var b strings.Builder
	for _, o := range content(p) {
		for _, r := range o.Insert {
			if visible(r, o.Attrs) {
				b.WriteRune(char(r))
			}
		}
	}
	return b.String()
}

func char(r rune) rune {
	if r == '\v' {
		return '\n'
	}
	return r
}

// span is where runes of the plain text of a paragraph are in its flow.
type span struct {
	text []rune
	// at are the offsets in the flow of each rune, and of the end.
	at []int
}

func spanOf(p ot.Delta, start int) span {
	var s span
	at := start
	for _, o := range content(p) {
		for _, r := range o.Insert {
			if visible(r, o.Attrs) {
				s.text = append(s.text, char(r))
				s.at = append(s.at, at)
			}
			at += utf16.RuneLen(r)
		}
	}
	s.at = append(s.at, at)
	return s
}

// find is where the nth occurrence of text is in a paragraph's flow, the
// whole of its text when text is empty.
func (s span) find(text string, nth int) (from, to int, ok bool) {
	if text == "" {
		if len(s.text) == 0 {
			return s.at[0], s.at[0], true
		}
		return s.at[0], s.at[len(s.text)-1] + utf16.RuneLen(s.text[len(s.text)-1]), true
	}
	hay, needle := string(s.text), []rune(text)
	at := 0
	for i := range max(nth, 1) {
		k := strings.Index(hay[at:], text)
		if k < 0 {
			return 0, 0, false
		}
		at += k
		if i < max(nth, 1)-1 {
			at += len(text)
		}
	}
	r := runes(hay[:at])
	last := r + len(needle) - 1
	return s.at[r], s.at[last] + utf16.RuneLen(s.text[last]), true
}

func runes(s string) int {
	return len([]rune(s))
}

func excerpt(text string, from, to int) string {
	r := []rune(text)
	a, b := runes(text[:from]), runes(text[:to])
	lo, hi := max(0, a-30), min(len(r), b+30)
	out := string(r[lo:hi])
	if lo > 0 {
		out = "…" + out
	}
	if hi < len(r) {
		out += "…"
	}
	return out
}

// reader writes a document as Markdown.
type reader struct {
	s         *state
	critic    bool
	styles    map[string]*docx.Style
	numbering map[string]*docx.Numbering
	// answers are the comments answering each comment; told the comments
	// already written.
	answers map[string][]*ot.Node
	told    map[string]bool
	// only is the revision whose blocks are written, all when 0.
	only uint64
}

func newReader(s *state, view View) *reader {
	r := &reader{s: s, critic: view == Critic, answers: map[string][]*ot.Node{}, told: map[string]bool{}}
	if doc := s.tree.Node("doc"); doc != nil {
		_ = json.Unmarshal(doc.Attrs["styles"], &r.styles)
		_ = json.Unmarshal(doc.Attrs["numbering"], &r.numbering)
	}
	for _, n := range s.tree.Children("doc") {
		if n.Type == "comment" {
			parent := str(n.Attrs, "parent")
			r.answers[parent] = append(r.answers[parent], n)
		}
	}
	return r
}

func str(v ot.Values, key string) string {
	var s string
	_ = json.Unmarshal(v[key], &s)
	return s
}

func (r *reader) document() string {
	var b strings.Builder
	r.blocks("body", &b)
	return b.String()
}

func (r *reader) blocks(parent string, b *strings.Builder) {
	for _, n := range r.s.tree.Children(parent) {
		switch {
		case n.Text != nil:
			for i, p := range n.Text.Paragraphs() {
				id := r.s.paras[n.ID][i]
				if line, ok := r.paragraph(p); ok && (r.only == 0 || id.mod == r.only) {
					b.WriteString(strings.TrimSuffix("["+id.id+"] "+line, " ") + "\n")
				}
			}
		case n.Type == "tbl":
			if r.only == 0 || r.s.mod[n.ID] == r.only {
				r.table(n, b)
			}
		case n.Type == "sdt":
			r.blocks(n.ID, b)
		}
	}
}

func (r *reader) table(n *ot.Node, b *strings.Builder) {
	b.WriteString("[" + r.s.tables[n.ID] + "]\n")
	for i, row := range r.s.tree.Children(n.ID) {
		if row.Type != "tr" {
			continue
		}
		var cells []string
		for _, cell := range r.s.tree.Children(row.ID) {
			if cell.Type != "tc" {
				continue
			}
			var parts []string
			r.s.eachParagraph(cell.ID, func(id string, _ *ot.Node, p ot.Delta, _ int, _ bool) {
				if line, ok := r.paragraph(p); ok {
					parts = append(parts, strings.ReplaceAll("["+id+"] "+line, "|", `\|`))
				}
			})
			cells = append(cells, strings.Join(parts, " <br> "))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			b.WriteString(strings.Repeat("| --- ", len(cells)) + "|\n")
		}
	}
}

// paragraph is a paragraph as a line of Markdown, none for one deleted
// when revisions are not shown.
func (r *reader) paragraph(p ot.Delta) (string, bool) {
	m := mark(p)
	text := r.inline(content(p))
	if !r.critic && m["del"] != "" && text == "" {
		return "", false
	}
	if r.critic {
		switch {
		case m["del"] != "" && !strings.HasSuffix(text, "--}"):
			text += "{--¶--}"
		case m["ins"] != "" && m["del"] == "" && !strings.HasSuffix(text, "++}"):
			text += "{++¶++}"
		}
	}
	prefix := ""
	if level := r.level(m); level > 0 {
		prefix = strings.Repeat("#", level) + " "
	} else if num := m["num"]; num != "" && num != "0" {
		lvl, _ := strconv.Atoi(m["lvl"])
		prefix = strings.Repeat("  ", lvl) + "- "
		if n := r.numbering[num]; n != nil && lvl < len(n.Levels) && n.Levels[lvl] != nil && n.Levels[lvl].Fmt != "bullet" && n.Levels[lvl].Fmt != "" {
			prefix = strings.Repeat("  ", lvl) + "1. "
		}
	}
	return prefix + text, true
}

// level is the level of a heading, 0 for a paragraph of text.
func (r *reader) level(m ot.Attrs) int {
	outline := m["outline"]
	for id, seen := m["pstyle"], 0; outline == "" && id != "" && seen < 16; seen++ {
		st := r.styles[id]
		if st == nil {
			break
		}
		outline, id = st.P["outline"], st.BasedOn
	}
	n, err := strconv.Atoi(outline)
	if err != nil || n < 0 || n > 8 {
		return 0
	}
	return n + 1
}

// look is how a run of text is written.
type look struct {
	rev          string
	bold, italic bool
}

func (r *reader) inline(ops ot.Delta) string {
	var b strings.Builder
	var cur look
	to := func(l look) {
		if cur.italic && (!l.italic || l.bold != cur.bold || l.rev != cur.rev) {
			b.WriteString("*")
			cur.italic = false
		}
		if cur.bold && (!l.bold || l.rev != cur.rev) {
			b.WriteString("**")
			cur.bold = false
		}
		if l.rev != cur.rev {
			b.WriteString(map[string]string{"ins": "++}", "del": "--}"}[cur.rev] + map[string]string{"ins": "{++", "del": "{--"}[l.rev])
		}
		if l.bold && !cur.bold {
			b.WriteString("**")
		}
		if l.italic && !cur.italic {
			b.WriteString("*")
		}
		cur = l
	}
	closeAll := func() { to(look{}) }
	for _, o := range ops {
		for _, c := range o.Insert {
			if c == []rune(docx.Object)[0] {
				if r.critic {
					r.anchor(o.Attrs, &b, closeAll)
				}
				continue
			}
			if !r.critic && o.Attrs["del"] != "" {
				continue
			}
			l := look{bold: o.Attrs["b"] == "1", italic: o.Attrs["i"] == "1"}
			if r.critic {
				switch {
				case o.Attrs["del"] != "":
					l.rev = "del"
				case o.Attrs["ins"] != "":
					l.rev = "ins"
				}
			}
			to(l)
			if c == '\v' {
				b.WriteString("<br>")
			} else {
				b.WriteRune(c)
			}
		}
	}
	closeAll()
	return b.String()
}

// anchor writes where a comment starts, ends or is referred to.
func (r *reader) anchor(a ot.Attrs, b *strings.Builder, closeAll func()) {
	switch {
	case a["cs"] != "":
		closeAll()
		b.WriteString("{==")
	case a["ce"] != "":
		closeAll()
		b.WriteString("==}")
		r.comment(a["ce"], b)
	case a["comment"] != "":
		closeAll()
		r.comment(a["comment"], b)
	}
}

func (r *reader) comment(id string, b *strings.Builder) {
	if r.told[id] || r.s.tree.Node(id) == nil {
		return
	}
	r.told[id] = true
	var parts []string
	var add func(n *ot.Node)
	add = func(n *ot.Node) {
		var text []string
		r.s.eachParagraph(n.ID, func(_ string, _ *ot.Node, p ot.Delta, _ int, _ bool) {
			text = append(text, plain(p))
		})
		parts = append(parts, "["+n.ID+"] "+str(n.Attrs, "author")+": "+strings.TrimSpace(strings.Join(text, " ")))
		for _, answer := range r.answers[n.ID] {
			add(answer)
		}
	}
	add(r.s.tree.Node(id))
	b.WriteString("{>>" + strings.Join(parts, " | ") + "<<}")
}

// revision is a run of text a tracked change made, its offsets those of
// its flow.
type revision struct {
	id, kind, author, date string
	block, node            string
	from, to               int
	text                   string
}

// revisions are those of the body, each paragraph numbering its own after
// its id: p3/1, p3/2.
func (s *state) revisions() []revision {
	var out []revision
	s.eachParagraph("body", func(id string, n *ot.Node, p ot.Delta, start int, _ bool) {
		out = append(out, revisionsOf(id, n.ID, p, start)...)
	})
	return out
}

func revisionsOf(block, node string, p ot.Delta, start int) []revision {
	var out []revision
	open := map[string]int{}
	at := start
	for _, o := range p {
		for _, kind := range []string{"ins", "del"} {
			author, date := o.Attrs[kind], o.Attrs[kind+"d"]
			if i, ok := open[kind]; ok && (out[i].author != author || out[i].date != date) {
				delete(open, kind)
			}
			if author == "" {
				continue
			}
			i, ok := open[kind]
			if !ok {
				out = append(out, revision{kind: kind, author: author, date: date, block: block, node: node, from: at, to: at})
				i = len(out) - 1
				open[kind] = i
			}
			out[i].to = at + o.Len()
			out[i].text += strings.ReplaceAll(o.Insert, docx.Object, "")
		}
		at += o.Len()
	}
	for i := range out {
		out[i].id = block + "/" + strconv.Itoa(i+1)
	}
	return out
}
