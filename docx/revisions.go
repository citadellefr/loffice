package docx

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/ot"
)

// Tracked changes: text and paragraph marks inserted or deleted carry
// "ins" or "del", their author, and "insd" or "deld", their date. Those
// read ride in "wrap" too; a client tracking its own changes sets the keys
// alone and the writer makes the elements. A revision whose author the
// keys no longer name was accepted or rejected: its element goes.

// revisionKinds are the elements of revisions of text, by the key they
// set.
var revisionKinds = map[string]string{"ins": "ins", "moveTo": "ins", "del": "del", "moveFrom": "del"}

// revisionDates are the keys of the dates of revisions, by the key of
// their author.
var revisionDates = map[string]string{"ins": "insd", "del": "deld"}

const typeSettings = "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"

// settingsOrder is the sequence of settings.xml up to trackRevisions.
var settingsOrder = []string{
	"writeProtection", "view", "zoom", "removePersonalInformation", "removeDateAndTime",
	"doNotDisplayPageBoundaries", "displayBackgroundShape", "printPostScriptOverText",
	"printFractionalCharacterWidth", "printFormsData", "embedTrueTypeFonts", "embedSystemFonts",
	"saveSubsetFonts", "saveFormsData", "mirrorMargins", "alignBordersAndEdges",
	"bordersDoNotSurroundHeader", "bordersDoNotSurroundFooter", "gutterAtTop", "hideSpellingErrors",
	"hideGrammaticalErrors", "activeWritingStyle", "proofState", "formsDesign", "attachedTemplate",
	"linkStyles", "stylePaneFormatFilter", "stylePaneSortMethod", "documentType", "mailMerge",
	"revisionView", "trackRevisions",
}

// readRevision adds to attrs the revision e is, if it is one.
func readRevision(attrs ot.Attrs, e *xmldom.Element) {
	kind := revisionKinds[e.Local]
	if e.Space != NS || kind == "" {
		return
	}
	if a := attr(e, "author"); a != "" {
		attrs[kind] = a
	}
	if d := attr(e, "date"); d != "" {
		attrs[revisionDates[kind]] = d
	}
}

// revisionOf is the kind and author of the revision a wrap read stands
// for, "" when it is none.
func (w *writer) revisionOf(wrap string) (kind, author string) {
	if r, ok := w.revs[wrap]; ok {
		return r[0], r[1]
	}
	if e := w.d.fragment(wrap); e != nil && e.Space == NS && revisionKinds[e.Local] != "" {
		kind, author = revisionKinds[e.Local], attr(e, "author")
	}
	if w.revs == nil {
		w.revs = map[string][2]string{}
	}
	w.revs[wrap] = [2]string{kind, author}
	return kind, author
}

// revised are the elements around an item as its keys tell: without the
// revisions read that they no longer name, with those they name and no
// element makes, innermost.
func (w *writer) revised(wraps []string, a ot.Attrs) []string {
	have := map[string]bool{}
	out := wraps[:0:0]
	for _, s := range wraps {
		if kind, author := w.revisionOf(s); kind != "" {
			if a[kind] != author {
				continue
			}
			have[kind] = true
		}
		out = append(out, s)
	}
	for _, kind := range []string{"ins", "del"} {
		if a[kind] != "" && !have[kind] {
			out = append(out, "\x00"+kind+a[kind]+"\x00"+a[revisionDates[kind]])
		}
	}
	return out
}

// revision is a w:ins or w:del a client asked for, its id given when the
// part is written.
func (w *writer) revision(in *xmldom.Element, kind, author, date string) *xmldom.Element {
	e := newW(in, kind, "author", author)
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		setAttr(e, "date", t.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if w.made == nil {
		w.made = map[*xmldom.Element]bool{}
	}
	w.made[e] = true
	return e
}

// madeRevision is the element of a wrap made for a revision, nil if the
// wrap is not one.
func (w *writer) madeRevision(wrap string, in *xmldom.Element) *xmldom.Element {
	for _, kind := range []string{"ins", "del"} {
		if rest, ok := strings.CutPrefix(wrap, "\x00"+kind); ok {
			author, date, _ := strings.Cut(rest, "\x00")
			return w.revision(in, kind, author, date)
		}
	}
	return nil
}

// markRevisions writes the revisions of a paragraph mark into the rPr of
// its pPr.
func (w *writer) markRevisions(pPr *xmldom.Element, mark ot.Attrs) {
	rPr := child(pPr, "rPr")
	have := map[string]bool{}
	for _, c := range elements(rPr) {
		kind := revisionKinds[c.Local]
		if c.Space != NS || kind == "" {
			continue
		}
		if mark[kind] == attr(c, "author") {
			have[kind] = true
		} else if rPr.Remove(c); len(rPr.Content) == 0 {
			pPr.Remove(rPr)
			rPr = nil
		}
	}
	for _, kind := range []string{"ins", "del"} {
		if mark[kind] == "" || have[kind] {
			continue
		}
		if rPr == nil {
			rPr = newW(pPr, "rPr")
			insert(pPr, rPr, pPrOrder)
		}
		insert(rPr, w.revision(rPr, kind, mark[kind], mark[revisionDates[kind]]), rPrOrder)
	}
}

// signed tells whether the revisions a change of a flow sets are the
// author's, or copied with the element they were read in.
func (d *Document) signed(delta ot.Delta, author string) bool {
	for _, op := range delta {
		for kind, date := range revisionDates {
			who := op.Attrs[kind]
			if who == "" {
				if op.Attrs[date] != "" {
					return false
				}
				continue
			}
			if op.Attrs[date] != "" {
				if _, err := time.Parse(time.RFC3339, op.Attrs[date]); err != nil {
					return false
				}
			}
			if who == author && strings.IndexFunc(author, unicode.IsControl) < 0 || op.Insert != "" && d.readRevision(op.Attrs["wrap"], kind, who) {
				continue
			}
			return false
		}
	}
	return true
}

// readRevision tells whether wrap holds a revision of the document of
// that kind and author.
func (d *Document) readRevision(wrap, kind, author string) bool {
	var wraps []string
	if json.Unmarshal([]byte(wrap), &wraps) != nil {
		return false
	}
	for _, s := range wraps {
		if e := d.fragment(s); e != nil && e.Space == NS && revisionKinds[e.Local] == kind && attr(e, "author") == author {
			return true
		}
	}
	return false
}

// settings writes whether changes are tracked into settings.xml, when a
// client turned it on or off.
func (w *writer) settings() error {
	track := tracking(w.tree)
	if track == tracking(w.d.loaded) {
		return nil
	}
	name := w.d.mainRels.OfType(w.d.main, relSettings)
	var doc *xmldom.Document
	if name != "" {
		data, err := w.pkg.Read(name)
		if err != nil {
			return err
		}
		if doc, err = xmldom.Parse(data); err != nil {
			return err
		}
	} else {
		name = "word/settings.xml"
		root := xmldom.New(NS, "w:settings")
		root.Set("xmlns:w", NS)
		doc = &xmldom.Document{Prolog: []byte(xmlHeader), Root: root}
	}
	remove(doc.Root, "trackRevisions")
	if track {
		insert(doc.Root, newW(doc.Root, "trackRevisions"), settingsOrder)
	}
	return w.write(name, typeSettings, doc.Bytes(), relSettings)
}

// tracking tells whether the document of a tree tracks changes.
func tracking(tree *ot.Tree) bool {
	return string(tree.Node("doc").Attrs["track"]) == "true"
}
