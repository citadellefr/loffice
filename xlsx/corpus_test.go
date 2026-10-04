package xlsx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func workbooks(t *testing.T) []string {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	var out []string
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".xlsx", ".xlsm", ".xltx":
			out = append(out, f)
		}
	}
	return out
}

func TestCorpusOpen(t *testing.T) {
	var opened, refused, cells int
	var slowest time.Duration
	var slowestName string
	reasons := map[string]int{}
	for _, f := range workbooks(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, tree, err := Open(data)
		took := time.Since(start)
		if took > slowest {
			slowest, slowestName = took, f
		}
		if took > time.Second {
			t.Logf("%s: %v", f, took)
		}
		if err != nil {
			refused++
			reason := err.Error()
			if errors.Is(err, opc.ErrCompound) || errors.Is(err, opc.ErrInvalid) {
				reason = "package"
			}
			reasons[reason]++
			continue
		}
		opened++
		for _, n := range tree.Children("book") {
			if n.Grid != nil {
				cells += n.Grid.Len()
			}
		}
	}
	t.Logf("%d opened, %d cells; slowest %s in %v; %d refused: %v", opened, cells, slowestName, slowest, refused, reasons)
}

func TestCorpusSave(t *testing.T) {
	saved := 0
	for _, f := range workbooks(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		out, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		samePackage(t, f, data, out)
		saved++
	}
	t.Logf("%d workbooks saved unchanged", saved)
}

// Every sheet rewritten from the tree reads back into the same tree. When
// LOFFICE_OUT is set, the packages are written there for the validator.
func TestCorpusRewrite(t *testing.T) {
	out := os.Getenv("LOFFICE_OUT")
	files := 0
	for _, f := range workbooks(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		saved, err := d.save(tree, true)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		files++
		_, again, err := Open(saved)
		if err != nil {
			t.Fatalf("%s: rewritten workbook does not open: %v", f, err)
		}
		if err := sameTree(tree, again); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if out != "" {
			rel, _ := filepath.Rel(filepath.Join("..", "corpus", "files"), f)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, saved, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d workbooks rewritten", files)
}

// sameTree tells how two trees differ, node by node and cell by cell.
func sameTree(a, b *ot.Tree) error {
	ea, eb := a.Edit(), b.Edit()
	if len(ea) != len(eb) {
		return fmt.Errorf("%d nodes, then %d", len(ea), len(eb))
	}
	for i := range ea {
		x, y := ea[i], eb[i]
		cx, cy := x.Cells, y.Cells
		x.Cells, y.Cells = nil, nil
		jx, _ := json.Marshal(x)
		jy, _ := json.Marshal(y)
		if !bytes.Equal(jx, jy) {
			return fmt.Errorf("node %s\n%s\n%s", x.ID, jx, jy)
		}
		if len(cx) != len(cy) {
			return fmt.Errorf("node %s: %d cells, then %d", x.ID, len(cx), len(cy))
		}
		for j := range cx {
			if cx[j].Row != cy[j].Row || cx[j].Col != cy[j].Col || !bytes.Equal(cx[j].Fields, cy[j].Fields) {
				return fmt.Errorf("node %s: cell %v, then %v", x.ID, cx[j], cy[j])
			}
		}
	}
	return nil
}

// samePackage checks that two packages hold the same parts, byte for byte.
func samePackage(t *testing.T, name string, a, b []byte) {
	t.Helper()
	pa, err := opc.Open(a, Limits)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := opc.Open(b, Limits)
	if err != nil {
		t.Fatalf("%s: saved package does not open: %v", name, err)
	}
	if strings.Join(pa.Names(), " ") != strings.Join(pb.Names(), " ") {
		t.Fatalf("%s: parts differ", name)
	}
	for _, n := range pa.Names() {
		x, _ := pa.Read(n)
		y, _ := pb.Read(n)
		if !bytes.Equal(x, y) {
			t.Fatalf("%s: %s changed", name, n)
		}
	}
}
