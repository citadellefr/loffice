package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/citadellefr/loffice/agent"
)

const (
	maxKey      = 512
	maxEdits    = 100
	maxMatches  = 100
	maxReadSize = 100_000
	maxNth      = 10_000
)

// params are the arguments of every tool: each one takes the ones its
// schema lists.
type params struct {
	Doc        string       `json:"doc"`
	OnBehalfOf string       `json:"on_behalf_of"`
	View       string       `json:"view"`
	From       string       `json:"from"`
	To         string       `json:"to"`
	Query      string       `json:"query"`
	Regex      bool         `json:"regex"`
	Block      string       `json:"block"`
	After      string       `json:"after"`
	Find       string       `json:"find"`
	Nth        int          `json:"nth"`
	Text       string       `json:"text"`
	Format     formatMap    `json:"format"`
	Comment    string       `json:"comment"`
	Revisions  []string     `json:"revisions"`
	Edits      []editParams `json:"edits"`
	Atomic     *bool        `json:"atomic"`
	DryRun     bool         `json:"dry_run"`
	Version    string       `json:"version"`
}

// editParams is an edit of a batch, as agent.Edit.
type editParams struct {
	Op       string    `json:"op"`
	Block    string    `json:"block"`
	Find     string    `json:"find"`
	Nth      int       `json:"nth"`
	Text     string    `json:"text"`
	Format   formatMap `json:"format"`
	Comment  string    `json:"comment"`
	Revision string    `json:"revision"`
}

// formatMap is the formats of a text: "1" sets one, "" clears it. A model
// may write true and false.
type formatMap map[string]string

func (f *formatMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(formatMap, len(raw))
	for k, v := range raw {
		var s string
		var b bool
		switch {
		case json.Unmarshal(v, &s) == nil:
			out[k] = s
		case json.Unmarshal(v, &b) == nil:
			out[k] = map[bool]string{true: "1"}[b]
		case bytes.Equal(v, []byte("null")):
			out[k] = ""
		default:
			return fmt.Errorf("format %q must be a string or true or false", k)
		}
	}
	*f = out
	return nil
}

// call is a tool called, its arguments read.
type call struct {
	*params
	user, doc string
}

type tool struct {
	name        string
	description string
	props       map[string]any
	required    []string
	// write tells whether the call changes the document.
	write func(p *params) bool
	// needsDoc is false for the tools that touch no document.
	noDoc bool
	run   func(ctx context.Context, s *Server, c *call) (string, error)
}

func (t *tool) schema() map[string]any {
	props := map[string]any{"on_behalf_of": prop("string", "Id of the user you work for, as your instructions give it. Required.")}
	if !t.noDoc {
		props["doc"] = prop("string", "Key of the document, as doc_list gives it.")
	}
	for k, v := range t.props {
		props[k] = v
	}
	required := []string{"on_behalf_of"}
	if !t.noDoc {
		required = append(required, "doc")
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             append(required, t.required...),
		"additionalProperties": false,
	}
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "maximum": maxNth, "description": desc}
}

func list(item map[string]any, desc string) map[string]any {
	return map[string]any{"type": "array", "items": item, "maxItems": maxEdits, "description": desc}
}

var (
	findProp   = prop("string", "Text to find in the block, exactly as doc_read shows it, without markup.")
	nthProp    = integer("Which occurrence of the text to take, from 0; the first by default.")
	blockProp  = prop("string", "Id of the block, such as p3, as doc_read shows it.")
	versionOpt = prop("string", "Version you read the blocks on: the call fails, and changes nothing, if a block it touches changed since. The current one by default.")
	formatProp = map[string]any{
		"type":                 "object",
		"description":          `Formats to set: b, i, u, strike, dstrike, caps, smallCaps, hl, color, sz, font, va, shd. "1" or true sets a format, "" or false clears it; color, hl, sz and font take their value.`,
		"additionalProperties": map[string]any{"type": []string{"string", "boolean"}},
	}
	editProp = map[string]any{
		"type":     "object",
		"required": []string{"op"},
		"properties": map[string]any{
			"op":       map[string]any{"type": "string", "enum": []string{"replace", "insert", "delete", "format", "comment", "reply", "accept", "reject"}, "description": "replace: text; insert: Markdown paragraphs after block; delete: block; format; comment: on text; reply: to a comment; accept, reject: a revision."},
			"block":    prop("string", "Id of the block; for insert, the block the new paragraphs go after (none: the start of the document)."),
			"find":     prop("string", "Text of the block the edit applies to; replace without it takes all the text of the block."),
			"nth":      nthProp,
			"text":     prop("string", "New text, Markdown paragraphs for insert, or the comment."),
			"format":   formatProp,
			"comment":  prop("string", "Id of the comment answered, such as cm0, for reply."),
			"revision": prop("string", "Id of the revision, as revision_list gives it, for accept and reject."),
		},
		"additionalProperties": false,
	}
	revisionsProp = list(map[string]any{"type": "string"}, "Ids of the revisions, as revision_list gives them, in any order.")
)

func always(*params) bool { return true }

func newTools() []*tool {
	return []*tool{
		{
			name:        "doc_list",
			description: "List the documents you may open for this user.",
			noDoc:       true,
			run:         runList,
		},
		{
			name:        "doc_outline",
			description: "The headings of a document with their block ids: read it first to find your way in a long one.",
			run:         runOutline,
		},
		{
			name:        "doc_read",
			description: "Read a document as Markdown, each block after its id in brackets, with the version. With view criticmarkup, the revisions and comments show as {++added++}, {--deleted--} and {==commented==}{>>[id] author: comment<<}. Give from and to, ids of blocks, to read a part.",
			props: map[string]any{
				"view": map[string]any{"type": "string", "enum": []string{"markdown", "criticmarkup"}, "description": "markdown, as if every revision were accepted and comments left out (default); criticmarkup, to see the revisions and comments."},
				"from": prop("string", "First block to read, inclusive; the start by default."),
				"to":   prop("string", "Last block to read, inclusive; the end by default."),
			},
			run: runRead,
		},
		{
			name:        "doc_search",
			description: "Find text in the blocks of a document: the block, the place in it and an excerpt of each match.",
			props: map[string]any{
				"query": prop("string", "Text to find, or a regular expression with regex."),
				"regex": prop("boolean", "The query is a regular expression (Go syntax)."),
			},
			required: []string{"query"},
			run:      runSearch,
		},
		{
			name:        "text_replace",
			description: "Replace text in a block, as a tracked change people accept or reject. Without find, all the text of the block is replaced.",
			props:       map[string]any{"block": blockProp, "find": findProp, "nth": nthProp, "text": prop("string", "The new text, on one line."), "version": versionOpt},
			required:    []string{"block", "text"},
			write:       always,
			run:         runReplace,
		},
		{
			name:        "block_insert",
			description: "Insert paragraphs written in Markdown (headings, lists, bold, italic) after a block, as tracked changes.",
			props: map[string]any{
				"after":   prop("string", "Id of the block the new ones go after; the start of the document if omitted."),
				"text":    prop("string", "The paragraphs, in Markdown."),
				"version": versionOpt,
			},
			required: []string{"text"},
			write:    always,
			run:      runInsert,
		},
		{
			name:        "block_delete",
			description: "Delete a paragraph, as a tracked change.",
			props:       map[string]any{"block": blockProp, "version": versionOpt},
			required:    []string{"block"},
			write:       always,
			run:         runDelete,
		},
		{
			name:        "text_format",
			description: "Format text of a block (bold, italic, underline, color…), as a tracked change.",
			props:       map[string]any{"block": blockProp, "find": findProp, "nth": nthProp, "format": formatProp, "version": versionOpt},
			required:    []string{"block", "find", "format"},
			write:       always,
			run:         runFormat,
		},
		{
			name:        "comment_add",
			description: "Comment text of a block, in your name.",
			props:       map[string]any{"block": blockProp, "find": findProp, "nth": nthProp, "text": prop("string", "The comment."), "version": versionOpt},
			required:    []string{"block", "find", "text"},
			write:       always,
			run:         runComment,
		},
		{
			name:        "comment_reply",
			description: "Answer a comment, in your name.",
			props:       map[string]any{"comment": prop("string", "Id of the comment, such as cm0, as doc_read with view criticmarkup shows it."), "text": prop("string", "Your answer."), "version": versionOpt},
			required:    []string{"comment", "text"},
			write:       always,
			run:         runReply,
		},
		{
			name:        "revision_list",
			description: "List the tracked changes of a document: id, kind, author, block and text. Those you made name the user you made them for.",
			run:         runRevisions,
		},
		{
			name:        "revision_accept",
			description: "Accept tracked changes, which then count as the text.",
			props:       map[string]any{"revisions": revisionsProp, "version": versionOpt},
			required:    []string{"revisions"},
			write:       always,
			run:         runReview("accept"),
		},
		{
			name:        "revision_reject",
			description: "Reject tracked changes, which then leave the text as it was.",
			props:       map[string]any{"revisions": revisionsProp, "version": versionOpt},
			required:    []string{"revisions"},
			write:       always,
			run:         runReview("reject"),
		},
		{
			name:        "batch",
			description: "Make several edits together, as tracked changes: each finds its block in the document as the ones before it left it, and all are applied or none. After accepting or rejecting a revision, the next ones of its paragraph move up one: take them from the last to the first. With dry_run, nothing changes and you get the blocks as they would be, in CriticMarkup, and the version to give back to apply them: if the document changed meanwhile the batch fails without writing, so read again and prepare it again.",
			props: map[string]any{
				"edits":   list(editProp, "The edits, in order."),
				"atomic":  prop("boolean", "All or nothing (default true). False applies the edits one by one and stops at the first that fails, the earlier ones staying."),
				"dry_run": prop("boolean", "Only show what the batch would do."),
				"version": versionOpt,
			},
			required: []string{"edits"},
			write:    func(p *params) bool { return !p.DryRun },
			run:      runBatch,
		},
		{
			name:        "select",
			description: "Put your cursor on a block, to show the people in the document what you are looking at.",
			props:       map[string]any{"block": blockProp},
			required:    []string{"block"},
			run:         runSelect,
		},
		{
			name:        "undo_mine",
			description: "Reject every tracked change you made for this user, and only those.",
			write:       always,
			run:         runUndo,
		},
	}
}

// decode reads the arguments of a tool. It always returns the parameters
// it could read, for the host to be asked about.
func (t *tool) decode(raw json.RawMessage) (*params, error) {
	p := &params{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return p, errors.New("no arguments")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return p, errors.New("the arguments must be an object")
	}
	schema := t.schema()["properties"].(map[string]any)
	for k := range keys {
		if _, ok := schema[k]; !ok {
			return lenient(raw, p), fmt.Errorf("unknown argument %q: this tool takes %s", k, strings.Join(slices.Sorted(maps.Keys(schema)), ", "))
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(p); err != nil {
		var typ *json.UnmarshalTypeError
		if errors.As(err, &typ) {
			return lenient(raw, p), fmt.Errorf("argument %s must be of type %s, not %s", typ.Field, typ.Type, typ.Value)
		}
		return lenient(raw, p), fmt.Errorf("invalid arguments: %w", err)
	}
	return p, nil
}

// lenient reads what it can of arguments that did not decode.
func lenient(raw json.RawMessage, p *params) *params {
	_ = json.Unmarshal(raw, p)
	return p
}

// check refuses what a call lacks or overdoes.
func (t *tool) check(p *params) (*call, error) {
	if strings.TrimSpace(p.OnBehalfOf) == "" {
		return nil, errors.New("on_behalf_of is required: give the id of the user you work for, as your instructions name them")
	}
	c := &call{params: p, user: p.OnBehalfOf, doc: p.Doc}
	if !t.noDoc {
		switch {
		case strings.TrimSpace(p.Doc) == "":
			return nil, errors.New("doc is required: give the key of the document, as doc_list gives it")
		case len(p.Doc) > maxKey || !utf8.ValidString(p.Doc) || strings.ContainsFunc(p.Doc, func(r rune) bool { return r < ' ' || r == 0x7f }):
			return nil, errors.New("doc is not the key of a document: use one doc_list gives")
		}
	}
	if len(p.Edits) > maxEdits || len(p.Revisions) > maxEdits {
		return nil, fmt.Errorf("at most %d edits at a time: split the work", maxEdits)
	}
	if p.Nth < 0 || p.Nth > maxNth {
		return nil, errors.New("nth is the number of the occurrence, from 0")
	}
	for _, e := range p.Edits {
		if e.Nth < 0 || e.Nth > maxNth {
			return nil, errors.New("nth is the number of the occurrence, from 0")
		}
	}
	return c, nil
}

// explain tells a model what went wrong and what to do.
func explain(err error) string {
	switch {
	case errors.Is(err, agent.ErrGone):
		return err.Error() + ". Read the document again with doc_read to see the blocks as they are, then redo the edit."
	case errors.Is(err, agent.ErrConflict):
		return err.Error() + ". Nothing was written. Read the blocks again with doc_read, then prepare the edit on the version you get."
	case errors.Is(err, agent.ErrStale):
		return err.Error() + ". Nothing was written. Versions only hold for a while: read the document with doc_read and use the version it gives."
	case errors.Is(err, agent.ErrNotFound):
		return err.Error() + ". Read the block again with doc_read: find is its text exactly as written, without markup, and nth counts from 0."
	case errors.Is(err, agent.ErrUnsupported):
		return "this kind of document cannot be read or edited yet: only Word documents can."
	case errors.Is(err, agent.ErrClosed), errors.Is(err, errClosing):
		return "the document was closed meanwhile. Nothing was lost: call again."
	case errors.Is(err, agent.ErrRefused):
		return err.Error() + ". The document does not take this edit: change it, or read the document again."
	case errors.Is(err, context.DeadlineExceeded):
		return "this took too long and was given up. If it was an edit, read the document again to see whether it was made before trying again."
	case errors.Is(err, context.Canceled):
		return "the call was cancelled."
	}
	return err.Error()
}
