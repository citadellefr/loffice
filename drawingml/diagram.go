package drawingml

import (
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// DiagramNS is the namespace of the drawing PowerPoint and Word record of
// a SmartArt.
const DiagramNS = "http://schemas.microsoft.com/office/drawing/2008/diagram"

// Diagram is the drawing recorded of a SmartArt: its shapes, laid out and
// formatted, in EMU from the corner of the graphic. It is drawn and not
// edited.
type Diagram struct {
	Shapes []*DiagramShape `json:"shapes"`
}

// DiagramShape is a shape or a picture of a diagram, with the attributes
// the shapes of a slide have, and the box of its text.
type DiagramShape struct {
	Xfrm  *Xfrm            `json:"xfrm"`
	Geom  *Geometry        `json:"geom,omitempty"`
	Fill  *Fill            `json:"fill,omitempty"`
	Line  *Line            `json:"line,omitempty"`
	Style *Style           `json:"style,omitempty"`
	Blip  *Blip            `json:"blip,omitempty"`
	Body  Props            `json:"body,omitempty"`
	Lst   map[string]Props `json:"lst,omitempty"`
	Text  ot.Delta         `json:"text,omitempty"`
	// TextXfrm is where the text goes, when it is not in the shape's box.
	TextXfrm *Xfrm `json:"txXfrm,omitempty"`
}

// ReadDiagram reads a dsp:drawing; media names the picture a relationship
// id points to. It is nil when the drawing shows nothing.
func ReadDiagram(root *xmldom.Element, media func(rid string) string) *Diagram {
	d := &Diagram{}
	if tree := diagramChild(root, "spTree"); tree != nil {
		d.shapes(tree, media)
	}
	if len(d.Shapes) == 0 {
		return nil
	}
	return d
}

func diagramChild(e *xmldom.Element, local string) *xmldom.Element {
	return e.Child(DiagramNS, local)
}

// shapes reads the shapes under a group, those of its subgroups after
// them.
func (d *Diagram) shapes(group *xmldom.Element, media func(rid string) string) {
	for _, e := range group.Elements() {
		if e.Space != DiagramNS {
			continue
		}
		switch e.Local {
		case "grpSp":
			d.shapes(e, media)
		case "sp", "pic":
			if s := readDiagramShape(e, media); s != nil {
				d.Shapes = append(d.Shapes, s)
			}
		}
	}
}

func readDiagramShape(e *xmldom.Element, media func(rid string) string) *DiagramShape {
	pr := diagramChild(e, "spPr")
	s := &DiagramShape{Xfrm: ReadXfrm(child(pr, "xfrm")), Style: ReadStyle(diagramChild(e, "style")), Blip: ReadBlip(diagramChild(e, "blipFill"), media)}
	if s.Xfrm == nil {
		return nil
	}
	s.Geom, s.Fill, s.Line = GeometryIn(pr), FillIn(pr, media), ReadLine(child(pr, "ln"))
	if tx := diagramChild(e, "txBody"); tx != nil {
		s.Body = BodyProps(child(tx, "bodyPr"))
		s.Lst = ListStyle(child(tx, "lstStyle"))
		s.Text = plainFlow(Flow(tx, func(string) string { return "" }))
		s.TextXfrm = ReadXfrm(diagramChild(e, "txXfrm"))
	}
	return s
}

// plainFlow is a flow without the XML its runs and paragraphs were read
// from, nil when it has no text.
func plainFlow(flow ot.Delta) ot.Delta {
	var text bool
	out := make(ot.Delta, len(flow))
	for i, op := range flow {
		attrs := ot.Attrs{}
		for k, v := range op.Attrs {
			if k != "r" && k != "p" && k != "fld" && k != "o" {
				attrs[k] = v
			}
		}
		if len(attrs) == 0 {
			attrs = nil
		}
		out[i] = ot.Op{Insert: op.Insert, Attrs: attrs}
		text = text || op.Insert != "\n"
	}
	if !text {
		return nil
	}
	return out
}
