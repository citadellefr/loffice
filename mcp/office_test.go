package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice/agent"
)

// eventually waits for cond, which the edits of the server make true.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(what)
}

func decode[T any](t *testing.T, text string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("%v in %s", err, text)
	}
	return v
}

type slideList struct {
	Slides  []agent.SlideInfo
	Layouts []agent.Layout
}

func TestPresentationTools(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	u := func(args map[string]any) map[string]any { return as("u1", deckKey, args) }
	watcher := h.someone(deckKey, "Alice")

	list := decode[slideList](t, ok(t, c, "slide_list", u(nil)))
	if len(list.Slides) == 0 || len(list.Layouts) == 0 {
		t.Fatalf("%+v", list)
	}
	for _, l := range list.Layouts {
		if !strings.HasPrefix(l.ID, "L") {
			t.Fatalf("a layout that is not one: %+v", l)
		}
	}
	slide := list.Slides[0].ID
	read := decode[struct{ Slide agent.Slide }](t, ok(t, c, "slide_read", u(map[string]any{"slide": slide})))
	shape := ""
	for _, s := range read.Slide.Shapes {
		if s.Type == "sp" {
			shape = s.ID
			break
		}
	}
	if shape == "" {
		t.Fatalf("no shape with text: %+v", read.Slide)
	}
	textOf := func(a *agent.Agent, id string) string {
		_, s, err := a.Slide(slide)
		if err != nil {
			t.Fatal(err)
		}
		for _, sh := range s.Shapes {
			if sh.ID == id {
				return sh.Text
			}
		}
		return ""
	}
	before := textOf(watcher, shape)

	ok(t, c, "shape_text_set", u(map[string]any{"shape": shape, "text": "Nouveau titre\nSous-titre"}))
	eventually(t, "the room did not get the new text", func() bool { return textOf(watcher, shape) == "Nouveau titre\nSous-titre" })

	ok(t, c, "notes_set", u(map[string]any{"slide": slide, "text": "Notes de l'IA"}))
	eventually(t, "the room did not get the notes", func() bool {
		_, s, err := watcher.Slide(slide)
		return err == nil && s.Notes == "Notes de l'IA"
	})

	ok(t, c, "shape_add", u(map[string]any{"slide": slide, "box": map[string]any{"x": 50, "y": 300, "w": 400, "h": 60}, "text": "Une zone de texte"}))
	ok(t, c, "comment_add", u(map[string]any{"block": slide, "text": "À relire."}))
	got := ok(t, c, "slide_read", u(map[string]any{"slide": slide}))
	if !strings.Contains(got, "Une zone de texte") || !strings.Contains(got, "À relire.") || !strings.Contains(got, "Héraut") {
		t.Fatal(got)
	}

	ok(t, c, "slide_add", u(map[string]any{"layout": list.Layouts[0].ID, "after": slide}))
	after := decode[slideList](t, ok(t, c, "slide_list", u(nil)))
	if len(after.Slides) != len(list.Slides)+1 {
		t.Fatalf("no slide added: %+v", after)
	}
	ok(t, c, "slide_delete", u(map[string]any{"slide": after.Slides[1].ID}))
	if got := decode[slideList](t, ok(t, c, "slide_list", u(nil))); len(got.Slides) != len(list.Slides) {
		t.Fatalf("slide not deleted: %+v", got)
	}

	if msg := fails(t, c, "shape_text_set", u(map[string]any{"shape": "nope", "text": "x"})); !strings.Contains(msg, "nope") {
		t.Fatal(msg)
	}
	if msg := fails(t, c, "doc_read", u(nil)); !strings.Contains(msg, "slide_") {
		t.Fatal(msg)
	}
	preview := ok(t, c, "batch", u(map[string]any{"dry_run": true, "edits": []map[string]any{{"op": "text", "block": shape, "text": "Autre"}}}))
	if !strings.HasPrefix(preview, "Nothing changed.") || !strings.Contains(preview, "Autre") {
		t.Fatal(preview)
	}

	if got := ok(t, c, "undo_mine", u(nil)); !strings.Contains(got, "Undid") {
		t.Fatal(got)
	}
	eventually(t, "undo_mine did not take back the text", func() bool { return textOf(watcher, shape) == before })
	if got := decode[struct{ Slide agent.Slide }](t, ok(t, c, "slide_read", u(map[string]any{"slide": slide}))); len(got.Slide.Shapes) != len(read.Slide.Shapes) || len(got.Slide.Comments) != len(read.Slide.Comments) {
		t.Fatalf("undo_mine left shapes or comments: %+v", got.Slide)
	} else if got.Slide.Notes != read.Slide.Notes {
		t.Fatalf("undo_mine left the notes: %+v", got.Slide)
	}

	ok(t, c, "slide_delete", u(map[string]any{"slide": slide}))
	if got := ok(t, c, "undo_mine", u(nil)); !strings.Contains(got, "Undid 1 of") || !strings.Contains(got, "Not restored") || !strings.Contains(got, "the notes of slide "+slide) {
		t.Fatal(got)
	}
}

type rangeRead struct {
	Version string
	Range   agent.Range
}

func TestWorkbookTools(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	u := func(args map[string]any) map[string]any { return as("u1", bookKey, args) }
	watcher := h.someone(bookKey, "Alice")
	cell := func(a *agent.Agent, ref string) agent.Cell {
		_, r, err := a.Range("Feuil1", ref)
		if err != nil || len(r.Rows) != 1 || len(r.Rows[0]) != 1 {
			t.Fatalf("%s: %+v %v", ref, r, err)
		}
		return r.Rows[0][0]
	}
	value := func(ref string) any {
		return decode[rangeRead](t, ok(t, c, "range_read", u(map[string]any{"sheet": "Feuil1", "range": ref, "view": "json"}))).Range.Rows[0][0].Value
	}

	first := cell(watcher, "A1")
	if got := ok(t, c, "sheet_list", u(nil)); !strings.Contains(got, `"name": "Feuil1"`) {
		t.Fatal(got)
	}
	if got := ok(t, c, "range_read", u(map[string]any{"sheet": "Feuil1"})); !strings.Contains(got, "| 42 |") || !strings.Contains(got, "texte") {
		t.Fatal(got)
	}

	ok(t, c, "cells_set", u(map[string]any{"sheet": "Feuil1", "range": "D1", "values": [][]any{{"=B1*2", "=D1+1"}}}))
	eventually(t, "the formula was not calculated", func() bool { return value("D1") == 84.0 && value("E1") == 85.0 })
	if got := ok(t, c, "range_read", u(map[string]any{"sheet": "Feuil1", "range": "D1:E1"})); !strings.Contains(got, "=B1*2") {
		t.Fatal(got)
	}
	ok(t, c, "cells_set", u(map[string]any{"sheet": "Feuil1", "range": "B1", "values": [][]any{{10}}}))
	eventually(t, "the dependent formulas were not calculated again", func() bool {
		return cell(watcher, "B1").Value == 10.0 && cell(watcher, "D1").Value == 20.0 && cell(watcher, "E1").Value == 21.0
	})

	ok(t, c, "rows_insert", u(map[string]any{"sheet": "Feuil1", "range": "1:2"}))
	eventually(t, "rows not inserted", func() bool { return cell(watcher, "B3").Value == 10.0 && cell(watcher, "D3").Formula == "=B3*2" })
	ok(t, c, "rows_delete", u(map[string]any{"sheet": "Feuil1", "range": "1:2"}))
	ok(t, c, "columns_insert", u(map[string]any{"sheet": "Feuil1", "range": "A"}))
	eventually(t, "column not inserted", func() bool { return cell(watcher, "C1").Value == 10.0 })
	ok(t, c, "columns_delete", u(map[string]any{"sheet": "Feuil1", "range": "A"}))
	eventually(t, "column not deleted", func() bool { return cell(watcher, "B1").Value == 10.0 })

	preview := ok(t, c, "batch", u(map[string]any{"dry_run": true, "edits": []map[string]any{{"op": "cells", "sheet": "Feuil1", "range": "F1", "values": [][]any{{"aperçu"}}}}}))
	if !strings.HasPrefix(preview, "Nothing changed.") || !strings.Contains(preview, "aperçu") || value("B1") != 10.0 {
		t.Fatal(preview)
	}
	ok(t, c, "cells_set", u(map[string]any{"sheet": "Feuil1", "range": "G1:G5", "values": [][]any{{strings.Repeat("x", 30000)}, {strings.Repeat("x", 30000)}, {strings.Repeat("x", 30000)}, {strings.Repeat("x", 30000)}, {strings.Repeat("x", 30000)}}}))
	if msg := fails(t, c, "range_read", u(map[string]any{"sheet": "Feuil1"})); !strings.Contains(msg, "read less") {
		t.Fatal(msg)
	}
	if msg := fails(t, c, "rows_insert", u(map[string]any{"sheet": "Nulle", "range": "1"})); !strings.Contains(msg, "Nulle") {
		t.Fatal(msg)
	}
	if msg := fails(t, c, "cells_set", u(map[string]any{"sheet": "Feuil1", "range": "A1", "values": make([][]any, maxCells+1)})); !strings.Contains(msg, "cells at a time") {
		t.Fatal(msg)
	}

	if got := ok(t, c, "undo_mine", u(nil)); !strings.Contains(got, "Undid 7 of") {
		t.Fatal(got)
	}
	eventually(t, "undo_mine did not take back the cells", func() bool {
		return cell(watcher, "A1").Value == first.Value && cell(watcher, "B1").Value == 42.0 && cell(watcher, "C1").Value == "texte" &&
			cell(watcher, "D1").Value == nil && cell(watcher, "D1").Formula == "" && cell(watcher, "E1").Formula == "" && cell(watcher, "B3").Value == nil && cell(watcher, "G1").Value == nil
	})
}
