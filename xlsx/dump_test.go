package xlsx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
)

// TestDump writes the trees of workbooks to $LOFFICE_DUMP, as the hub
// serves them, for the Dart package to draw them: $LOFFICE_DUMP_FILES lists
// them, relative to corpus/files.
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
		_, tree, err := Open(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		NewCalc(tree, formula.Options{})
		nodes, _ := json.Marshal(tree.Edit())
		base := filepath.Join(dir, strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
		if err := os.WriteFile(base+".json", nodes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
