package agent

import (
	"maps"
	"slices"
	"strconv"

	"github.com/citadellefr/trame/ot"
)

// state is the document as the agent has it: the tree, the ids it gives
// blocks, and the revision that last changed each of them.
//
// A paragraph is known by its mark: it keeps its id while its mark stays,
// whatever is typed in it or around it. Splitting one gives the first half
// a new id; joining two keeps the id of the second, unless all of its text
// went too. Tables keep theirs until they are deleted.
type state struct {
	tree    *ot.Tree
	version uint64
	next    int
	// paras are the ids and changes of the paragraphs of each flow, in
	// order; where is the flow of each paragraph id.
	paras map[string][]para
	where map[string]string
	// tables are the ids of the tables, by node, and nodes the reverse.
	tables map[string]string
	nodes  map[string]string
	// mod is the revision that last changed a node or what is under it.
	mod map[string]uint64
	// cells are the revisions that last changed the cells of each sheet,
	// and shifted the last that inserted or removed its rows or columns.
	cells   map[string]map[[2]int]uint64
	shifted map[string]uint64
}

type para struct {
	id  string
	mod uint64
}

func newState(tree *ot.Tree, version uint64) *state {
	s := &state{
		tree:    tree,
		version: version,
		paras:   map[string][]para{},
		where:   map[string]string{},
		tables:  map[string]string{},
		nodes:   map[string]string{},
		mod:     map[string]uint64{},
		cells:   map[string]map[[2]int]uint64{},
		shifted: map[string]uint64{},
	}
	var walk func(parent string)
	walk = func(parent string) {
		for _, n := range tree.Children(parent) {
			s.added(n, 0)
			walk(n.ID)
		}
	}
	walk("")
	return s
}

func (s *state) clone() *state {
	c := *s
	c.tree = s.tree.Clone()
	c.paras = make(map[string][]para, len(s.paras))
	for id, list := range s.paras {
		c.paras[id] = slices.Clone(list)
	}
	c.where = maps.Clone(s.where)
	c.tables = maps.Clone(s.tables)
	c.nodes = maps.Clone(s.nodes)
	c.mod = maps.Clone(s.mod)
	c.cells = make(map[string]map[[2]int]uint64, len(s.cells))
	for id, cells := range s.cells {
		c.cells[id] = maps.Clone(cells)
	}
	c.shifted = maps.Clone(s.shifted)
	return &c
}

func (s *state) newID(prefix string) string {
	s.next++
	return prefix + strconv.Itoa(s.next)
}

// added names what a new node holds.
func (s *state) added(n *ot.Node, v uint64) {
	switch {
	case n.Text != nil:
		list := make([]para, len(n.Text.Paragraphs()))
		for i := range list {
			list[i] = para{id: s.newID("p"), mod: v}
			s.where[list[i].id] = n.ID
		}
		s.paras[n.ID] = list
	case n.Type == "tbl":
		id := s.newID("t")
		s.tables[n.ID], s.nodes[id] = id, n.ID
	}
}

func (s *state) removed(id string) {
	for _, p := range s.paras[id] {
		delete(s.where, p.id)
	}
	delete(s.paras, id)
	if t, ok := s.tables[id]; ok {
		delete(s.nodes, t)
		delete(s.tables, id)
	}
	delete(s.mod, id)
	delete(s.cells, id)
	delete(s.shifted, id)
}

// touch records that revision v changed node id, and so its ancestors.
func (s *state) touch(id string, v uint64) {
	for n := s.tree.Node(id); n != nil; n = s.tree.Node(n.Parent) {
		s.mod[n.ID] = v
	}
}

// apply makes the edit revision v. The edit is one the hub applied: it
// applies to the tree.
func (s *state) apply(e ot.Edit, v uint64) error {
	for _, c := range e {
		if err := s.change(c, v); err != nil {
			return err
		}
	}
	s.version = v
	return nil
}

func (s *state) change(c ot.Change, v uint64) error {
	switch c.Op {
	case ot.OpNew:
		if err := s.tree.Apply(ot.Edit{c}); err != nil {
			return err
		}
		if n := s.tree.Node(c.ID); n != nil {
			s.added(n, v)
			s.touch(c.ID, v)
		}
	case ot.OpDel:
		n := s.tree.Node(c.ID)
		if n == nil {
			return nil
		}
		gone := s.subtree(c.ID, nil)
		s.touch(n.Parent, v)
		if err := s.tree.Apply(ot.Edit{c}); err != nil {
			return err
		}
		for _, id := range gone {
			s.removed(id)
		}
	case ot.OpTxt:
		n := s.tree.Node(c.ID)
		if n == nil || n.Text == nil {
			return s.tree.Apply(ot.Edit{c})
		}
		old := n.Text.Paragraphs()
		if err := s.tree.Apply(ot.Edit{c}); err != nil {
			return err
		}
		if s.follow(c.ID, old, c.Text, v) {
			s.touch(c.ID, v)
		}
	default:
		if err := s.tree.Apply(ot.Edit{c}); err != nil {
			return err
		}
		if s.tree.Node(c.ID) == nil {
			return nil
		}
		s.touch(c.ID, v)
		switch c.Op {
		case ot.OpCel:
			if s.cells[c.ID] == nil {
				s.cells[c.ID] = map[[2]int]uint64{}
			}
			for _, cell := range c.Cells {
				s.cells[c.ID][[2]int{cell.Row, cell.Col}] = v
			}
		case ot.OpIns, ot.OpRem:
			s.shifted[c.ID] = v
			delete(s.cells, c.ID)
		}
	}
	return nil
}

// cellChanged is the revision that last changed a cell of a sheet, or
// moved it.
func (s *state) cellChanged(sheet string, row, col int) uint64 {
	return max(s.cells[sheet][[2]int{row, col}], s.shifted[sheet])
}

func (s *state) subtree(id string, out []string) []string {
	out = append(out, id)
	for _, k := range s.tree.Children(id) {
		out = s.subtree(k.ID, out)
	}
	return out
}

// follow moves the ids of the paragraphs of a flow over the delta that
// changed it, and tells whether any paragraph changed.
func (s *state) follow(id string, old []ot.Delta, d ot.Delta, v uint64) bool {
	now := s.tree.Node(id).Text.Paragraphs()
	before := s.paras[id]
	was := marks(old)
	moved := make([]int, len(was))
	for i, m := range was {
		moved[i] = follows(d, m)
	}
	list := make([]para, len(now))
	changed := false
	i, at := 0, 0
	for j, p := range now {
		at += flowLen(p)
		for i < len(moved) && (moved[i] < 0 || moved[i] < at-1) {
			delete(s.where, before[i].id)
			changed = true
			i++
		}
		if i < len(moved) && moved[i] == at-1 {
			list[j] = before[i]
			if i > 0 && moved[i-1] < 0 && emptied(d, was[i-1]+1, was[i]) {
				delete(s.where, before[i].id)
				list[j] = before[i-1]
				s.where[list[j].id] = id
			}
			if !same(old[i], p) {
				list[j].mod, changed = v, true
			}
			i++
			continue
		}
		list[j] = para{id: s.newID("p"), mod: v}
		s.where[list[j].id] = id
		changed = true
	}
	for ; i < len(before); i++ {
		delete(s.where, before[i].id)
	}
	s.paras[id] = list
	return changed
}

// emptied tells whether d deleted all from offset from to to.
func emptied(d ot.Delta, from, to int) bool {
	for pos := from; pos < to; pos++ {
		if follows(d, pos) >= 0 {
			return false
		}
	}
	return true
}

// marks are the offsets of the marks of paragraphs.
func marks(paras []ot.Delta) []int {
	out := make([]int, len(paras))
	at := 0
	for i, p := range paras {
		at += flowLen(p)
		out[i] = at - 1
	}
	return out
}

// follows is where the unit at offset pos of a flow is once d changed it,
// -1 when d deleted it.
func follows(d ot.Delta, pos int) int {
	from, to := 0, 0
	for _, o := range d {
		switch {
		case o.Insert != "":
			to += o.Len()
		case o.Delete > 0:
			if pos < from+o.Delete {
				return -1
			}
			from += o.Delete
		default:
			if pos < from+o.Retain {
				return to + pos - from
			}
			from += o.Retain
			to += o.Retain
		}
	}
	return to + pos - from
}

// same tells whether two paragraphs hold the same.
func same(a, b ot.Delta) bool {
	if len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0]) {
		return true
	}
	return slices.EqualFunc(a, b, func(x, y ot.Op) bool {
		return x.Insert == y.Insert && maps.Equal(x.Attrs, y.Attrs)
	})
}

func flowLen(d ot.Delta) int {
	n := 0
	for _, o := range d {
		n += o.Len()
	}
	return n
}

// paragraph finds a paragraph by id: its flow, its index there and where
// it starts.
func (s *state) paragraph(id string) (node *ot.Node, index, start int, ok bool) {
	node = s.tree.Node(s.where[id])
	if node == nil || node.Text == nil {
		return nil, 0, 0, false
	}
	paras := node.Text.Paragraphs()
	for i, p := range s.paras[node.ID] {
		if p.id == id {
			return node, i, start, true
		}
		start += flowLen(paras[i])
	}
	return nil, 0, 0, false
}

// changed tells the revision that last changed a block, and whether it
// exists.
func (s *state) changed(id string) (uint64, bool) {
	if node, i, _, ok := s.paragraph(id); ok {
		return s.paras[node.ID][i].mod, true
	}
	if n, ok := s.nodes[id]; ok {
		return s.mod[n], true
	}
	return 0, false
}
