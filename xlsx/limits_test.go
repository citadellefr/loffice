package xlsx

import (
	"errors"
	"os"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

func TestLimits(t *testing.T) {
	data, err := os.ReadFile("../testdata/xlsx/transitional.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(data, Limits{}); err != nil {
		t.Fatal(err)
	}
	for name, limits := range map[string]Limits{
		"parts":     {MaxParts: 1},
		"part size": {MaxPartSize: 10},
		"total":     {MaxTotalSize: 10},
		"cell data": {MaxCellData: 1},
	} {
		if _, _, err := Open(data, limits); !errors.Is(err, opc.ErrTooLarge) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
