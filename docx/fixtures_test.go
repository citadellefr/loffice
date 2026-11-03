package docx

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

var update = flag.Bool("update", false, "rewrite the trees in testdata/docx")

// fixtures are documents of python-docx (MIT) whose trees the Dart package
// lays out and edits in its tests.
var fixtures = []string{"par-known-styles.docx", "tbl-having-applied-style.docx", "num-having-numbering-part.docx", "hdr-header-footer.docx", "comments-rich-para.docx"}

func TestFixtures(t *testing.T) {
	for _, name := range fixtures {
		data, err := os.ReadFile(filepath.Join("../corpus/files/python-docx", name))
		if err != nil {
			t.Skip("no corpus: go run ./tools/corpus")
		}
		_, tree, err := Open(data, opc.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(tree.Edit()); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("../testdata/docx", strings.TrimSuffix(name, ".docx")+".json")
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
			t.Fatalf("%s is stale: go test ./docx -run Fixtures -update", path)
		}
	}
}
