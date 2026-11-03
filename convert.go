package loffice

import (
	"fmt"
	"path"
	"strings"

	"github.com/citadellefr/loffice/legacy"
)

// Legacy are the formats of the Office of before 2007 that Convert takes,
// by extension, with the format each is converted into.
var Legacy = map[string]string{".xls": ".xlsx"}

// Convert makes of a document of the Office of before 2007 one a hub
// serves. name is the file's, whose extension tells its format; ext is
// the extension of the document returned, and lost what the file held
// that it does not. The file itself is left as it is: nothing of it is
// edited. A file larger than limits allow is refused.
func Convert(name string, data []byte, limits Limits) (out []byte, ext string, lost []legacy.Loss, err error) {
	limits = limits.withDefaults()
	if err := limits.check(len(data)); err != nil {
		return nil, "", nil, err
	}
	from := strings.ToLower(path.Ext(name))
	switch from {
	case ".xls":
		out, lost, err = legacy.Workbook(data, limits.legacy())
	default:
		return nil, "", nil, fmt.Errorf("loffice: %q files are not converted", from)
	}
	return out, Legacy[from], lost, err
}
