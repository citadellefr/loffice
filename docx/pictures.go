package docx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
)

// MaxPicture is the size of the largest picture a client may add.
const MaxPicture = 20 << 20

var ErrPicture = errors.New("docx: pictures are PNG, JPEG or GIF files of 20 MB at most")

// extensions are the pictures a client may add, by content type.
var extensions = map[string]string{"image/png": "png", "image/jpeg": "jpeg", "image/gif": "gif"}

// pending is a picture a client added, written into the package when a
// drawing shows it.
type pending struct {
	data        []byte
	contentType string
}

// AddPicture keeps a picture a client sends, for its drawings to show: its
// name, what an "img" gives as its media.
func (d *Document) AddPicture(data []byte) (string, error) {
	contentType := http.DetectContentType(data)
	ext := extensions[contentType]
	if ext == "" || len(data) > MaxPicture {
		return "", ErrPicture
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for n := 1; ; n++ {
		target := fmt.Sprintf("word/media/bref%d.%s", n, ext)
		if d.pkg.Has(target) || d.pending[target] != nil {
			continue
		}
		name := d.names.Picture(target, data)
		if r, _ := d.names.Lookup(name); r.Target == target {
			d.pending[target] = &pending{data, contentType}
		}
		return strings.TrimPrefix(name, "@"), nil
	}
}

// drawing is the w:drawing of a picture a client inserted, in the line.
func (w *writer) drawing(img string) *xmldom.Element {
	var p Picture
	if json.Unmarshal([]byte(img), &p) != nil || p.W <= 0 || p.H <= 0 || p.W > 1<<31 || p.H > 1<<31 {
		return nil
	}
	if r, ok := w.d.names.Lookup("@" + p.Media); !ok || r.Type != partrel.Image {
		return nil
	}
	cx, cy := strconv.FormatInt(p.W, 10), strconv.FormatInt(p.H, 10)
	w.drawings++
	id := strconv.Itoa(w.drawings)
	e, err := xmldom.ParseFragment([]byte(`<w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="`+cx+`" cy="`+cy+`"/><wp:docPr id="`+id+`" name="Image `+id+`"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic>`+
		`<pic:nvPicPr><pic:cNvPr id="0" name="Image"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="@`+p.Media+`"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="`+cx+`" cy="`+cy+`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing>`), standardSpaces())
	if err != nil {
		return nil
	}
	if p.Descr != "" {
		docPr := find(e, wpNS, "docPr")
		docPr.Set("descr", p.Descr)
	}
	return e
}

// lastDrawing is the largest id of the drawings of a part.
func lastDrawing(root *xmldom.Element) int {
	last := 0
	var walk func(e *xmldom.Element)
	walk = func(e *xmldom.Element) {
		if e.Space == wpNS && e.Local == "docPr" {
			if n, err := strconv.Atoi(e.Get("id")); err == nil {
				last = max(last, n)
			}
		}
		for _, c := range e.Elements() {
			walk(c)
		}
	}
	walk(root)
	return last
}

// addPictures writes into the package the pictures clients added that a
// drawing of the tree shows.
func (w *writer) addPictures() error {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	if len(w.d.pending) == 0 {
		return nil
	}
	shown := map[string]bool{}
	for _, n := range w.tree.Edit() {
		for _, o := range n.Text {
			var p Picture
			if img := o.Attrs["img"]; img != "" && json.Unmarshal([]byte(img), &p) == nil {
				if r, ok := w.d.names.Lookup("@" + p.Media); ok {
					shown[r.Target] = true
				}
			}
		}
	}
	for target, p := range w.d.pending {
		if shown[target] && !w.pkg.Has(target) {
			if err := w.pkg.Add(target, p.contentType, p.data); err != nil {
				return err
			}
		}
	}
	return nil
}
