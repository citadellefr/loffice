package loffice

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/pptx"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
)

// mediaFile is a document that serves its pictures.
type mediaFile interface {
	media(name string) ([]byte, string, error)
}

// pictureAdder is a format a client adds pictures to, for its drawings.
type pictureAdder interface {
	addPicture(data []byte) (string, error)
}

// formats read files into documents, by extension; name is the file's.
var formats = map[string]trame.Format{
	".txt":  openText,
	".csv":  openCSV,
	".pptx": openPresentation,
	".pptm": openPresentation,
	".ppsx": openPresentation,
	".potx": openPresentation,
	".potm": openPresentation,
	".xlsx": openWorkbook,
	".xlsm": openWorkbook,
	".xltx": openWorkbook,
	".xltm": openWorkbook,
	".docx": openDocument,
	".docm": openDocument,
	".dotx": openDocument,
	".dotm": openDocument,
}

// guard, deferred, turns a panic into the error its function returns,
// with the stack logged: a malformed file must not take the server down.
func guard(what string, err *error) {
	if r := recover(); r != nil {
		slog.Error("loffice: panic", "in", what, "panic", r, "stack", string(debug.Stack()))
		*err = fmt.Errorf("loffice: %s: panic: %v", what, r)
	}
}

// text is a plain text file.
type text struct {
	trame.File
}

func openText(key string, data []byte) (*ot.Tree, trame.File, error) {
	tree, f, err := trame.Text(key, data)
	if err != nil {
		return nil, nil, err
	}
	return tree, text{f}, nil
}

func (t text) Check(doc *ot.Tree, e ot.Edit, by Peer) (err error) {
	defer guard("check", &err)
	return t.File.Check(doc, e, by)
}

func (t text) Encode(doc *ot.Tree) (data []byte, err error) {
	defer guard("encode", &err)
	return t.File.Encode(doc)
}

// document is a Word file.
type document struct {
	doc *docx.Document
}

func openDocument(_ string, data []byte) (*ot.Tree, trame.File, error) {
	doc, tree, err := docx.Open(data)
	if err != nil {
		return nil, nil, err
	}
	return tree, document{doc}, nil
}

func (d document) Check(doc *ot.Tree, e ot.Edit, by Peer) (err error) {
	defer guard("check", &err)
	return d.doc.Check(doc, e, by.Name)
}

func (d document) Encode(doc *ot.Tree) (data []byte, err error) {
	defer guard("encode", &err)
	return d.doc.Save(doc)
}

func (d document) addPicture(data []byte) (name string, err error) {
	defer guard("add picture", &err)
	name, err = d.doc.AddPicture(data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPicture, err)
	}
	return name, nil
}

func (d document) media(name string) (data []byte, typ string, err error) {
	defer guard("media", &err)
	data, typ, err = d.doc.Media(name)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrNoMedia, err)
	}
	return data, typ, nil
}

// presentation is a PowerPoint file.
type presentation struct {
	doc *pptx.Document
}

func openPresentation(_ string, data []byte) (*ot.Tree, trame.File, error) {
	doc, tree, err := pptx.Open(data)
	if err != nil {
		return nil, nil, err
	}
	return tree, presentation{doc}, nil
}

func (p presentation) Check(doc *ot.Tree, e ot.Edit, by Peer) (err error) {
	defer guard("check", &err)
	return p.doc.Check(doc, e, by.Name)
}

func (p presentation) Encode(doc *ot.Tree) (data []byte, err error) {
	defer guard("encode", &err)
	return p.doc.Save(doc)
}

func (p presentation) media(name string) (data []byte, typ string, err error) {
	defer guard("media", &err)
	data, typ, err = p.doc.Media(name)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrNoMedia, err)
	}
	return data, typ, nil
}

// workbook is an Excel or CSV file, whose formulas the hub calculates.
type workbook struct {
	doc interface {
		Check(tree *ot.Tree, e ot.Edit) error
		Save(tree *ot.Tree) ([]byte, error)
	}
	calc *xlsx.Calc
	// stale is set when the calculation failed and could not start over:
	// the engine and the values cannot be trusted until a Refresh succeeds.
	stale bool
}

func openWorkbook(_ string, data []byte) (*ot.Tree, trame.File, error) {
	doc, tree, err := xlsx.Open(data)
	if err != nil {
		return nil, nil, err
	}
	return tree, &workbook{doc: doc, calc: xlsx.NewCalc(tree, formula.Options{})}, nil
}

func openCSV(name string, data []byte) (*ot.Tree, trame.File, error) {
	doc, tree, err := xlsx.OpenCSV(data, name)
	if err != nil {
		return nil, nil, err
	}
	return tree, &workbook{doc: doc, calc: xlsx.NewCalc(tree, formula.Options{})}, nil
}

func (w *workbook) Check(doc *ot.Tree, e ot.Edit, _ Peer) (err error) {
	defer guard("check", &err)
	return w.doc.Check(doc, e)
}

func (w *workbook) Encode(doc *ot.Tree) (data []byte, err error) {
	defer guard("encode", &err)
	return w.doc.Save(doc)
}

var _ trame.Follower = (*workbook)(nil)

// Follow calculates again what the edit reaches. The edit is taken and
// answered already when the calculation fails: the engine then starts over
// from the tree and every formula is calculated again, and when that fails
// too the workbook stays stale, refreshed before the next edit.
func (w *workbook) Follow(doc *ot.Tree, e ot.Edit, since []ot.Edit, _ trame.Peer) ot.Edit {
	var out ot.Edit
	if w.stale {
		out = w.refresh()
		if w.stale {
			return out
		}
	}
	more, err := w.follow(doc, e, since)
	if err != nil {
		return w.refresh()
	}
	return append(out, more...)
}

func (w *workbook) follow(doc *ot.Tree, e ot.Edit, since []ot.Edit) (out ot.Edit, err error) {
	defer guard("follow", &err)
	out = w.calc.Follow(e, since)
	book, ok := w.doc.(*xlsx.Document)
	if !ok {
		return out, nil
	}
	for _, c := range e {
		if c.Op == ot.OpIns || c.Op == ot.OpRem {
			moves := ot.Change{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"moves": json.RawMessage(strconv.Itoa(book.Moved(e)))}}
			if doc.Apply(ot.Edit{moves}) == nil {
				out = append(out, moves)
			}
			break
		}
	}
	return out, nil
}

// refresh calculates every formula again, and tells whether the workbook
// is stale by w.stale.
func (w *workbook) refresh() (out ot.Edit) {
	var err error
	defer func() { w.stale = err != nil }()
	defer guard("refresh", &err)
	return w.calc.Refresh()
}

// open reads a file into a document, in the format its key's extension
// names.
func open(key string, data []byte) (tree *ot.Tree, f trame.File, err error) {
	defer guard("open", &err)
	ext := strings.ToLower(path.Ext(key))
	read := formats[ext]
	if read == nil {
		return nil, nil, fmt.Errorf("loffice: %q files are not supported", ext)
	}
	return read(path.Base(key), data)
}
