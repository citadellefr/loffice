package agent

import "github.com/citadellefr/trame/ot"

func (p *planner) sheetEdit(e Edit) (ot.Edit, error) { return nil, ErrUnsupported }

func previewSheets(s *state, e ot.Edit) string { return "" }
