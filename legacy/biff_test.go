package legacy

import (
	"encoding/hex"
	"errors"
	"slices"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRK(t *testing.T) {
	for v, want := range map[uint32]float64{5<<2 | 2: 5, 1234<<2 | 3: 12.34, 0xFFFFFFFE: -1, 0x3FF80000: 1.5, 0x40590001: 1} {
		if got := rk(v); got != want {
			t.Errorf("rk(%#x) = %v, want %v", v, got, want)
		}
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		data string
		n    int
		want string
		size int
		rich bool
	}{
		{"00616263", 3, "abc", 4, false},
		{"00e9", 1, "é", 2, false},
		{"01e900ac20", 2, "é€", 5, false},
		// two runs of fonts, then four bytes of phonetics
		{"0c02000400000061620000010001000200aabbccdd", 2, "ab", 21, true},
		{"0161", 1, "", 0, false},
	}
	for _, c := range tests {
		got, size, rich := text(unhex(t, c.data), c.n)
		if got != c.want || size != c.size || rich != c.rich {
			t.Errorf("text(%s) = %q, %d, %v", c.data, got, size, rich)
		}
	}
}

// A string cut by the end of a record goes on in the next, where its
// characters may be wider.
func TestSharedStringsContinued(t *testing.T) {
	got, rich := sharedStrings([][]byte{
		unhex(t, "0200000002000000"+"0300"+"00"+"6162"),
		unhex(t, "01"+"6300"+"0200"+"08"+"0100"+"6869"+"00000100"),
	})
	if !slices.Equal(got, []string{"abc", "hi"}) || !rich {
		t.Fatalf("%q, rich %v", got, rich)
	}
	if got, _ := sharedStrings([][]byte{unhex(t, "0100000001000000"+"0500"+"00"+"6162")}); len(got) != 0 {
		t.Fatalf("a string cut short is read: %q", got)
	}
}

func TestRefused(t *testing.T) {
	for data, want := range map[string]error{
		"":                                 ErrInvalid,
		"504b0304140006000800000021000000": ErrInvalid,
		"0904060000001000":                 ErrTooOld,
		"d0cf11e0a1b11ae10000":             ErrInvalid,
	} {
		if _, _, err := Workbook(unhex(t, data)); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", data, err, want)
		}
	}
}
