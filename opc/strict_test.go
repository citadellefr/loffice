package opc

import "testing"

func TestPercents(t *testing.T) {
	strict := `<a:solidFill xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:schemeClr val="accent1"><a:lumMod val="12.5%"/><a:tint val="-3%"/></a:schemeClr><a:ext val="50%"/></a:solidFill>`
	transitional := `<a:solidFill xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:schemeClr val="accent1"><a:lumMod val="12500"/><a:tint val="-3000"/></a:schemeClr><a:ext val="50%"/></a:solidFill>`
	if got := string(percents([]byte(strict), false)); got != transitional {
		t.Errorf("read as\n%s", got)
	}
	if got := string(percents([]byte(transitional), true)); got != strict {
		t.Errorf("written as\n%s", got)
	}
	other := `<x:lumMod xmlns:x="urn:other" val="12.5%"/>`
	if got := string(percents([]byte(other), false)); got != other {
		t.Errorf("other namespace read as %s", got)
	}
}
