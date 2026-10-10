package xlsx

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// workbook writes the list of sheets, the names defined, the active
// sheet, the date system when it is no longer the one read, and asks
// Excel to calculate again when cells were rewritten.
func (w *writer) workbook(sheets []sheet, rewritten bool) error {
	root := w.d.book.Root.Clone()
	list := child(root, "sheets", workbookOrder)
	old := map[int]*xmldom.Element{}
	for _, e := range elements(list, "sheet") {
		old[atoi(e.Get("sheetId"))] = e
	}
	rel := "r"
	if p, ok := prefixFor(root, relNS); ok {
		rel = p
	} else {
		root.Set("xmlns:r", relNS)
	}
	list.Content = nil
	index := map[string]int{}
	used := map[string]bool{}
	visible := -1
	for i, s := range sheets {
		e := old[s.part.sheetID]
		if e == nil {
			e = xmldom.New(mainNS, prefixOf(list)+"sheet", "name", "", "sheetId", strconv.Itoa(s.part.sheetID), rel+":id", s.part.rid)
		}
		e.Set("name", sheetName(str(s.node, "name"), i, used))
		state := str(s.node, "state")
		if state != "hidden" && state != "veryHidden" {
			state = ""
			if visible < 0 {
				visible = i
			}
		}
		setAttr(e, "state", state)
		list.Append(e)
		index[s.node.ID] = i
	}
	if visible < 0 {
		elements(list, "sheet")[0].Unset("state")
		visible = 0
	}

	book := w.tree.Node("book")
	active, ok := index[str(book, "active")]
	if !ok || str(sheets[active].node, "state") != "" {
		active = visible
	}
	if views := root.Child(mainNS, "bookViews"); views != nil {
		if view := views.Child(mainNS, "workbookView"); view != nil {
			setAttr(view, "activeTab", strconv.Itoa(active))
			if active == 0 {
				view.Unset("activeTab")
			}
			if atoi(view.Get("firstSheet")) > active {
				view.Unset("firstSheet")
			}
		}
	}

	var date1904 bool
	_ = json.Unmarshal(book.Attrs["date1904"], &date1904)
	if date1904 != w.d.date1904 {
		setAttr(child(root, "workbookPr", workbookOrder), "date1904", boolAttr(date1904))
	}
	w.definedNames(root, book, index)
	if rewritten {
		child(root, "calcPr", workbookOrder).Set("fullCalcOnLoad", "1")
	}
	doc := &xmldom.Document{Prolog: w.d.book.Prolog, Root: root, Tail: w.d.book.Tail}
	return w.put(w.d.bookName, "", doc.Bytes())
}

// definedNames writes the names of the workbook, keeping what the model
// does not tell of those read. A name local to a sheet that is gone goes
// with it.
func (w *writer) definedNames(root *xmldom.Element, book *ot.Node, index map[string]int) {
	type key struct{ name, sheet string }
	old := map[key][]*xmldom.Element{}
	defs := root.Child(mainNS, "definedNames")
	if defs != nil {
		for _, e := range elements(defs, "definedName") {
			k := key{strings.ToLower(e.Get("name")), ""}
			if local, ok := e.Attr("localSheetId"); ok {
				if i := atoi(local); i < len(w.d.order) {
					k.sheet = w.d.order[i]
				}
			}
			old[k] = append(old[k], e)
		}
		root.Remove(defs)
	}
	var names []Name
	_ = json.Unmarshal(book.Attrs["names"], &names)
	if len(names) == 0 {
		return
	}
	defs = child(root, "definedNames", workbookOrder)
	for _, n := range names {
		i, local := index[n.Sheet]
		if n.Sheet != "" && !local {
			continue
		}
		k := key{strings.ToLower(n.Name), n.Sheet}
		var e *xmldom.Element
		if same := old[k]; len(same) > 0 {
			e, old[k] = same[0], same[1:]
		} else {
			e = xmldom.New(mainNS, prefixOf(defs)+"definedName")
		}
		e.Set("name", n.Name)
		if local {
			e.Set("localSheetId", strconv.Itoa(i))
		} else {
			e.Unset("localSheetId")
		}
		setAttr(e, "hidden", boolAttr(n.Hidden))
		e.Content = []xmldom.Node{xmldom.EscapeText(n.Ref)}
		defs.Append(e)
	}
}

// prefixFor is the prefix a namespace is declared under.
func prefixFor(e *xmldom.Element, space string) (string, bool) {
	for p, s := range e.Spaces() {
		if s == space && p != "" {
			return p, true
		}
	}
	return "", false
}

// sheetName is a name Excel takes for the sheet at index i, unused by
// those before it.
func sheetName(name string, i int, used map[string]bool) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '_'
		}
		return r
	}, strings.Trim(name, "'"))
	if runes := []rune(name); len(runes) > 31 {
		name = string(runes[:31])
	}
	if name == "" {
		name = "Feuil" + strconv.Itoa(i+1)
	}
	base := []rune(name)
	for n := 2; used[strings.ToLower(name)]; n++ {
		suffix := " (" + strconv.Itoa(n) + ")"
		name = string(base[:min(len(base), 31-len(suffix))]) + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}
