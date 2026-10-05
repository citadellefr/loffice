package loffice

import (
	"encoding/json"
	"fmt"
	"path"
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
	".txt":  trame.Text,
	".csv":  openCSV,
	".pptx": openPresentation,
	".pptm": openPresentation,
	".ppsx": openPresentation,
	".xlsx": openWorkbook,
	".xlsm": openWorkbook,
	".xltx": openWorkbook,
	".docx": openDocument,
	".docm": openDocument,
	".dotx": openDocument,
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

func (d document) Check(doc *ot.Tree, e ot.Edit, by Peer) error {
	return d.doc.Check(doc, e, by.Name)
}

func (d document) Encode(doc *ot.Tree) ([]byte, error) {
	return d.doc.Save(doc)
}

func (d document) addPicture(data []byte) (string, error) {
	name, err := d.doc.AddPicture(data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPicture, err)
	}
	return name, nil
}

func (d document) media(name string) ([]byte, string, error) {
	data, typ, err := d.doc.Media(name)
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

func (p presentation) Check(doc *ot.Tree, e ot.Edit, by Peer) error {
	return p.doc.Check(doc, e, by.Name)
}

func (p presentation) Encode(doc *ot.Tree) ([]byte, error) {
	return p.doc.Save(doc)
}

func (p presentation) media(name string) ([]byte, string, error) {
	data, typ, err := p.doc.Media(name)
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

func (w *workbook) Check(doc *ot.Tree, e ot.Edit, _ Peer) error {
	return w.doc.Check(doc, e)
}

func (w *workbook) Encode(doc *ot.Tree) ([]byte, error) {
	return w.doc.Save(doc)
}

// Follow calculates again what the edit reaches. A failure of the
// calculation leaves the values as they are rather than the hub down.
func (w *workbook) Follow(doc *ot.Tree, e ot.Edit, since []ot.Edit) (out ot.Edit) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	out = w.calc.Follow(e, since)
	book, ok := w.doc.(*xlsx.Document)
	if !ok {
		return out
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
	return out
}

// open reads a file into a document, in the format its key's extension
// names.
func open(key string, data []byte) (*ot.Tree, trame.File, error) {
	ext := strings.ToLower(path.Ext(key))
	read := formats[ext]
	if read == nil {
		return nil, nil, fmt.Errorf("loffice: %q files are not supported", ext)
	}
	return read(path.Base(key), data)
}
