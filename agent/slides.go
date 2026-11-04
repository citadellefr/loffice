package agent

import "github.com/citadellefr/trame/ot"

func (p *planner) slideEdit(e Edit) (ot.Edit, error) { return nil, ErrUnsupported }

func (a *Agent) restorable(c ot.Change) bool { return true }

func previewSlides(before, after *state, e ot.Edit) string { return "" }
