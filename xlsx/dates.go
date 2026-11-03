package xlsx

import (
	"math"
	"time"

	"github.com/citadellefr/loffice/formula"
)

// Cells of type d hold ISO 8601 dates, which Strict workbooks write for
// the cells formatted as dates.

var (
	epoch1900 = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	epoch1904 = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
)

// isoSerial is the serial number of an ISO 8601 date, time or both. The
// offset from UTC, if any, is dropped: workbooks keep the time as shown.
func isoSerial(s string, date1904 bool) (float64, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02", "15:04:05Z07:00", "15:04:05"} {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		if t.Year() == 0 {
			return float64(t.Hour()*3600+t.Minute()*60+t.Second())/86400 + float64(t.Nanosecond())/86400e9, true
		}
		return formula.Serial(t, date1904), true
	}
	return 0, false
}

// isoDate is a serial number as an ISO 8601 date, when it reads back
// exactly. Times alone and the 29th of February 1900 have none.
func isoDate(serial float64, date1904 bool) (string, bool) {
	n, epoch := serial, epoch1904
	if !date1904 {
		switch {
		case n < 1 || n >= 60 && n < 61:
			return "", false
		case n < 60:
			n++
		}
		epoch = epoch1900
	}
	if n < 0 || n >= 2958466 {
		return "", false
	}
	t := epoch.Add(time.Duration(math.Round(n*86400e3)) * time.Millisecond)
	layout := "2006-01-02T15:04:05.999"
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		layout = "2006-01-02"
	}
	s := t.Format(layout)
	if back, ok := isoSerial(s, date1904); !ok || back != serial {
		return "", false
	}
	return s, true
}
