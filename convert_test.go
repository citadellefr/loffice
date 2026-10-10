package loffice

import (
	"errors"
	"os"
	"testing"

	"github.com/citadellefr/loffice/legacy"
)

func TestConvert(t *testing.T) {
	if _, _, _, err := Convert("notes.odt", nil); err == nil {
		t.Fatal("an .odt file was converted")
	}
	if _, _, _, err := Convert("book.XLS", []byte("not a workbook")); !errors.Is(err, legacy.ErrInvalid) {
		t.Fatalf("%v", err)
	}
	data, err := os.ReadFile("corpus/files/poi/SampleSS.xls")
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	out, ext, lost, err := Convert("Sample.xls", data)
	if err != nil || ext != ".xlsx" || len(lost) != 0 {
		t.Fatalf("%q, lost %v, %v", ext, lost, err)
	}
	// what it gives is a workbook the hub serves
	if _, _, err := open("Sample"+ext, out); err != nil {
		t.Fatal(err)
	}
}
