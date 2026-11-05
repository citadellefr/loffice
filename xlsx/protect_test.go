package xlsx

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
)

func TestProtectRoundTrip(t *testing.T) {
	for _, want := range []Protect{
		{},
		{Pass: "CA7F", Objects: true, Scenarios: true},
		{Allow: []string{"formatCells", "selectLockedCells", "sort"}},
	} {
		root := xmldom.New(mainNS, "worksheet")
		writeProtect(root, mustJSON(want))
		got := readProtect(root)
		if got == nil || !reflect.DeepEqual(*got, want) {
			t.Errorf("%+v read back as %+v", want, got)
		}
	}
}

func TestProtectRemoved(t *testing.T) {
	root := xmldom.New(mainNS, "worksheet")
	writeProtect(root, json.RawMessage(`{"pass":"CA7F"}`))
	writeProtect(root, json.RawMessage(`null`))
	if p := readProtect(root); p != nil {
		t.Errorf("protection left: %+v", p)
	}
}
