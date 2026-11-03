package agent

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/trame/ot"
)

// mdPara is a paragraph written in Markdown: # headings, - or 1. lists
// indented by two spaces a level, > quotes, **bold** and *italic*.
type mdPara struct {
	heading int
	list    string
	level   int
	quote   bool
	runs    []mdRun
}

type mdRun struct {
	text  string
	attrs ot.Attrs
}

var (
	blockID  = regexp.MustCompile(`^\[[pt]\d+\] `)
	headings = regexp.MustCompile(`^(#{1,9}) +`)
	bullets  = regexp.MustCompile(`^( *)[-*+] +`)
	numbers  = regexp.MustCompile(`^( *)\d+[.)] +`)
)

// parseMarkdown reads each line that is not blank as a paragraph.
func parseMarkdown(text string) []mdPara {
	var out []mdPara
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, " \t\r\n")
		line = blockID.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		var p mdPara
		if m := headings.FindStringSubmatch(line); m != nil {
			p.heading, line = len(m[1]), line[len(m[0]):]
		} else if m := bullets.FindStringSubmatch(line); m != nil {
			p.list, p.level, line = "bullet", len(m[1])/2, line[len(m[0]):]
		} else if m := numbers.FindStringSubmatch(line); m != nil {
			p.list, p.level, line = "decimal", len(m[1])/2, line[len(m[0]):]
		} else if rest, ok := strings.CutPrefix(line, "> "); ok {
			p.quote, line = true, rest
		}
		p.runs = inline(strings.TrimSpace(line))
		out = append(out, p)
	}
	return out
}

// inline reads **bold** and *italic*; a star left open is text.
func inline(s string) []mdRun {
	var out []mdRun
	var text strings.Builder
	bold, italic := false, false
	flush := func() {
		if text.Len() == 0 {
			return
		}
		attrs := ot.Attrs{}
		if bold {
			attrs["b"] = "1"
		}
		if italic {
			attrs["i"] = "1"
		}
		out = append(out, mdRun{strings.ReplaceAll(text.String(), "<br>", "\v"), attrs})
		text.Reset()
	}
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "**") && (bold || strings.Contains(s[i+2:], "**")):
			flush()
			bold = !bold
			i += 2
		case s[i] == '*' && (italic || strings.Contains(s[i+1:], "*")):
			flush()
			italic = !italic
			i++
		default:
			text.WriteByte(s[i])
			i++
		}
	}
	flush()
	return out
}

// mark are the attributes of the paragraph's mark, a heading styled as
// such when the document has the style.
func (p mdPara) mark(styles map[string]*docx.Style) ot.Attrs {
	switch {
	case p.heading > 0:
		if style := "Heading" + strconv.Itoa(p.heading); styles[style] != nil {
			return ot.Attrs{"pstyle": style}
		}
		return ot.Attrs{"outline": strconv.Itoa(p.heading - 1)}
	case p.list != "":
		return ot.Attrs{"num": p.list, "lvl": strconv.Itoa(min(p.level, 8))}
	case p.quote && styles["Quote"] != nil:
		return ot.Attrs{"pstyle": "Quote"}
	}
	return nil
}
