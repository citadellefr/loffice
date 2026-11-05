package docx

import (
	"testing"

	"github.com/citadellefr/loffice/opc"
)

func BenchmarkOpen(b *testing.B) {
	data := blankDocx(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Open(data, opc.Limits{}); err != nil {
			b.Fatal(err)
		}
	}
}
