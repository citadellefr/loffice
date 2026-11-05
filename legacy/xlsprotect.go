package legacy

import (
	"fmt"

	"github.com/citadellefr/loffice/xlsx"
)

// The records that tell how a sheet is protected: the password, the
// protection of its objects and scenarios, and the permissions Excel 2002
// added.
const (
	recPassword     = 0x0013
	recObjProtect   = 0x0063
	recScenProtect  = 0x00DD
	recFeatHeader   = 0x0867
	featProtection  = 2
	featProtectData = 19
)

// permitted are what a protected sheet leaves to its users, by the bit of
// the permissions that grants it.
var permitted = map[int]string{
	2: "formatCells", 3: "formatColumns", 4: "formatRows", 5: "insertColumns", 6: "insertRows", 7: "insertHyperlinks",
	8: "deleteColumns", 9: "deleteRows", 10: "selectLockedCells", 11: "sort", 12: "autoFilter", 13: "pivotTables",
	14: "selectUnlockedCells",
}

// protection is how a sheet is protected.
type protection struct {
	on, objects, scenarios bool
	password               uint16
	// permissions are what Excel 2002 lets the users of the sheet do, when
	// the sheet tells them; before, they could select cells and nothing more.
	permissions    uint32
	hasPermissions bool
}

func flag(d []byte) bool {
	return len(d) >= 2 && le.Uint16(d) == 1
}

// result is the protection of the sheet, nil when it has none.
func (p *protection) result() *xlsx.Protect {
	if !p.on {
		return nil
	}
	out := &xlsx.Protect{Objects: p.objects, Scenarios: p.scenarios}
	if p.password != 0 {
		out.Pass = fmt.Sprintf("%04X", p.password)
	}
	if !p.hasPermissions {
		p.permissions = 1<<10 | 1<<14
	}
	for bit := 2; bit <= 14; bit++ {
		if p.permissions&(1<<bit) != 0 {
			out.Allow = append(out.Allow, permitted[bit])
		}
	}
	return out
}
