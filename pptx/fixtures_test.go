package pptx

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/partrel"
)

var update = flag.Bool("update", false, "rewrite the trees in testdata/pptx")

// fixtures are presentations of python-pptx (MIT) whose trees the Dart
// package draws in its tests.
var fixtures = []string{"shp-shapes.pptx", "ph-populated-placeholders.pptx", "txt-text.pptx", "dml-fill.pptx", "cht-chart-type.pptx", "tbl-cell.pptx"}

func TestFixtures(t *testing.T) {
	for _, name := range fixtures {
		data, err := os.ReadFile(filepath.Join("../corpus/files/python-pptx", name))
		if err != nil {
			t.Skip("no corpus: corpus/fetch.sh")
		}
		_, tree, err := Open(data)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(tree.Edit()); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("../testdata/pptx", strings.TrimSuffix(name, ".pptx")+".json")
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, b.Bytes()) {
			t.Fatalf("%s is stale: go test ./pptx -run Fixtures -update", path)
		}
	}
}

// TestDump writes the trees and pictures of presentations to $LOFFICE_DUMP,
// for the Dart package to draw them: $LOFFICE_DUMP_FILES lists them,
// relative to corpus/files.
func TestDump(t *testing.T) {
	dir := os.Getenv("LOFFICE_DUMP")
	if dir == "" {
		t.Skip("LOFFICE_DUMP not set")
	}
	for _, name := range strings.Split(os.Getenv("LOFFICE_DUMP_FILES"), ",") {
		data, err := os.ReadFile(filepath.Join("../corpus/files", name))
		if err != nil {
			t.Fatal(err)
		}
		d, tree, err := Open(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		base := filepath.Join(dir, strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
		nodes, _ := json.Marshal(tree.Edit())
		if err := os.WriteFile(base+".json", nodes, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(base, 0o755); err != nil {
			t.Fatal(err)
		}
		for n, r := range d.names.All() {
			if r.Type != partrel.Image || r.External {
				continue
			}
			picture, _, err := d.Media(strings.TrimPrefix(n, "@"))
			if err == nil {
				_ = os.WriteFile(filepath.Join(base, strings.TrimPrefix(n, "@")), picture, 0o644)
			}
		}
	}
}
