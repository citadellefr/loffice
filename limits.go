package loffice

import (
	"fmt"

	"github.com/citadellefr/loffice/legacy"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/xlsx"
)

// Limits bound the files a hub opens and Convert reads, against files
// that would take a small server's memory. Zero fields take the defaults,
// set to hold every file of the test corpus, the largest of which is
// 3 MiB, has 1714 parts and takes 46 MiB once decompressed.
type Limits struct {
	// MaxFileSize is the size of a file as it is stored, 32 MiB by default.
	MaxFileSize int64
	// MaxParts is the number of parts of an Office package, 4096 by
	// default.
	MaxParts int
	// MaxPartSize and MaxTotalSize bound the sizes of the parts of an
	// Office package, and of all of them, once decompressed: 64 MiB and
	// 128 MiB by default.
	MaxPartSize  int64
	MaxTotalSize int64
	// MaxCellData bounds the fields of all the cells of a workbook, in
	// bytes, 64 MiB by default.
	MaxCellData int
	// MaxCells bounds the cells of a workbook of the Office of before 2007,
	// 4 Mi by default.
	MaxCells int
}

func (l Limits) withDefaults() Limits {
	if l.MaxFileSize <= 0 {
		l.MaxFileSize = 32 << 20
	}
	return l
}

func (l Limits) check(size int) error {
	if int64(size) > l.MaxFileSize {
		return fmt.Errorf("%w: the file has %d bytes, the limit is %d", ErrTooLarge, size, l.MaxFileSize)
	}
	return nil
}

func (l Limits) opc() opc.Limits {
	return opc.Limits{MaxParts: l.MaxParts, MaxPartSize: l.MaxPartSize, MaxTotalSize: l.MaxTotalSize}
}

func (l Limits) xlsx() xlsx.Limits {
	return xlsx.Limits{MaxParts: l.MaxParts, MaxPartSize: l.MaxPartSize, MaxTotalSize: l.MaxTotalSize, MaxCellData: l.MaxCellData}
}

func (l Limits) legacy() legacy.Limits {
	return legacy.Limits{MaxCells: l.MaxCells, MaxCellData: l.MaxCellData}
}
