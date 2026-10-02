package chart

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

var chartPart = regexp.MustCompile(`/charts/chart\d*\.xml$`)

// Every chart of the corpus is read, with its series and their values;
// chartex and Strict Open XML parts are told apart.
func TestCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	kinds := map[string]int{}
	charts, empty, others := 0, 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Open(data, opc.Limits{})
		if err != nil {
			continue
		}
		for _, name := range p.Names() {
			if !chartPart.MatchString(name) {
				continue
			}
			part, err := p.Read(name)
			if err != nil {
				continue
			}
			c, err := Read(part)
			if err == ErrNotChart {
				others++
				continue
			}
			if err != nil {
				t.Errorf("%s %s: %v", f, name, err)
				continue
			}
			charts++
			values := 0
			for _, pl := range c.Plots {
				kinds[pl.Kind]++
				for _, s := range pl.Series {
					if s.Val != nil {
						values += len(s.Val.Num)
					}
				}
			}
			if values == 0 {
				t.Logf("no values: %s %s", f, name)
				empty++
			}
			if len(c.JSON()) == 0 {
				t.Errorf("%s %s: not written as JSON", f, name)
			}
		}
	}
	var names []string
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		t.Logf("%s: %d", k, kinds[k])
	}
	t.Logf("%d charts, %d without values; %d parts of another kind (chartex, Strict)", charts, empty, others)
	if charts < 250 {
		t.Errorf("only %d charts read", charts)
	}
}
