package legacy

import (
	"unicode/utf16"

	"github.com/citadellefr/loffice/xlsx"
)

// The records of a note: the text box that holds its text, and the note
// itself.
const (
	recTxo = 0x01B6
	// maxAuthor is the most characters Excel allows to the author of a
	// note.
	maxAuthor = 255
)

// noteRecord is a NOTE: the cell it is on, the drawing that holds its text,
// and its author.
type noteRecord struct {
	at     [2]int
	object uint16
	author string
}

// noteText is the text box of a note being read: the characters it still
// misses, which the records that continue it give.
type noteText struct {
	object uint16
	text   []uint16
	left   int
}

// note reads the record of a note, which tells whose it is and what draws
// its text.
func (s *sheet) note(d []byte) {
	if len(d) < 8 {
		s.b.lose(Comments)
		return
	}
	author, _, _ := longText(d[8:])
	s.notes = append(s.notes, noteRecord{[2]int{int(le.Uint16(d)) + 1, int(le.Uint16(d[2:])) + 1}, le.Uint16(d[6:]), author})
}

// textBox starts the text of a drawing, which is that of a note when the
// last drawing was one.
func (s *sheet) textBox(d []byte) {
	if !s.noteDrawn || len(d) < 14 {
		return
	}
	s.noteDrawn = false
	if left := int(le.Uint16(d[10:])); left > 0 {
		s.box = &noteText{object: s.noteObject, left: left}
	}
}

// boxText continues the text of a note: each record holds a run of its
// characters after a flag that tells their width, then the last ones hold
// the fonts.
func (s *sheet) boxText(d []byte) {
	box := s.box
	if box == nil || len(d) < 1 {
		return
	}
	wide, d := d[0]&strWide != 0, d[1:]
	for ; box.left > 0 && len(d) > 0; box.left-- {
		if wide {
			if len(d) < 2 {
				break
			}
			box.text, d = append(box.text, le.Uint16(d)), d[2:]
		} else {
			box.text, d = append(box.text, uint16(d[0])), d[1:]
		}
	}
	if box.left == 0 || len(d) > 0 {
		s.texts[box.object] = string(utf16.Decode(box.text))
		s.box = nil
	}
}

// putNotes gives the cells their notes.
func (s *sheet) putNotes() {
	for _, n := range s.notes {
		text, ok := s.texts[n.object]
		if !ok || n.at[1] > biffCols {
			s.b.lose(Comments)
			continue
		}
		if runes := []rune(n.author); len(runes) > maxAuthor {
			n.author = string(runes[:maxAuthor])
		}
		s.get(n.at).Note = &xlsx.Note{Author: n.author, Text: text}
	}
}
