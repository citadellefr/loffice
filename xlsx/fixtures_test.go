package xlsx

import (
	"bytes"
	"encoding/json"
	goflag "flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
)

var update = goflag.Bool("update", false, "rewrite the trees in testdata/xlsx")

// fixtures are workbooks of Apache POI (Apache 2.0) whose trees the Dart
// package lays out on pages in its tests.
var fixtures = []string{"47737.xlsx", "49156.xlsx", "WithChart.xlsx"}

func TestFixtures(t *testing.T) {
	for _, name := range fixtures {
		data, err := os.ReadFile(filepath.Join("../corpus/files/poi", name))
		if err != nil {
			t.Skip("no corpus: corpus/fetch.sh")
		}
		_, tree, err := Open(data)
		if err != nil {
			t.Fatal(err)
		}
		NewCalc(tree, formula.Options{})
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(tree.Edit()); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("../testdata/xlsx", strings.TrimSuffix(name, ".xlsx")+".json")
		if *update {
			if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, b.Bytes()) {
			t.Fatalf("%s is stale: go test ./xlsx -run Fixtures -update", path)
		}
	}
}
