package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/agent"
)

// in runs f with the AI in the room of the document.
func (s *Server) in(ctx context.Context, c *call, f func(a *agent.Agent) (string, error)) (string, error) {
	a, release, err := s.acquire(ctx, c.doc)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, errClosing) {
			slog.Warn("mcp: document not opened", "doc", c.doc, "error", err)
			return "", errOpen
		}
		return "", err
	}
	defer release()
	return f(a)
}

// errOpen is a document the hub does not open: it may not exist.
var errOpen = errors.New("the document cannot be opened: check its key with doc_list")

func asJSON(v any) (string, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	return string(out), err
}

func runList(ctx context.Context, s *Server, c *call) (string, error) {
	docs, err := s.cfg.List(ctx, Access{Method: "tools/call", Tool: "doc_list", User: c.user})
	if err != nil {
		return "", err
	}
	if docs == nil {
		docs = []Document{}
	}
	return asJSON(docs)
}

func runOutline(ctx context.Context, s *Server, c *call) (string, error) {
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		version, headings, err := a.Outline()
		if err != nil {
			return "", err
		}
		if headings == nil {
			headings = []agent.Heading{}
		}
		return asJSON(map[string]any{"version": version, "headings": headings})
	})
}

func runRead(ctx context.Context, s *Server, c *call) (string, error) {
	view := agent.Markdown
	switch c.View {
	case "", "markdown":
	case "criticmarkup":
		view = agent.Critic
	default:
		return "", fmt.Errorf("%w: view is markdown or criticmarkup", agent.ErrInvalid)
	}
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		version, text, err := a.Read(view)
		if err != nil {
			return "", err
		}
		text, err = slice(text, c.From, c.To)
		if err != nil {
			return "", err
		}
		return "version: " + version + "\n\n" + text, nil
	})
}

var blockID = regexp.MustCompile(`^\[[^\]\n]+\]`)

// slice is the blocks of a document from the one that holds the id from to
// the first from there that holds the id to, at most maxReadSize bytes of
// them.
func slice(text, from, to string) (string, error) {
	var blocks []string
	for line := range strings.Lines(text) {
		if blockID.MatchString(line) || len(blocks) == 0 {
			blocks = append(blocks, "")
		}
		blocks[len(blocks)-1] += line
	}
	holds := func(block, id string) bool { return strings.Contains(block, "["+id+"]") }
	first, last := 0, len(blocks)
	if from != "" {
		first = -1
		for i, b := range blocks {
			if holds(b, from) {
				first = i
				break
			}
		}
		if first < 0 {
			return "", fmt.Errorf("%w: %s", agent.ErrGone, from)
		}
	}
	if to != "" {
		last = -1
		for i := first; i < len(blocks); i++ {
			if holds(blocks[i], to) {
				last = i + 1
				break
			}
		}
		if last < 0 {
			return "", fmt.Errorf("%w: %s, or it comes before %s", agent.ErrGone, to, from)
		}
	}
	var out strings.Builder
	for i := first; i < last; i++ {
		if out.Len()+len(blocks[i]) > maxReadSize && out.Len() > 0 {
			next := blockID.FindString(blocks[i])
			fmt.Fprintf(&out, "[…] cut here, %d blocks to read: call again with from %s", last-i, strings.Trim(next, "[]"))
			break
		}
		out.WriteString(blocks[i])
	}
	return out.String(), nil
}

func runSearch(ctx context.Context, s *Server, c *call) (string, error) {
	if c.Query == "" {
		return "", fmt.Errorf("%w: query is empty", agent.ErrInvalid)
	}
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		version, matches, err := a.Search(c.Query, c.Regex)
		if err != nil {
			return "", err
		}
		found := len(matches)
		if found > maxMatches {
			matches = matches[:maxMatches]
		}
		if matches == nil {
			matches = []agent.Match{}
		}
		return asJSON(map[string]any{"version": version, "found": found, "matches": matches})
	})
}

func runRevisions(ctx context.Context, s *Server, c *call) (string, error) {
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		version, revs, err := a.Revisions()
		if err != nil {
			return "", err
		}
		if revs == nil {
			revs = []agent.Revision{}
		}
		return asJSON(map[string]any{"version": version, "revisions": revs})
	})
}

func runSelect(ctx context.Context, s *Server, c *call) (string, error) {
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		if err := a.Focus(c.Block); err != nil {
			return "", err
		}
		return "Your cursor is on " + c.Block + ".", nil
	})
}

func runUndo(ctx context.Context, s *Server, c *call) (string, error) {
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		rec, err := a.UndoMine(ctx, c.user)
		switch {
		case err != nil:
			return "", err
		case len(rec.Edits) == 0:
			return "Nothing to undo: no change of yours remains for this user.", nil
		}
		return fmt.Sprintf("Rejected %d of your tracked changes. version: %s", len(rec.Edits), rec.Version), nil
	})
}

// editing runs a tool that makes one edit.
func editing(op func(c *call) agent.Edit) func(context.Context, *Server, *call) (string, error) {
	return func(ctx context.Context, s *Server, c *call) (string, error) {
		return s.in(ctx, c, func(a *agent.Agent) (string, error) {
			return apply(ctx, a, c, []agent.Edit{op(c)})
		})
	}
}

func apply(ctx context.Context, a *agent.Agent, c *call, edits []agent.Edit) (string, error) {
	rec, err := a.Apply(ctx, agent.Batch{OnBehalfOf: c.user, Version: c.Version, Edits: edits})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Done: %d edit(s) made as tracked changes. version: %s. Read the document again for the ids of what you added.", len(edits), rec.Version), nil
}

var (
	runReplace = editing(func(c *call) agent.Edit {
		return agent.Edit{Op: "replace", Block: c.Block, Find: c.Find, Nth: c.Nth, Text: c.Text}
	})
	runInsert = editing(func(c *call) agent.Edit { return agent.Edit{Op: "insert", Block: c.After, Text: c.Text} })
	runDelete = editing(func(c *call) agent.Edit { return agent.Edit{Op: "delete", Block: c.Block} })
	runFormat = editing(func(c *call) agent.Edit {
		return agent.Edit{Op: "format", Block: c.Block, Find: c.Find, Nth: c.Nth, Format: c.Format}
	})
	runComment = editing(func(c *call) agent.Edit {
		return agent.Edit{Op: "comment", Block: c.Block, Find: c.Find, Nth: c.Nth, Text: c.Text}
	})
	runReply = editing(func(c *call) agent.Edit { return agent.Edit{Op: "reply", Comment: c.Comment, Text: c.Text} })
)

func runReview(op string) func(context.Context, *Server, *call) (string, error) {
	return func(ctx context.Context, s *Server, c *call) (string, error) {
		if len(c.Revisions) == 0 {
			return "", fmt.Errorf("%w: revisions is empty", agent.ErrInvalid)
		}
		return s.in(ctx, c, func(a *agent.Agent) (string, error) {
			ids := slices.Clone(c.Revisions)
			slices.SortStableFunc(ids, func(x, y string) int { return cmp.Compare(number(y), number(x)) })
			edits := make([]agent.Edit, len(ids))
			for i, id := range ids {
				edits[i] = agent.Edit{Op: op, Revision: id}
			}
			return apply(ctx, a, c, edits)
		})
	}
}

// number is the rank of a revision in its paragraph, p3/2 being 2: those
// after it move up one when it goes, so the last goes first.
func number(id string) int {
	_, n, _ := strings.Cut(id, "/")
	rank, _ := strconv.Atoi(n)
	return rank
}

func runBatch(ctx context.Context, s *Server, c *call) (string, error) {
	edits := make([]agent.Edit, len(c.Edits))
	for i, e := range c.Edits {
		edits[i] = agent.Edit{Op: e.Op, Block: e.Block, Find: e.Find, Nth: e.Nth, Text: e.Text, Format: e.Format, Comment: e.Comment, Revision: e.Revision}
	}
	return s.in(ctx, c, func(a *agent.Agent) (string, error) {
		b := agent.Batch{OnBehalfOf: c.user, Version: c.Version, Edits: edits}
		if c.DryRun {
			p, err := a.Preview(b)
			if err != nil {
				return "", err
			}
			return "Nothing changed. version: " + p.Version + "\n\n" + p.Markup, nil
		}
		if c.Atomic == nil || *c.Atomic {
			return apply(ctx, a, c, edits)
		}
		for i, e := range edits {
			one := *c.params
			one.Version = ""
			if i == 0 {
				one.Version = c.Version
			}
			if _, err := apply(ctx, a, &call{params: &one, user: c.user, doc: c.doc}, []agent.Edit{e}); err != nil {
				return "", fmt.Errorf("%d of %d edits made, edit %d failed: %w", i, len(edits), i+1, err)
			}
		}
		return fmt.Sprintf("Done: %d edits made one by one, as tracked changes. version: %s", len(edits), a.Version()), nil
	})
}
