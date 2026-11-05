package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice/agent"
	"github.com/citadellefr/trame"
)

var toolNames = []string{"doc_list", "doc_outline", "doc_read", "doc_search", "text_replace", "block_insert", "block_delete", "text_format",
	"comment_add", "comment_reply", "revision_list", "revision_accept", "revision_reject", "batch", "select", "undo_mine",
	"slide_list", "slide_read", "shape_text_set", "shape_add", "slide_add", "slide_delete", "notes_set",
	"sheet_list", "range_read", "cells_set", "rows_insert", "rows_delete", "columns_insert", "columns_delete"}

// handshake is what a client does on connecting, and gives the tools.
func handshake(t *testing.T, c client) []map[string]any {
	t.Helper()
	r := c.rpc("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1"},
	})
	var init struct {
		ProtocolVersion string
		Capabilities    map[string]any
		ServerInfo      struct{ Name string }
		Instructions    string
	}
	if err := json.Unmarshal(r.Result, &init); err != nil || r.Error != nil {
		t.Fatalf("initialize: %s %v", r.Result, err)
	}
	if init.ProtocolVersion != "2025-06-18" || init.Capabilities["tools"] == nil || init.ServerInfo.Name != "loffice" || init.Instructions == "" {
		t.Fatalf("initialize: %+v", init)
	}
	c.notify("notifications/initialized")
	if r := c.rpc("ping", nil); r.Error != nil || string(r.Result) != "{}" {
		t.Fatalf("ping: %s %+v", r.Result, r.Error)
	}
	var list struct{ Tools []map[string]any }
	if err := json.Unmarshal(c.rpc("tools/list", nil).Result, &list); err != nil {
		t.Fatal(err)
	}
	return list.Tools
}

func checkTools(t *testing.T, tools []map[string]any) {
	t.Helper()
	var names []string
	for _, tool := range tools {
		name := tool["name"].(string)
		names = append(names, name)
		desc, _ := tool["description"].(string)
		schema, _ := tool["inputSchema"].(map[string]any)
		if desc == "" || len(desc) > 600 || schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("%s: description %q schema %v", name, desc, schema)
		}
		props, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		if !slices.Contains(required, any("on_behalf_of")) || props["on_behalf_of"] == nil {
			t.Errorf("%s does not require on_behalf_of", name)
		}
		for _, r := range required {
			if props[r.(string)] == nil {
				t.Errorf("%s requires %s, which it does not describe", name, r)
			}
		}
		for p, def := range props {
			if d, _ := def.(map[string]any)["description"].(string); d == "" {
				t.Errorf("%s.%s has no description", name, p)
			}
		}
	}
	if !slices.Equal(names, toolNames) {
		t.Errorf("tools %v", names)
	}
}

func TestProtocolOverHTTP(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	checkTools(t, handshake(t, c))
	if got := ok(t, c, "doc_outline", as("u1", docKey, nil)); !strings.Contains(got, `"text": "Risques"`) {
		t.Fatal(got)
	}
	if r := c.rpc("resources/list", nil); r.Error == nil || r.Error.Code != codeNoMethod {
		t.Fatalf("%+v", r)
	}
	if r := c.rpc("tools/call", map[string]any{"name": "nothing", "arguments": map[string]any{}}); r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("%+v", r)
	}
	resp, _ := c.post(`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("batch: %d", resp.StatusCode)
	}
	resp, _ = c.post(`{"jsonrpc":"2.0","id":null,"method":"ping"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("null id: %d", resp.StatusCode)
	}
	resp, _ = c.post(`not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("garbage: %d", resp.StatusCode)
	}
	if r := c.rpc("initialize", map[string]any{"protocolVersion": "1999-01-01"}); !strings.Contains(string(r.Result), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("an unknown version gets the server's: %s", r.Result)
	}
}

func TestProtocolOverStdio(t *testing.T) {
	h := newHost(t)
	c := newStdioClient(t, h.srv)
	checkTools(t, handshake(t, c))
	if got := ok(t, c, "doc_read", as("u1", docKey, map[string]any{"from": "p1", "to": "p2"})); !strings.Contains(got, "[p2] Le projet est **en avance**") {
		t.Fatal(got)
	}
	for _, a := range h.accesses() {
		if a.Request != nil {
			t.Fatalf("a stream has no HTTP request: %+v", a)
		}
	}
}

func TestHTTPTransportRules(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	req, _ := http.NewRequest(http.MethodGet, c.url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "POST" {
		t.Fatalf("GET: %v %v", resp, err)
	}
	c.headers.Set("Origin", "https://evil.example")
	if resp, _ := c.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origin: %d", resp.StatusCode)
	}
	if len(h.accesses()) != 0 {
		t.Fatal("a request of another origin reached the host's code")
	}
	c.headers.Del("Origin")
	c.headers.Set("MCP-Protocol-Version", "1999-01-01")
	if resp, _ := c.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("protocol version: %d", resp.StatusCode)
	}
}

// TestEveryTool calls each tool on the sample document, as a model does.
func TestEveryTool(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	u := func(args map[string]any) map[string]any { return as("u1", docKey, args) }

	if got := ok(t, c, "doc_list", map[string]any{"on_behalf_of": "u1"}); !strings.Contains(got, `"doc": "rapport.docx"`) {
		t.Fatal(got)
	}
	read := ok(t, c, "doc_read", u(nil))
	if !strings.HasPrefix(read, "version: ") || !strings.Contains(read, "[p11] Fin du rapport.") || strings.Contains(read, "{++") {
		t.Fatal(read)
	}
	critic := ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup", "from": "p4", "to": "p5"}))
	if !strings.Contains(critic, "{--pourrait --}{++va ++}") || !strings.Contains(critic, "[cm0] Alice") || strings.Contains(critic, "[p3]") || strings.Contains(critic, "[p11]") {
		t.Fatal(critic)
	}
	if got := ok(t, c, "doc_read", u(map[string]any{"from": "p8", "to": "p9"})); !strings.Contains(got, "[t6]") || !strings.Contains(got, "[p10]") {
		t.Fatalf("a range from a cell reads its table: %s", got)
	}
	if got := ok(t, c, "doc_search", u(map[string]any{"query": "budget"})); !strings.Contains(got, `"found": 2`) || !strings.Contains(got, `"block": "p5"`) {
		t.Fatal(got)
	}
	if got := ok(t, c, "doc_search", u(map[string]any{"query": `L[a-z]+`, "regex": true})); !strings.Contains(got, `"text": "Lot"`) {
		t.Fatal(got)
	}

	ok(t, c, "select", u(map[string]any{"block": "p2"}))
	ok(t, c, "text_replace", u(map[string]any{"block": "p2", "find": "en avance", "text": "à l'heure"}))
	ok(t, c, "text_format", u(map[string]any{"block": "p5", "find": "valider", "format": map[string]any{"u": "1", "b": true}}))
	ok(t, c, "comment_add", u(map[string]any{"block": "p4", "find": "retard", "text": "Depuis quand ?"}))
	ok(t, c, "comment_reply", u(map[string]any{"comment": "cm0", "text": "Dix mille euros."}))
	ok(t, c, "block_insert", u(map[string]any{"after": "p3", "text": "Un risque **nouveau**."}))
	ok(t, c, "block_delete", u(map[string]any{"block": "p11"}))

	revs := ok(t, c, "revision_list", u(nil))
	for _, want := range []string{`"author": "Héraut"`, `"onBehalfOf": "u1"`, `"author": "Alice"`} {
		if !strings.Contains(revs, want) {
			t.Fatalf("%s in\n%s", want, revs)
		}
	}
	critic = ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup"}))
	for _, want := range []string{"{--**en avance**--}{++**à l'heure**++}", "{++Un risque **nouveau**.++}", "Héraut: Dix mille euros.", "{--Fin du rapport.--}"} {
		if !strings.Contains(critic, want) {
			t.Fatalf("%s in\n%s", want, critic)
		}
	}

	if got := ok(t, c, "revision_accept", u(map[string]any{"revisions": []string{"p4/1", "p4/2"}})); !strings.Contains(got, "2 edit(s)") {
		t.Fatal(got)
	}
	ok(t, c, "revision_reject", u(map[string]any{"revisions": []string{"p2/1", "p2/2"}}))
	if got := ok(t, c, "undo_mine", as("u1", docKey, nil)); !strings.Contains(got, "Undid") {
		t.Fatal(got)
	}
	if got := ok(t, c, "undo_mine", as("u1", docKey, nil)); !strings.Contains(got, "Nothing to undo") {
		t.Fatal(got)
	}
	if got := ok(t, c, "doc_read", u(nil)); !strings.Contains(got, "[p2] Le projet est **en avance**") || !strings.Contains(got, "[p11] Fin du rapport.") {
		t.Fatalf("undo_mine took back all the AI did:\n%s", got)
	}
}

func TestBatch(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	u := func(args map[string]any) map[string]any { return as("u1", docKey, args) }
	edits := []map[string]any{
		{"op": "replace", "block": "p2", "find": "sous budget", "text": "dans les clous"},
		{"op": "format", "block": "p2", "find": "dans les clous", "format": map[string]any{"i": false}},
		{"op": "insert", "block": "p2", "text": "- un\n- deux"},
		{"op": "comment", "block": "p11", "find": "Fin", "text": "Ok ?"},
	}
	preview := ok(t, c, "batch", u(map[string]any{"edits": edits, "dry_run": true}))
	if !strings.HasPrefix(preview, "Nothing changed. version: ") || !strings.Contains(preview, "{++dans les clous++}") {
		t.Fatal(preview)
	}
	if got := ok(t, c, "doc_read", u(nil)); !strings.Contains(got, "sous budget") {
		t.Fatalf("a dry run changed the document:\n%s", got)
	}
	if got := ok(t, c, "batch", u(map[string]any{"edits": edits})); !strings.Contains(got, "4 edit(s)") {
		t.Fatal(got)
	}
	bad := []map[string]any{
		{"op": "delete", "block": "p3"},
		{"op": "replace", "block": "p5", "find": "absent", "text": "x"},
	}
	msg := fails(t, c, "batch", u(map[string]any{"edits": bad}))
	if !strings.Contains(msg, "edit 2") || !strings.Contains(msg, "doc_read") {
		t.Fatal(msg)
	}
	if got := ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup", "from": "p3", "to": "p3"})); strings.Contains(got, "{--") {
		t.Fatalf("a batch that failed left its first edit:\n%s", got)
	}
	msg = fails(t, c, "batch", u(map[string]any{"edits": bad, "atomic": false}))
	if !strings.Contains(msg, "1 of 2 edits made") {
		t.Fatal(msg)
	}
	if got := ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup", "from": "p3", "to": "p3"})); !strings.Contains(got, "{--") {
		t.Fatalf("a batch that is not atomic keeps what it did:\n%s", got)
	}
}

func TestToolErrorsSayWhatToDo(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"text_replace", as("u1", docKey, map[string]any{"block": "p99", "find": "x", "text": "y"}), "doc_read"},
		{"text_replace", as("u1", docKey, map[string]any{"block": "p2", "find": "absent", "text": "y"}), "doc_read"},
		{"text_replace", as("u1", docKey, map[string]any{"block": "p2", "text": "y", "version": "nope-1"}), "version"},
		{"text_replace", as("u1", docKey, map[string]any{"block": "p2", "text": "y", "bogus": 1}), `unknown argument "bogus"`},
		{"text_replace", as("u1", docKey, map[string]any{"block": 2, "text": "y"}), "argument block must be of type string"},
		{"text_replace", as("u1", docKey, map[string]any{"block": "p2", "text": "a\nb", "find": "Le"}), "insert"},
		{"doc_read", as("u1", docKey, map[string]any{"view": "html"}), "markdown or criticmarkup"},
		{"doc_read", as("u1", docKey, map[string]any{"from": "p99"}), "doc_read"},
		{"doc_read", as("u1", docKey, map[string]any{"from": "p5", "to": "p2"}), "before"},
		{"doc_read", as("u1", "missing.docx", nil), "check its key"},
		{"doc_read", as("u1", "", nil), "doc is required"},
		{"doc_read", as("u1", "a\x00b", nil), "not the key"},
		{"doc_search", as("u1", docKey, map[string]any{"query": "(", "regex": true}), "invalid"},
		{"text_format", as("u1", docKey, map[string]any{"block": "p2", "find": "Le", "format": map[string]any{"bogus": "1"}}), "unknown format"},
		{"revision_accept", as("u1", docKey, map[string]any{"revisions": []string{}}), "empty"},
		{"revision_accept", as("u1", docKey, map[string]any{"revisions": []string{"p9/9"}}), "doc_read"},
		{"batch", as("u1", docKey, map[string]any{"edits": []map[string]any{{"op": "explode"}}}), "invalid"},
		{"doc_outline", map[string]any{"doc": docKey}, "on_behalf_of is required"},
		{"doc_outline", as("  ", docKey, nil), "on_behalf_of is required"},
		{"text_replace", as("u1", docKey, map[string]any{"block": "p2", "text": "y", "nth": -1}), "nth"},
	} {
		if got := fails(t, c, tc.tool, tc.args); !strings.Contains(got, tc.want) {
			t.Errorf("%s %v:\n%s\nwant %q", tc.tool, tc.args, got, tc.want)
		}
	}
	if got := fails(t, c, "doc_outline", nil); !strings.Contains(got, "on_behalf_of is required") {
		t.Error(got)
	}
}

func TestAuthorizeIsAskedOnEveryRequest(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	handshake(t, c)
	ok(t, c, "doc_outline", as("u1", docKey, nil))
	ok(t, c, "text_replace", as("u2", docKey, map[string]any{"block": "p2", "find": "Le", "text": "Un"}))
	ok(t, c, "batch", as("u3", docKey, map[string]any{"dry_run": true, "edits": []map[string]any{{"op": "delete", "block": "p3"}}}))
	ok(t, c, "doc_list", map[string]any{"on_behalf_of": "u4"})
	got := h.accesses()
	want := []Access{
		{Method: "initialize"}, {Method: "notifications/initialized"}, {Method: "ping"}, {Method: "tools/list"},
		{Method: "tools/call", Tool: "doc_outline", User: "u1", Doc: docKey},
		{Method: "tools/call", Tool: "text_replace", User: "u2", Doc: docKey, Write: true},
		{Method: "tools/call", Tool: "batch", User: "u3", Doc: docKey},
		{Method: "tools/call", Tool: "doc_list", User: "u4"},
	}
	if len(got) != len(want) {
		t.Fatalf("asked %d times: %+v", len(got), got)
	}
	for i := range want {
		if got[i].Request == nil || got[i].Request.Header.Get("X-Service") != "heraut" {
			t.Errorf("%d: the caller's request is not given: %+v", i, got[i])
		}
		got[i].Request = nil
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAuthorizeRefuses(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	h.refuse = func(a Access) error {
		switch {
		case a.Request.Header.Get("X-Service") != "heraut":
			return ErrUnauthenticated
		case a.User == "stranger" || a.Method != "tools/call" && c.headers.Get("X-Deny") != "":
			return errors.New("this user may not open this document")
		}
		return nil
	}
	c.headers.Set("X-Service", "other")
	for _, method := range []string{"initialize", "tools/list", "tools/call"} {
		if r := c.rpc(method, map[string]any{"name": "doc_outline", "arguments": as("u1", docKey, nil)}); r.status != http.StatusUnauthorized || r.Error == nil {
			t.Fatalf("%s from another service: %d %s", method, r.status, r.Result)
		}
	}
	if resp, _ := c.post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("notification from another service: %d", resp.StatusCode)
	}
	c.headers.Set("X-Service", "heraut")
	r := c.rpc("tools/call", map[string]any{"name": "doc_read", "arguments": as("stranger", docKey, nil)})
	if res := r.tool(t); !res.IsError || !strings.Contains(res.Content[0].Text, "not allowed: this user may not open this document") {
		t.Fatalf("%+v", res)
	}
	c.headers.Set("X-Deny", "1")
	if r := c.rpc("tools/list", nil); r.status != http.StatusForbidden {
		t.Fatalf("%d", r.status)
	}
	c.headers.Del("X-Deny")
	if got := ok(t, c, "doc_outline", as("u1", docKey, nil)); !strings.Contains(got, "Rapport") {
		t.Fatal(got)
	}
}

func TestOnBehalfOfIsRequired(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	for _, args := range []map[string]any{
		{"doc": docKey, "block": "p2", "text": "x"},
		{"doc": docKey, "block": "p2", "text": "x", "on_behalf_of": ""},
		{"doc": docKey, "block": "p2", "text": "x", "on_behalf_of": 7},
	} {
		if got := fails(t, c, "text_replace", args); !strings.Contains(got, "on_behalf_of") {
			t.Fatal(got)
		}
	}
	if got := ok(t, c, "doc_read", as("u1", docKey, nil)); strings.Contains(got, "{++") {
		t.Fatal(got)
	}
}

// TestRefusedWriteSendsNothing is the consent of the host: what it refuses
// does not reach the room.
func TestRefusedWriteSendsNothing(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	watcher := h.someone(docKey, "Alice")
	_, before, err := watcher.Read(agent.Critic)
	if err != nil {
		t.Fatal(err)
	}
	version := watcher.Version()
	h.refuse = func(a Access) error {
		if a.Write && a.User != "u1" {
			return errors.New("no consent")
		}
		return nil
	}
	denied := as("u2", docKey, map[string]any{"block": "p2", "find": "en avance", "text": "à l'heure"})
	for tool, args := range map[string]map[string]any{
		"text_replace": denied,
		"block_delete": as("u2", docKey, map[string]any{"block": "p2"}),
		"batch":        as("u2", docKey, map[string]any{"edits": []map[string]any{{"op": "delete", "block": "p2"}}}),
		"undo_mine":    as("u2", docKey, nil),
	} {
		if got := fails(t, c, tool, args); !strings.Contains(got, "no consent") {
			t.Fatalf("%s: %s", tool, got)
		}
	}
	if got := ok(t, c, "batch", as("u2", docKey, map[string]any{"dry_run": true, "edits": []map[string]any{{"op": "delete", "block": "p2"}}})); !strings.Contains(got, "{--") {
		t.Fatal("a preview writes nothing, it needs no consent:", got)
	}
	time.Sleep(100 * time.Millisecond)
	if _, after, _ := watcher.Read(agent.Critic); after != before || watcher.Version() != version {
		t.Fatalf("the room received an edit:\n%s", after)
	}
	ok(t, c, "text_replace", as("u1", docKey, map[string]any{"block": "p2", "find": "en avance", "text": "à l'heure"}))
	if watcher.Version() == version && func() bool { time.Sleep(100 * time.Millisecond); return watcher.Version() == version }() {
		t.Fatal("the user who consented could not write")
	}
}

func TestStaleBatchDoesNotOverwrite(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	u := func(args map[string]any) map[string]any { return as("u1", docKey, args) }
	edits := []map[string]any{{"op": "replace", "block": "p2", "find": "en avance", "text": "à l'heure"}}
	preview := ok(t, c, "batch", u(map[string]any{"edits": edits, "dry_run": true}))
	version, _, _ := strings.Cut(strings.TrimPrefix(preview, "Nothing changed. version: "), "\n")

	other := h.someone(docKey, "Alice")
	if _, err := other.Apply(t.Context(), agent.Batch{OnBehalfOf: "a", Direct: true, Edits: []agent.Edit{{Op: "replace", Block: "p2", Find: "sous budget", Text: "dans les clous"}}}); err != nil {
		t.Fatal(err)
	}
	msg := fails(t, c, "batch", u(map[string]any{"edits": edits, "version": version}))
	if !strings.Contains(msg, "changed since") || !strings.Contains(msg, "doc_read") {
		t.Fatal(msg)
	}
	got := ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup", "from": "p2", "to": "p2"}))
	if !strings.Contains(got, "dans les clous") || strings.Contains(got, "à l'heure") {
		t.Fatalf("the stale batch wrote, or the other's edit is lost:\n%s", got)
	}
	fresh, _, _ := strings.Cut(strings.TrimPrefix(got, "version: "), "\n")
	ok(t, c, "batch", u(map[string]any{"edits": edits, "version": fresh}))
	if got := ok(t, c, "doc_read", u(map[string]any{"view": "criticmarkup", "from": "p2", "to": "p2"})); !strings.Contains(got, "{++**à l'heure**++}") {
		t.Fatal(got)
	}
	if msg := fails(t, c, "text_replace", u(map[string]any{"block": "p2", "text": "x", "version": "other-1"})); !strings.Contains(msg, "version") {
		t.Fatal(msg)
	}
}

func TestVersionsHoldBetweenCalls(t *testing.T) {
	h := newHost(t)
	c := newHTTPClient(t, h.srv)
	read := ok(t, c, "doc_read", as("u1", docKey, nil))
	version, _, _ := strings.Cut(strings.TrimPrefix(read, "version: "), "\n")
	ok(t, c, "text_replace", as("u1", docKey, map[string]any{"block": "p2", "find": "Le projet", "text": "Le chantier", "version": version}))
	h.srv.Close()
	if got := fails(t, c, "doc_read", as("u1", docKey, nil)); !strings.Contains(got, "closed") && !strings.Contains(got, "closing") {
		t.Fatal(got)
	}
}

func TestIdleAgentLeavesAndReturnsWithItsJournal(t *testing.T) {
	h := newHost(t)
	h.srv.cfg.Idle = 50 * time.Millisecond
	c := newHTTPClient(t, h.srv)
	ok(t, c, "text_replace", as("u1", docKey, map[string]any{"block": "p2", "find": "en avance", "text": "à l'heure"}))
	for range 100 {
		h.srv.mu.Lock()
		n := len(h.srv.rooms)
		h.srv.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(h.srv.rooms); n != 0 {
		t.Fatal("the AI stays in a room nobody asks about")
	}
	if got := ok(t, c, "revision_list", as("u1", docKey, nil)); !strings.Contains(got, `"onBehalfOf": "u1"`) {
		t.Fatalf("the next visit forgot who the revisions were for:\n%s", got)
	}
}

func TestLongCallStreams(t *testing.T) {
	h := newHost(t)
	h.srv.keepalive = 30 * time.Millisecond
	h.refuse = func(a Access) error {
		if a.Tool == "doc_outline" {
			time.Sleep(200 * time.Millisecond)
		}
		return nil
	}
	c := newHTTPClient(t, h.srv)
	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"_meta":{"progressToken":"tok"},"name":"doc_outline","arguments":{"doc":"rapport.docx","on_behalf_of":"u1"}}}`
	resp, data := c.post(body)
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%s %s", resp.Header.Get("Content-Type"), data)
	}
	if !strings.Contains(string(data), `"progressToken":"tok"`) {
		t.Fatalf("no progress notification:\n%s", data)
	}
	var out rpcResponse
	if err := json.Unmarshal(lastEvent(t, data), &out); err != nil || !strings.Contains(string(out.Result), "Risques") {
		t.Fatalf("%s %v", data, err)
	}
	c.headers.Set("Accept", "application/json")
	resp, data = c.post(body)
	if resp.Header.Get("Content-Type") != "application/json" || !strings.Contains(string(data), "Risques") {
		t.Fatalf("a client that does not take a stream gets the answer: %s %s", resp.Header.Get("Content-Type"), data)
	}
}

func TestSlowHubIsGivenUp(t *testing.T) {
	h := newHost(t)
	h.srv.cfg.Timeout = 100 * time.Millisecond
	h.srv.cfg.Hub = stuckHub{}
	c := newHTTPClient(t, h.srv)
	start := time.Now()
	got := fails(t, c, "doc_read", as("u1", docKey, nil))
	if time.Since(start) > 2*time.Second || !strings.Contains(got, "took too long") && !strings.Contains(got, "cannot be opened") {
		t.Fatalf("%v %s", time.Since(start), got)
	}
}

// stuckHub never lets anyone in.
type stuckHub struct{}

func (stuckHub) Serve(ctx context.Context, _ trame.Conn, _ string, _ trame.Peer) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestNewNeedsWhatItRelyOn(t *testing.T) {
	h := newHost(t)
	for _, cfg := range []Config{
		{Name: "Héraut", Authorize: h.srv.cfg.Authorize},
		{Hub: h.hub, Authorize: h.srv.cfg.Authorize},
		{Hub: h.hub, Name: "Héraut"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%+v", cfg)
		}
	}
}

func TestDocListNeedsHostList(t *testing.T) {
	h := newHost(t)
	cfg := h.srv.cfg
	cfg.List = nil
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range s.tools {
		if tool.name == "doc_list" {
			t.Fatal("doc_list without a list of documents")
		}
	}
}
