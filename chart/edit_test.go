package chart

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
)

func numbers(v ...float64) []*float64 {
	out := make([]*float64, len(v))
	for i := range v {
		out[i] = &v[i]
	}
	return out
}

func sample(kind, dir string) *Chart {
	c := &Chart{
		Title:  &Title{Text: "Ventes"},
		Legend: &Legend{Pos: "r"},
		Plots: []Plot{{Kind: kind, Dir: dir, Series: []Series{
			{Index: 0, Order: 0, Name: &Data{Ref: "Feuil1!$B$1", Count: 1, Str: []string{"Nord"}},
				Cat: &Data{Ref: "Feuil1!$A$2:$A$4", Count: 3, Str: []string{"T1", "T2", "T3"}},
				Val: &Data{Ref: "Feuil1!$B$2:$B$4", Count: 3, Num: numbers(1, 2.5, -4)}},
			{Index: 1, Order: 1, Name: &Data{Ref: "Feuil1!$C$1", Count: 1, Str: []string{"Sud"}},
				Cat: &Data{Ref: "Feuil1!$A$2:$A$4", Count: 3, Str: []string{"T1", "T2", "T3"}},
				Val: &Data{Ref: "Feuil1!$C$2:$C$4", Count: 3, Num: numbers(3, 4, 5)}},
		}}},
	}
	Normalize(c)
	return c
}

func readBytes(t *testing.T, data []byte) *Chart {
	t.Helper()
	c, err := Read(data)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	return c
}

func TestNew(t *testing.T) {
	for _, k := range []struct{ kind, dir, axes string }{
		{"bar", "col", "cat b val l"}, {"bar", "bar", "cat l val b"}, {"line", "", "cat b val l"},
		{"pie", "", ""}, {"scatter", "", "val b val l"}, {"area", "", "cat b val l"},
	} {
		c := readBytes(t, New(sample(k.kind, k.dir)))
		if len(c.Plots) != 1 || c.Plots[0].Kind != k.kind || k.dir != "" && c.Plots[0].Dir != k.dir {
			t.Errorf("%s: plots %+v", k.kind, c.Plots)
			continue
		}
		var axes []string
		for _, a := range c.Axes {
			axes = append(axes, a.Kind, a.Pos)
		}
		if strings.Join(axes, " ") != k.axes {
			t.Errorf("%s: axes %v", k.kind, axes)
		}
		s := c.Plots[0].Series[1]
		if c.Title.Text != "Ventes" || c.Legend.Pos != "r" || s.Name.Str[0] != "Sud" || s.Val.Ref != "Feuil1!$C$2:$C$4" || *s.Val.Num[2] != 5 ||
			s.Cat.Str[1] != "T2" {
			t.Errorf("%s: %+v %+v", k.kind, c.Title, s)
		}
	}
}

func patched(t *testing.T, data []byte, change func(c *Chart)) (*Chart, []byte) {
	t.Helper()
	doc, err := xmldom.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := readBytes(t, data)
	change(want)
	Normalize(want)
	Patch(doc.Root, want)
	out := doc.Bytes()
	return readBytes(t, out), out
}

// between is the part of data between from and to.
func between(data []byte, from, to string) string {
	s := string(data)
	i := strings.Index(s, from)
	j := strings.Index(s, to)
	if i < 0 || j < i {
		return ""
	}
	return s[i:j]
}

func TestPatchKeeps(t *testing.T) {
	data, err := os.ReadFile("testdata/bar.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, out := patched(t, data, func(*Chart) {})
	if !bytes.Equal(out, data) {
		t.Errorf("unedited chart rewritten:\n%s", out)
	}

	c, out := patched(t, data, func(c *Chart) {
		c.Title.Text = "Chiffre"
		c.Legend.Pos = "b"
		c.Axes[1].Title = &Title{Text: "Euros"}
	})
	if c.Title.Text != "Chiffre" || c.Title.Props["b"] != "1" || c.Legend.Pos != "b" || c.Axes[1].Title.Text != "Euros" {
		t.Errorf("title, legend: %+v %+v %+v", c.Title, c.Legend, c.Axes[1].Title)
	}
	if a, b := between(data, "<c:barChart>", "</c:barChart>"), between(out, "<c:barChart>", "</c:barChart>"); a == "" || a != b {
		t.Errorf("plot rewritten:\n%s\n%s", a, b)
	}

	c, out = patched(t, data, func(c *Chart) {
		c.Title, c.Legend = nil, nil
		c.Plots[0].Series[0].Val.Ref = "Sheet1!$B$2:$E$2"
	})
	if c.Title != nil || c.Legend != nil || c.Plots[0].Series[0].Val.Ref != "Sheet1!$B$2:$E$2" || !strings.Contains(string(out), `<c:autoTitleDeleted val="1"/>`) {
		t.Errorf("removed: %+v %+v %s", c.Title, c.Legend, out)
	}
	if a, b := between(data, "<c:numCache>", "</c:numCache>"), between(out, "<c:numCache>", "</c:numCache>"); a != b {
		t.Errorf("cache rewritten:\n%s\n%s", a, b)
	}
}

func TestPatchKind(t *testing.T) {
	data := New(sample("bar", "col"))
	for _, k := range []struct{ kind, dir, axes string }{
		{"line", "", "cat b val l"}, {"bar", "bar", "cat l val b"}, {"scatter", "", "val b val l"}, {"pie", "", ""},
		{"area", "", "cat b val l"}, {"bar", "col", "cat b val l"},
	} {
		var out []byte
		var c *Chart
		c, out = patched(t, data, func(c *Chart) { c.Plots[0].Kind, c.Plots[0].Dir = k.kind, k.dir })
		data = out
		var axes []string
		for _, a := range c.Axes {
			axes = append(axes, a.Kind, a.Pos)
		}
		if (k.kind == "line" || k.kind == "area") && c.Plots[0].Grouping != "standard" {
			t.Errorf("%s: grouping %s", k.kind, c.Plots[0].Grouping)
		}
		if c.Plots[0].Kind != k.kind || strings.Join(axes, " ") != k.axes || len(c.Plots[0].Series) != 2 || *c.Plots[0].Series[1].Val.Num[1] != 4 ||
			c.Plots[0].Series[0].Name.Str[0] != "Nord" {
			t.Errorf("%s: %+v %v", k.kind, c.Plots[0], axes)
		}
	}
}

func TestPatchSeries(t *testing.T) {
	data := New(sample("line", ""))
	c, _ := patched(t, data, func(c *Chart) {
		s := c.Plots[0].Series
		c.Plots[0].Series = []Series{s[1], {Val: &Data{Ref: "Feuil1!$D$2:$D$4", Count: 3, Num: numbers(7, 8, 9)}}}
	})
	s := c.Plots[0].Series
	if len(s) != 2 || s[0].Val.Ref != "Feuil1!$C$2:$C$4" || s[1].Index != 1 || s[1].Name != nil || *s[1].Val.Num[0] != 7 {
		t.Errorf("series: %+v", s)
	}
}
