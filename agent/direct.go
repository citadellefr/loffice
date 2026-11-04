package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// mine is a batch the agent applied to a document that has no revisions,
// with the edits that take it back after those of the user's later
// batches, rebased over what everyone else did since.
type mine struct {
	user    string
	version uint64
	edits   []Edit
	undo    []ot.Edit
	// changed is what someone else changed since of what the batch did,
	// "" while nobody did; lost what taking it back cannot make again.
	changed string
	lost    []string
}

// Mark is what the agent changed lately, for editors to highlight a
// while: a node, or the cells of a sheet from Top, Left to Bottom, Right.
type Mark struct {
	Node  string  `json:"n"`
	Cells *[4]int `json:"c,omitempty"`
	At    int64   `json:"at"`
}

const (
	maxMarks = 32
	markLife = 2 * time.Minute
)

func (a *Agent) direct() bool {
	return a.isSlides() || a.isSheets()
}

// rebaseMine moves what takes back the agent's batches over an edit the
// hub applied, before the agent applies it. The batches of a user are
// taken back the last first, each after the next: an edit made for user
// leaves them, it goes down those of the others from the last one. A batch
// whose content it changes can no longer be taken back.
func (a *Agent) rebaseMine(e ot.Edit, user string, server bool) {
	over := map[string]ot.Edit{}
	for _, m := range slices.Backward(a.mine) {
		if m.user == user {
			continue
		}
		h, ok := over[m.user]
		if !ok {
			h = e
		}
		if !server && m.changed == "" {
			m.changed = a.s.overlap(h, m.undo)
		}
		m.undo, over[m.user] = rebase(m.undo, h)
	}
}

// rebase transforms edits applied one after the other over e, which they
// win over, and gives what e becomes once they are applied.
func rebase(edits []ot.Edit, e ot.Edit) ([]ot.Edit, ot.Edit) {
	out := make([]ot.Edit, 0, len(edits))
	for _, u := range edits {
		next := ot.TransformEdit(u, e, false)
		if u = ot.TransformEdit(e, u, true); len(u) > 0 {
			out = append(out, u)
		}
		e = next
	}
	return out, e
}

// appliedMine applies a batch of the agent as revision v, and keeps what
// takes it back.
func (a *Agent) appliedMine(e ot.Edit, v uint64, f *flight) {
	if a.s == nil || v != a.s.version+1 {
		a.fail(fmt.Errorf("revision %d out of order", v))
		return
	}
	var back ot.Edit
	for _, c := range e {
		back = append(a.s.inverse(c), back...)
		if err := a.s.change(c, v); err != nil {
			a.fail(err)
			return
		}
	}
	a.s.version = v
	var lost []string
	if a.isSlides() {
		back = slices.DeleteFunc(back, func(c ot.Change) bool {
			what := a.unrestorable(c)
			if what != "" {
				lost = append(lost, what)
			}
			return what != ""
		})
	}
	a.mine = append(a.mine, &mine{user: f.owner, version: v, edits: f.batch, undo: split(back), lost: lost})
}

// split cuts an edit where it goes from inserting or removing rows and
// columns to other changes, which a workbook takes apart.
func split(e ot.Edit) []ot.Edit {
	var out []ot.Edit
	for i, c := range e {
		if i == 0 || shifts(c) != shifts(e[i-1]) {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], c)
	}
	return out
}

func shifts(c ot.Change) bool {
	return c.Op == ot.OpIns || c.Op == ot.OpRem
}

var null = json.RawMessage("null")

// inverse are the changes that take back c, made on the tree as it is.
func (s *state) inverse(c ot.Change) ot.Edit {
	n := s.tree.Node(c.ID)
	switch c.Op {
	case ot.OpNew:
		return ot.Edit{{Op: ot.OpDel, ID: c.ID}}
	case ot.OpDel:
		if n == nil {
			return nil
		}
		return s.recreate(n, nil)
	case ot.OpSet:
		if n == nil {
			return nil
		}
		back := ot.Change{Op: ot.OpSet, ID: c.ID}
		if c.Key != "" {
			back.Key = n.Key
		}
		if len(c.Attrs) > 0 {
			back.Attrs = ot.Values{}
			for k := range c.Attrs {
				back.Attrs[k] = cmpOr(n.Attrs[k], null)
			}
		}
		return ot.Edit{back}
	case ot.OpTxt:
		if n == nil || n.Text == nil {
			return nil
		}
		return ot.Edit{{Op: ot.OpTxt, ID: c.ID, Text: invertDelta(n.Text.Delta(), c.Text)}}
	case ot.OpCel:
		if n == nil || n.Grid == nil {
			return nil
		}
		back := ot.Change{Op: ot.OpCel, ID: c.ID}
		for _, cell := range c.Cells {
			var old, set map[string]json.RawMessage
			_ = json.Unmarshal(n.Grid.Cell(cell.Row, cell.Col), &old)
			_ = json.Unmarshal(cell.Fields, &set)
			was := map[string]json.RawMessage{}
			for k := range set {
				was[k] = cmpOr(old[k], null)
			}
			fields, _ := json.Marshal(was)
			back.Cells = append(back.Cells, ot.Cell{Row: cell.Row, Col: cell.Col, Fields: fields})
		}
		return ot.Edit{back}
	case ot.OpIns:
		return ot.Edit{{Op: ot.OpRem, ID: c.ID, Dim: c.Dim, At: c.At, N: c.N}}
	case ot.OpRem:
		back := ot.Edit{{Op: ot.OpIns, ID: c.ID, Dim: c.Dim, At: c.At, N: c.N}}
		if n == nil || n.Grid == nil {
			return back
		}
		r1, r2, c1, c2 := c.At, c.At+c.N-1, 0, formula.MaxCols
		if c.Dim == ot.DimCols {
			r1, r2, c1, c2 = 0, formula.MaxRows, c.At, c.At+c.N-1
		}
		restore := ot.Change{Op: ot.OpCel, ID: c.ID}
		n.Grid.EachIn(r1, r2, c1, c2, func(row, col int, fields json.RawMessage) bool {
			restore.Cells = append(restore.Cells, ot.Cell{Row: row, Col: col, Fields: bytes.Clone(fields)})
			return true
		})
		if len(restore.Cells) > 0 {
			back = append(back, restore)
		}
		return append(back, s.broken(c, r1, r2, c1, c2)...)
	}
	return nil
}

// broken are the changes that set again the formulas of the workbook that
// removing lines c from rows r1 to r2 and columns c1 to c2 breaks, those
// that inserting the lines back does not mend.
func (s *state) broken(c ot.Change, r1, r2, c1, c2 int) ot.Edit {
	var out ot.Edit
	name := str(s.tree.Node(c.ID).Attrs, "name")
	rows := c.Dim == ot.DimRows
	for _, sheet := range s.tree.Children(s.tree.Node(c.ID).Parent) {
		if sheet.Grid == nil {
			continue
		}
		own := sheet.ID == c.ID
		set := ot.Change{Op: ot.OpCel, ID: sheet.ID}
		sheet.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
			if col == 0 || own && row >= r1 && row <= r2 && col >= c1 && col <= c2 || !bytes.Contains(raw, []byte(`"f":`)) {
				return true
			}
			var fields struct {
				F string `json:"f"`
			}
			if json.Unmarshal(raw, &fields) != nil || fields.F == "" {
				return true
			}
			gone, err := formula.Shift(fields.F, name, own, rows, c.At, -c.N)
			if err != nil {
				return true
			}
			if back, err := formula.Shift(gone, name, own, rows, c.At, c.N); err == nil && back == fields.F {
				return true
			}
			f, _ := json.Marshal(map[string]string{"f": fields.F})
			set.Cells = append(set.Cells, ot.Cell{Row: row, Col: col, Fields: f})
			return true
		})
		if len(set.Cells) > 0 {
			out = append(out, set)
		}
	}
	return out
}

func cmpOr(v, or json.RawMessage) json.RawMessage {
	if v == nil {
		return or
	}
	return v
}

// recreate are the changes that make a node again, with what it holds.
func (s *state) recreate(n *ot.Node, out ot.Edit) ot.Edit {
	c := ot.Change{Op: ot.OpNew, ID: n.ID, Type: n.Type, Parent: n.Parent, Key: n.Key, Attrs: n.Attrs}
	if n.Text != nil {
		c.Text = n.Text.Delta()
	}
	if n.Grid != nil {
		c.Cells = n.Grid.Cells()
		if c.Cells == nil {
			c.Cells = []ot.Cell{}
		}
	}
	out = append(out, c)
	for _, k := range s.tree.Children(n.ID) {
		out = s.recreate(k, out)
	}
	return out
}

// invertDelta is the delta that takes back d made on flow.
func invertDelta(flow, d ot.Delta) ot.Delta {
	var out ot.Delta
	at := 0
	for _, o := range d {
		switch {
		case o.Insert != "":
			out = out.Push(ot.Op{Delete: o.Len()})
		case o.Delete > 0:
			for _, old := range sliceFlow(flow, at, at+o.Delete) {
				out = out.Push(old)
			}
			at += o.Delete
		case o.Attrs == nil:
			out = out.Push(ot.Op{Retain: o.Retain})
			at += o.Retain
		default:
			for _, old := range sliceFlow(flow, at, at+o.Retain) {
				back := ot.Attrs{}
				for k := range o.Attrs {
					back[k] = old.Attrs[k]
				}
				out = out.Push(ot.Op{Retain: old.Len(), Attrs: back})
			}
			at += o.Retain
		}
	}
	return out
}

// sliceFlow is the part of a flow from offset from to to.
func sliceFlow(flow ot.Delta, from, to int) ot.Delta {
	var out ot.Delta
	each(flow, from, to, func(r rune, attrs ot.Attrs) {
		out = out.Push(ot.Op{Insert: string(r), Attrs: attrs})
	})
	return out
}

// overlap tells what of the content edits take back e changes, "" if
// nothing.
func (s *state) overlap(e ot.Edit, edits []ot.Edit) string {
	for _, u := range edits {
		for _, c := range u {
			for _, h := range e {
				if what := s.clash(h, c); what != "" {
					return what
				}
			}
		}
		e = ot.TransformEdit(u, e, false)
	}
	return ""
}

// clash tells what h changes of what c restores, "" if nothing.
func (s *state) clash(h, c ot.Change) string {
	id := c.ID
	if c.Op == ot.OpNew {
		id = c.Parent
	}
	if h.Op == ot.OpDel && s.under(id, h.ID) {
		return s.name(h.ID)
	}
	switch c.Op {
	case ot.OpDel:
		target := h.ID
		if h.Op == ot.OpNew {
			target = h.Parent
		}
		if s.under(target, c.ID) {
			return s.name(c.ID)
		}
	case ot.OpTxt:
		if h.Op == ot.OpTxt && h.ID == c.ID {
			return s.name(c.ID)
		}
	case ot.OpSet:
		if h.Op != ot.OpSet || h.ID != c.ID {
			break
		}
		if h.Key != "" && c.Key != "" {
			return s.name(c.ID)
		}
		for k := range h.Attrs {
			if _, ok := c.Attrs[k]; ok {
				return s.name(c.ID)
			}
		}
	case ot.OpCel:
		if h.ID != c.ID {
			break
		}
		for _, cell := range c.Cells {
			if hit(h, cell.Row, cell.Col) {
				return s.cellName(c.ID, cell.Row, cell.Col)
			}
		}
	case ot.OpRem:
		if h.ID != c.ID {
			break
		}
		from, to := c.At, c.At+c.N
		switch {
		case h.Op == ot.OpCel:
			for _, cell := range h.Cells {
				at := cell.Row
				if c.Dim == ot.DimCols {
					at = cell.Col
				}
				if at >= from && at < to {
					return s.cellName(c.ID, cell.Row, cell.Col)
				}
			}
		case shifts(h) && h.Dim == c.Dim && h.At < to && (h.Op == ot.OpRem && h.At+h.N > from || h.Op == ot.OpIns && h.At > from):
			return s.name(c.ID)
		}
	}
	return ""
}

// hit tells whether a change of cells or lines reaches a cell.
func hit(h ot.Change, row, col int) bool {
	switch h.Op {
	case ot.OpCel:
		return slices.ContainsFunc(h.Cells, func(x ot.Cell) bool { return x.Row == row && x.Col == col })
	case ot.OpRem:
		at := row
		if h.Dim == ot.DimCols {
			at = col
		}
		return at >= h.At && at < h.At+h.N
	}
	return false
}

// under tells whether node id is top or under it.
func (s *state) under(id, top string) bool {
	for n := s.tree.Node(id); n != nil; n = s.tree.Node(n.Parent) {
		if n.ID == top {
			return true
		}
	}
	return false
}

// name is how a node reads to someone.
func (s *state) name(id string) string {
	n := s.tree.Node(id)
	if n == nil {
		return id
	}
	if name := str(n.Attrs, "name"); name != "" {
		return fmt.Sprintf("%s %q (%s)", n.Type, name, id)
	}
	return n.Type + " " + id
}

func (s *state) cellName(sheet string, row, col int) string {
	switch {
	case row == 0:
		return fmt.Sprintf("column %s of %s", formula.ColumnName(col), s.name(sheet))
	case col == 0:
		return fmt.Sprintf("row %d of %s", row, s.name(sheet))
	}
	return fmt.Sprintf("%s of %s", formula.CellName(row, col), s.name(sheet))
}

// marks are what an edit changed, as editors highlight it.
func (s *state) marks(e ot.Edit, at time.Time) []Mark {
	var out []Mark
	add := func(m Mark) {
		m.At = at.UnixMilli()
		if !slices.ContainsFunc(out, func(x Mark) bool { return x.Node == m.Node && x.Cells == nil && m.Cells == nil }) {
			out = append(out, m)
		}
	}
	for _, c := range e {
		switch c.Op {
		case ot.OpNew:
			if !slices.ContainsFunc(e, func(p ot.Change) bool { return p.Op == ot.OpNew && p.ID == c.Parent }) {
				add(Mark{Node: c.ID})
			}
		case ot.OpTxt, ot.OpSet:
			add(Mark{Node: c.ID})
		case ot.OpDel:
			if s.tree.Node(c.Parent) != nil {
				add(Mark{Node: c.Parent})
			}
		case ot.OpCel:
			if box, ok := bounds(c.Cells); ok {
				add(Mark{Node: c.ID, Cells: &box})
			}
		case ot.OpIns:
			box := [4]int{c.At, 1, c.At + c.N - 1, formula.MaxCols}
			if c.Dim == ot.DimCols {
				box = [4]int{1, c.At, formula.MaxRows, c.At + c.N - 1}
			}
			add(Mark{Node: c.ID, Cells: &box})
		}
	}
	return out
}

// bounds is the area cells cover, without the lines.
func bounds(cells []ot.Cell) ([4]int, bool) {
	box, ok := [4]int{formula.MaxRows, formula.MaxCols, 0, 0}, false
	for _, c := range cells {
		if c.Row == 0 || c.Col == 0 {
			continue
		}
		box = [4]int{min(box[0], c.Row), min(box[1], c.Col), max(box[2], c.Row), max(box[3], c.Col)}
		ok = true
	}
	return box, ok
}

// mark keeps what an edit of the agent changed, and moves its selection
// there. Callers hold a.mu.
func (a *Agent) mark(e ot.Edit) {
	now := a.opt.Now()
	a.recent = slices.DeleteFunc(a.recent, func(m Mark) bool { return now.Sub(time.UnixMilli(m.At)) > markLife })
	a.recent = append(a.recent, a.s.marks(e, now)...)
	if len(a.recent) > maxMarks {
		a.recent = a.recent[len(a.recent)-maxMarks:]
	}
	if sel, ok := a.selectionAfter(e); ok {
		a.selection = sel
	}
}

// selectionAfter is where the agent's selection goes after its edit.
func (a *Agent) selectionAfter(e ot.Edit) (any, bool) {
	if c, ok := cursorAfter(e); ok && a.s.tree.Node(c.Node) != nil {
		return c, true
	}
	marks := a.s.marks(e, time.Time{})
	for i := len(marks) - 1; i >= 0; i-- {
		if m := marks[i]; m.Cells != nil {
			return cellSelection{m.Node, *m.Cells}, true
		}
	}
	return nil, false
}

// cellSelection is a selection of cells of a sheet, as editors share it.
type cellSelection struct {
	Node  string `json:"n"`
	Cells [4]int `json:"c"`
}

// textLen is the length of a text in UTF-16 code units, as flows count.
func textLen(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// previewDirect tells what a batch would do in a document without
// revisions, changing nothing.
func (a *Agent) previewDirect(b Batch) (Preview, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return Preview{}, a.err
	}
	e, work, err := a.plan(b, a.date())
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Version: a.version()}
	if a.isSlides() {
		out.Markup = previewSlides(a.s, work, e)
	} else {
		out.Markup = previewSheets(work, e)
	}
	return out, nil
}

// undone follows an edit of the agent that took back part of a batch:
// what is left to take back of it, and of the batches of other users.
func (a *Agent) undone(e ot.Edit, v uint64, f *flight) {
	m := f.undoing
	if len(m.undo) > 0 {
		m.undo = m.undo[1:]
	}
	a.rebaseMine(e, f.owner, false)
	if len(m.undo) == 0 {
		a.mine = slices.DeleteFunc(a.mine, func(o *mine) bool { return o == m })
	}
	a.applied(e, v)
}

// undoDirect takes back the batches of the agent for user, the last first,
// in a document without revisions.
func (a *Agent) undoDirect(ctx context.Context, user string) (Record, error) {
	if strings.TrimSpace(user) == "" {
		return Record{}, fmt.Errorf("%w: no user to act on behalf of", ErrInvalid)
	}
	if a.opt.Allow != nil {
		if err := a.opt.Allow(ctx, Batch{OnBehalfOf: user, Edits: []Edit{{Op: "undo"}}}); err != nil {
			return Record{}, fmt.Errorf("%w: %w", ErrDenied, err)
		}
	}
	a.sending.Lock()
	unlock := a.sending.Unlock
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	rec := Record{OnBehalfOf: user, Date: a.opt.Now().UTC().Truncate(time.Second)}
	batches := 0
	for {
		if err := context.Cause(ctx); err != nil {
			return rec, err
		}
		a.mu.Lock()
		if a.err != nil {
			a.mu.Unlock()
			return rec, a.err
		}
		var last *mine
		for _, m := range a.mine {
			if m.user != user {
				continue
			}
			if m.changed != "" {
				a.mu.Unlock()
				return rec, a.refuse(m, batches)
			}
			last = m
		}
		if last == nil {
			a.mu.Unlock()
			if batches > 0 {
				_ = a.publish()
			}
			return rec, nil
		}
		if len(last.undo) == 0 {
			a.mine = slices.DeleteFunc(a.mine, func(o *mine) bool { return o == last })
			a.mu.Unlock()
			continue
		}
		f, err := a.submit(ctx, &flight{edit: last.undo[0], owner: user, undoing: last})
		if f != nil && err != nil {
			unlock = nil
			go func() {
				defer a.sending.Unlock()
				<-f.done
			}()
		}
		if err != nil {
			if batches > 0 {
				err = fmt.Errorf("%d of your batches taken back, then: %w", batches, err)
			}
			return rec, err
		}
		rec.Version = a.versionOf(f.version)
		a.mu.Lock()
		taken := len(last.undo) == 0
		a.mu.Unlock()
		if taken {
			rec.Edits = append(rec.Edits, last.edits...)
			rec.Lost = append(rec.Lost, last.lost...)
			batches++
		}
	}
}

// refuse is the error of an UndoMine that meets a batch someone else
// changed since.
func (a *Agent) refuse(m *mine, batches int) error {
	err := fmt.Errorf("%w: %s, which the batch of version %s changed", ErrChanged, m.changed, a.versionOf(m.version))
	if batches > 0 {
		err = fmt.Errorf("%d later batches taken back, then: %w", batches, err)
	}
	return err
}
