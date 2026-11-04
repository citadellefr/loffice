package agent

import "github.com/citadellefr/trame/ot"

// SlideInfo is a slide of a presentation, Index counted from 1.
type SlideInfo struct {
	ID     string `json:"id"`
	Index  int    `json:"index"`
	Layout string `json:"layout,omitempty"`
	Title  string `json:"title,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// Layout is a slide layout a new slide can take.
type Layout struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Shape is a shape of a slide and its text, a paragraph a line; Shapes
// are those of a group.
type Shape struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Name        string  `json:"name,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
	Box         *Box    `json:"box,omitempty"`
	Text        string  `json:"text,omitempty"`
	Shapes      []Shape `json:"shapes,omitempty"`
}

// SlideComment is a comment of a slide.
type SlideComment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

// Slide is a slide with its shapes, notes and comments.
type Slide struct {
	SlideInfo
	Shapes   []Shape        `json:"shapes"`
	Notes    string         `json:"notes,omitempty"`
	Comments []SlideComment `json:"comments,omitempty"`
}

// Slides are the slides of a presentation and the layouts new ones take.
func (a *Agent) Slides() (version string, slides []SlideInfo, layouts []Layout, err error) {
	return "", nil, nil, ErrUnsupported
}

// Slide is the slide id with all it holds.
func (a *Agent) Slide(id string) (version string, slide Slide, err error) {
	return "", Slide{}, ErrUnsupported
}

func (p *planner) slideEdit(e Edit) (ot.Edit, error) { return nil, ErrUnsupported }

func (a *Agent) restorable(c ot.Change) bool { return true }

func previewSlides(before, after *state, e ot.Edit) string { return "" }
