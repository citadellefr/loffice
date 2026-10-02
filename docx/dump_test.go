package docx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/partrel"
)

// TestDump writes the trees and pictures of documents to $LOFFICE_DUMP, for
// the Dart package to draw them: $LOFFICE_DUMP_FILES lists them, relative to
// corpus/files.
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
			t.Logf("%s: %v", name, err)
			continue
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
