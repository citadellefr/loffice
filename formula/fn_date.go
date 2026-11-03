package formula

import (
	"math"
	"strconv"
	"strings"
	"time"
)

var (
	epoch1900 = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	epoch1904 = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Serial is the number a workbook counts a moment by: days since the
// start of 1900, the 29th of February 1900 that never was included, or
// since 1904.
func Serial(t time.Time, date1904 bool) float64 {
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	if date1904 {
		return wall.Sub(epoch1904).Hours() / 24
	}
	days := wall.Sub(epoch1900).Hours() / 24
	if days < 61 {
		days--
	}
	return days
}

// civil is a day as a calendar writes it.
type civil struct{ y, m, d int }

// dayOf is the day a serial number falls on, the 29th of February 1900
// included.
func dayOf(serial float64, date1904 bool) civil {
	days := math.Floor(serial)
	var t time.Time
	switch {
	case date1904:
		t = epoch1904.AddDate(0, 0, int(days))
	case days == 60:
		return civil{1900, 2, 29}
	case days == 0:
		return civil{1900, 1, 0}
	case days < 60:
		t = epoch1900.AddDate(0, 0, int(days)+1)
	default:
		t = epoch1900.AddDate(0, 0, int(days))
	}
	return civil{t.Year(), int(t.Month()), t.Day()}
}

// timeOfDay is the hours, minutes and seconds of a serial number, rounded
// to the second as Excel shows them.
func timeOfDay(serial float64) (h, m, s int) {
	secs := int(math.Round((serial - math.Floor(serial)) * 86400))
	if secs >= 86400 {
		secs = 86399
	}
	return secs / 3600, secs / 60 % 60, secs % 60
}

func (c *Context) date(y, m, d int) Value {
	if y >= 0 && y < 1900 && !c.Date1904 {
		y += 1900
	}
	if c.Date1904 && y < 1904 && y >= 0 {
		y += 1900
	}
	if y < 0 || y > 9999 {
		return ErrNum
	}
	n := Serial(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), c.Date1904)
	if n < 0 || n > 2958465 {
		return ErrNum
	}
	return Num(n)
}

func (c *Context) serialOf(v Value) (float64, *Value) {
	n, err := c.number(v)
	if err != nil {
		return 0, err
	}
	if n < 0 || n > 2958465.99999 {
		return 0, &ErrNum
	}
	return n, nil
}

var (
	frenchMonths  = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
	englishMonths = []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december"}
)

// monthNamed is the month text names, in full or cut short, in French or
// in English.
func monthNamed(s string) int {
	s = strings.TrimSuffix(strings.ToLower(s), ".")
	if len(s) < 3 {
		return 0
	}
	for i := range 12 {
		for _, names := range [][]string{frenchMonths, englishMonths} {
			if strings.HasPrefix(names[i], s) || s == strings.ReplaceAll(names[i], "é", "e") || s == strings.ReplaceAll(names[i], "û", "u") {
				return i + 1
			}
		}
	}
	if s == "fevr" || s == "fev" {
		return 2
	}
	if s == "aout" {
		return 8
	}
	return 0
}

// parseDate reads a date, a time, or both, as Excel reads them in the
// locale: 25/12/2024, 2024-12-25, 25 déc. 2024, 14:30, 25/12/2024 14:30.
func (c *Context) parseDate(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var day float64
	hasDay := false
	fields := strings.Fields(s)
	rest := fields
	if len(fields) > 0 && !strings.Contains(fields[0], ":") {
		n, used, ok := c.parseDay(fields)
		if !ok {
			return 0, false
		}
		day, hasDay, rest = n, true, fields[used:]
	}
	if len(rest) == 0 {
		return day, hasDay
	}
	t, ok := parseTime(strings.Join(rest, " "))
	if !ok {
		return 0, false
	}
	return day + t, true
}

// parseDay reads a date from the first fields, and tells how many it took.
func (c *Context) parseDay(fields []string) (float64, int, bool) {
	year := c.Now.Year()
	if f := fields[0]; strings.ContainsAny(f, "/-.") {
		parts := strings.FieldsFunc(f, func(r rune) bool { return r == '/' || r == '-' || r == '.' })
		if len(parts) < 2 || len(parts) > 3 {
			return 0, 0, false
		}
		nums := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				if m := monthNamed(p); m > 0 && i == 1 {
					n = m
				} else {
					return 0, 0, false
				}
			}
			nums[i] = n
		}
		var y, m, d int
		switch {
		case len(nums) == 3 && len(parts[0]) == 4:
			y, m, d = nums[0], nums[1], nums[2]
		case len(nums) == 3 && c.Locale.DayFirst:
			d, m, y = nums[0], nums[1], fullYear(nums[2], len(parts[2]))
		case len(nums) == 3:
			m, d, y = nums[0], nums[1], fullYear(nums[2], len(parts[2]))
		case c.Locale.DayFirst:
			d, m, y = nums[0], nums[1], year
		default:
			m, d, y = nums[0], nums[1], year
		}
		if m < 1 || m > 12 || d < 1 || d > daysIn(y, m) || y < 1900 || y > 9999 {
			return 0, 0, false
		}
		v := c.date(y, m, d)
		return v.Num, 1, v.Type == TypeNumber
	}
	// 25 déc. 2024, 25 décembre
	if len(fields) < 2 {
		return 0, 0, false
	}
	d, err := strconv.Atoi(fields[0])
	m := monthNamed(fields[1])
	if err != nil || m == 0 {
		return 0, 0, false
	}
	used, y := 2, year
	if len(fields) > 2 && !strings.Contains(fields[2], ":") {
		n, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, 0, false
		}
		y, used = fullYear(n, len(fields[2])), 3
	}
	if d < 1 || d > daysIn(y, m) {
		return 0, 0, false
	}
	v := c.date(y, m, d)
	return v.Num, used, v.Type == TypeNumber
}

// fullYear is a year written with two digits taken as Excel takes it:
// 00 to 29 in the 2000s, 30 to 99 in the 1900s.
func fullYear(y, digits int) int {
	if digits <= 2 {
		if y < 30 {
			return 2000 + y
		}
		return 1900 + y
	}
	return y
}

func daysIn(y, m int) int {
	return time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// parseTime reads a time of day as a fraction of a day: 14:30, 2:30:15 PM.
func parseTime(s string) (float64, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	pm, am := strings.HasSuffix(s, "PM"), strings.HasSuffix(s, "AM")
	if pm || am {
		s = strings.TrimSpace(s[:len(s)-2])
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var v [3]float64
	for i, p := range parts {
		n, err := strconv.ParseFloat(strings.Replace(p, ",", ".", 1), 64)
		if err != nil || n < 0 || i < 2 && strings.ContainsAny(p, ".,") {
			return 0, false
		}
		v[i] = n
	}
	h := v[0]
	if pm || am {
		if h < 1 || h > 12 {
			return 0, false
		}
		h = math.Mod(h, 12)
		if pm {
			h += 12
		}
	}
	if v[1] >= 60 || v[2] >= 60 {
		return 0, false
	}
	return (h*3600 + v[1]*60 + v[2]) / 86400, true
}

func init() {
	add(map[string]*function{
		"DATE": fn(3, 3, func(c *Context, v []Value) Value {
			var n [3]float64
			for i := range n {
				x, err := c.number(v[i])
				if err != nil {
					return *err
				}
				n[i] = math.Trunc(x)
			}
			return c.date(int(n[0]), int(n[1]), int(n[2]))
		}),
		"TIME": fn(3, 3, func(c *Context, v []Value) Value {
			var n [3]float64
			for i := range n {
				x, err := c.number(v[i])
				if err != nil {
					return *err
				}
				n[i] = math.Trunc(x)
			}
			secs := n[0]*3600 + n[1]*60 + n[2]
			if secs < 0 {
				return ErrNum
			}
			return Num(math.Mod(secs, 86400) / 86400)
		}),
		"TODAY": {max: 0, volatile: true, call: func(c *Context, _ []Expr) Value {
			return Num(math.Floor(Serial(c.Now, c.Date1904)))
		}},
		"NOW": {max: 0, volatile: true, call: func(c *Context, _ []Expr) Value {
			return Num(Serial(c.Now, c.Date1904))
		}},
		"YEAR":  datePart(func(d civil) int { return d.y }),
		"MONTH": datePart(func(d civil) int { return d.m }),
		"DAY":   datePart(func(d civil) int { return d.d }),
		"HOUR": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			h, _, _ := timeOfDay(n)
			return Num(float64(h))
		}),
		"MINUTE": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			_, m, _ := timeOfDay(n)
			return Num(float64(m))
		}),
		"SECOND": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			_, _, s := timeOfDay(n)
			return Num(float64(s))
		}),
		"WEEKDAY": fn(1, 2, func(c *Context, v []Value) Value {
			n, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			kind := 1.0
			if len(v) > 1 && v[1].Type != TypeBlank {
				if kind, err = c.number(v[1]); err != nil {
					return *err
				}
			}
			w, ok := weekday(c.weekday(n), int(kind))
			if !ok {
				return ErrNum
			}
			return Num(float64(w))
		}),
		"WEEKNUM": fn(1, 2, func(c *Context, v []Value) Value {
			n, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			kind := 1.0
			if len(v) > 1 && v[1].Type != TypeBlank {
				if kind, err = c.number(v[1]); err != nil {
					return *err
				}
			}
			if kind == 21 {
				return Num(float64(c.isoWeek(n)))
			}
			starts := map[int]int{1: 0, 17: 0, 2: 1, 11: 1, 12: 2, 13: 3, 14: 4, 15: 5, 16: 6}
			start, ok := starts[int(kind)]
			if !ok {
				return ErrNum
			}
			d := dayOf(n, c.Date1904)
			jan1 := c.date(d.y, 1, 1).Num
			offset := (c.weekday(jan1) - start + 7) % 7
			return Num(math.Floor((math.Floor(n)-jan1+float64(offset))/7) + 1)
		}),
		"ISOWEEKNUM": fn(1, 1, func(c *Context, v []Value) Value {
			n, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			return Num(float64(c.isoWeek(n)))
		}),
		"EDATE": fn(2, 2, func(c *Context, v []Value) Value {
			n, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			k, err := c.number(v[1])
			if err != nil {
				return *err
			}
			d := dayOf(n, c.Date1904)
			m := d.m + int(math.Trunc(k))
			y := d.y + (m-1)/12
			m = (m-1)%12 + 1
			if m < 1 {
				m += 12
				y--
			}
			return c.date(y, m, min(d.d, daysIn(y, m)))
		}),
		"EOMONTH": fn(2, 2, func(c *Context, v []Value) Value {
			n, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			k, err := c.number(v[1])
			if err != nil {
				return *err
			}
			d := dayOf(n, c.Date1904)
			return c.date(d.y, d.m+int(math.Trunc(k))+1, 0)
		}),
		"DAYS": fn(2, 2, func(c *Context, v []Value) Value {
			end, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			start, err := c.serialOf(v[1])
			if err != nil {
				return *err
			}
			return Num(math.Floor(end) - math.Floor(start))
		}),
		"DATEDIF": fn(3, 3, func(c *Context, v []Value) Value {
			a, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			b, err := c.serialOf(v[1])
			if err != nil {
				return *err
			}
			unit, err := c.text(v[2])
			if err != nil {
				return *err
			}
			if a > b {
				return ErrNum
			}
			return datedif(dayOf(a, c.Date1904), dayOf(b, c.Date1904), math.Floor(b)-math.Floor(a), strings.ToUpper(unit))
		}),
		"DAYS360": fn(2, 3, func(c *Context, v []Value) Value {
			a, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			b, err := c.serialOf(v[1])
			if err != nil {
				return *err
			}
			european := false
			if len(v) > 2 {
				if european, err = c.boolean(v[2]); err != nil {
					return *err
				}
			}
			return Num(days360(dayOf(a, c.Date1904), dayOf(b, c.Date1904), european))
		}),
		"DATEVALUE": fn(1, 1, func(c *Context, v []Value) Value {
			s, err := c.text(v[0])
			if err != nil {
				return *err
			}
			n, ok := c.parseDate(s)
			if !ok {
				return ErrValue
			}
			return Num(math.Floor(n))
		}),
		"TIMEVALUE": fn(1, 1, func(c *Context, v []Value) Value {
			s, err := c.text(v[0])
			if err != nil {
				return *err
			}
			n, ok := c.parseDate(s)
			if !ok {
				return ErrValue
			}
			return Num(n - math.Floor(n))
		}),
		"NETWORKDAYS": {min: 2, max: 3, call: func(c *Context, args []Expr) Value {
			a, err := c.serialOf(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			b, err := c.serialOf(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			holidays, e := c.holidays(args[2:])
			if e != nil {
				return *e
			}
			sign := 1.0
			a, b = math.Floor(a), math.Floor(b)
			if a > b {
				a, b, sign = b, a, -1
			}
			n := 0.0
			for d := a; d <= b; d++ {
				if w := c.weekday(d); w != 0 && w != 6 && !holidays[d] {
					n++
				}
			}
			return Num(sign * n)
		}},
		"WORKDAY": {min: 2, max: 3, call: func(c *Context, args []Expr) Value {
			a, err := c.serialOf(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			k, err := c.number(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			holidays, e := c.holidays(args[2:])
			if e != nil {
				return *e
			}
			d, step := math.Floor(a), 1.0
			if k < 0 {
				step = -1
			}
			for left := math.Abs(math.Trunc(k)); left > 0; {
				d += step
				if w := c.weekday(d); w != 0 && w != 6 && !holidays[d] {
					left--
				}
			}
			return Num(d)
		}},
		"YEARFRAC": fn(2, 3, func(c *Context, v []Value) Value {
			a, err := c.serialOf(v[0])
			if err != nil {
				return *err
			}
			b, err := c.serialOf(v[1])
			if err != nil {
				return *err
			}
			basis := 0.0
			if len(v) > 2 && v[2].Type != TypeBlank {
				if basis, err = c.number(v[2]); err != nil {
					return *err
				}
			}
			return c.yearFrac(a, b, int(basis))
		}),
	})
}

// yearFrac is the fraction of a year between two days, counted as basis
// says.
func (c *Context) yearFrac(a, b float64, basis int) Value {
	a, b = math.Floor(a), math.Floor(b)
	if a > b {
		a, b = b, a
	}
	da, db := dayOf(a, c.Date1904), dayOf(b, c.Date1904)
	switch basis {
	case 0:
		return Num(days360(da, db, false) / 360)
	case 1:
		years := float64(db.y - da.y + 1)
		total := c.date(db.y+1, 1, 1).Num - c.date(da.y, 1, 1).Num
		if da.y == db.y || da.y+1 == db.y && (db.m < da.m || db.m == da.m && db.d <= da.d) {
			total, years = 365, 1
			if isLeap(da.y) && (da.m <= 2) || isLeap(db.y) && (db.m > 2 || db.m == 2 && db.d == 29) {
				total = 366
			}
		}
		return Num((b - a) / (total / years))
	case 2:
		return Num((b - a) / 360)
	case 3:
		return Num((b - a) / 365)
	case 4:
		return Num(days360(da, db, true) / 360)
	}
	return ErrNum
}

func isLeap(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

func datePart(f func(civil) int) *function {
	return fn(1, 1, func(c *Context, v []Value) Value {
		return c.mapValues(v[0], func(x Value) Value {
			n, err := c.serialOf(x)
			if err != nil {
				return *err
			}
			return Num(float64(f(dayOf(n, c.Date1904))))
		})
	})
}

// weekday is the day of the week of a serial number, 0 for Sunday.
func (c *Context) weekday(n float64) int {
	d := int(math.Floor(n))
	if c.Date1904 {
		d += 1462
	}
	if !c.Date1904 && d < 60 {
		d--
	}
	return ((d-1)%7 + 7) % 7
}

// weekday is a day of the week counted as WEEKDAY's return type says.
func weekday(sunday0, kind int) (int, bool) {
	switch kind {
	case 1, 17:
		return sunday0 + 1, true
	case 2:
		return (sunday0+6)%7 + 1, true
	case 3:
		return (sunday0 + 6) % 7, true
	case 11, 12, 13, 14, 15, 16:
		start := kind - 10
		return (sunday0-start+7)%7 + 1, true
	}
	return 0, false
}

func (c *Context) isoWeek(n float64) int {
	d := dayOf(n, c.Date1904)
	_, w := time.Date(d.y, time.Month(d.m), d.d, 0, 0, 0, 0, time.UTC).ISOWeek()
	return w
}

func (c *Context) holidays(args []Expr) (map[float64]bool, *Value) {
	out := map[float64]bool{}
	if len(args) == 0 {
		return out, nil
	}
	var err *Value
	c.eachOf(c.Eval(args[0]), func(v Value, _ bool) bool {
		switch v.Type {
		case TypeError:
			found := v
			err = &found
			return false
		case TypeNumber:
			out[math.Floor(v.Num)] = true
		}
		return true
	})
	return out, err
}

func datedif(a, b civil, days float64, unit string) Value {
	months := (b.y-a.y)*12 + b.m - a.m
	if b.d < a.d {
		months--
	}
	switch unit {
	case "D":
		return Num(days)
	case "M":
		return Num(float64(months))
	case "Y":
		return Num(float64(months / 12))
	case "YM":
		return Num(float64(months % 12))
	case "MD":
		d := b.d - a.d
		if d < 0 {
			pm, py := b.m-1, b.y
			if pm < 1 {
				pm, py = 12, py-1
			}
			d += daysIn(py, pm)
		}
		return Num(float64(d))
	case "YD":
		y := b.y
		if b.m < a.m || b.m == a.m && b.d < a.d {
			y--
		}
		start := time.Date(y, time.Month(a.m), a.d, 0, 0, 0, 0, time.UTC)
		end := time.Date(b.y, time.Month(b.m), b.d, 0, 0, 0, 0, time.UTC)
		return Num(math.Round(end.Sub(start).Hours() / 24))
	}
	return ErrNum
}

// days360 counts days as a year of twelve months of 30 days does, the
// American way (NASD) or the European one.
func days360(a, b civil, european bool) float64 {
	da, db := a.d, b.d
	if european {
		da, db = min(da, 30), min(db, 30)
	} else {
		lastFeb := func(d civil) bool { return d.m == 2 && d.d == daysIn(d.y, 2) }
		if lastFeb(a) {
			if lastFeb(b) {
				db = 30
			}
			da = 30
		}
		if db == 31 && da >= 30 {
			db = 30
		}
		if da == 31 {
			da = 30
		}
	}
	return float64((b.y-a.y)*360 + (b.m-a.m)*30 + db - da)
}
