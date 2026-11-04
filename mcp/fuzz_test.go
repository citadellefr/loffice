package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

var seeds = []struct{ tool, args string }{
	{"doc_list", `{}`},
	{"doc_outline", `{}`},
	{"doc_read", `{}`},
	{"doc_read", `{"view":"criticmarkup","from":"p2","to":"p5"}`},
	{"doc_read", `{"from":"p9","to":"p2"}`},
	{"doc_read", `{"from":"[p2]","to":"]"}`},
	{"doc_search", `{"query":"projet"}`},
	{"doc_search", `{"query":"(a+)+$","regex":true}`},
	{"doc_search", `{"query":"(","regex":true}`},
	{"doc_search", `{"query":"\u0000�\ud800"}`},
	{"text_replace", `{"block":"p2","find":"en avance","text":"à l'heure"}`},
	{"text_replace", `{"block":"p2","find":"e","nth":10000,"text":"x"}`},
	{"text_replace", `{"block":"p2","find":"e","nth":-3,"text":"x"}`},
	{"text_replace", `{"block":"p2","find":"e","nth":99999999999999999999,"text":"x"}`},
	{"text_replace", `{"block":"t1","find":"","text":"**[p1]** \n\n| a |\n|---|"}`},
	{"block_insert", `{"after":"p2","text":"Un paragraphe.\n\n## Un titre\n\n- un\n- deux\n\n| a | b |\n|---|---|\n| 1 | 2 |"}`},
	{"block_insert", `{"after":"","text":""}`},
	{"block_insert", `{"after":"p99999999999999999999","text":"x"}`},
	{"block_delete", `{"block":"p3"}`},
	{"block_delete", `{"block":"t1"}`},
	{"block_delete", `{"block":"p1/1"}`},
	{"text_format", `{"block":"p2","find":"projet","format":{"b":"1","color":"FF0000","sz":"28"}}`},
	{"text_format", `{"block":"p2","find":"projet","format":{"b":true,"i":false,"hl":null}}`},
	{"text_format", `{"block":"p2","find":"projet","format":{"color":"zz","sz":"-1","font":"\u0000"}}`},
	{"text_format", `{"block":"p2","find":"projet","format":{}}`},
	{"comment_add", `{"block":"p2","find":"projet","text":"Précisez."}`},
	{"comment_add", `{"block":"p2","find":"projet","text":""}`},
	{"comment_reply", `{"comment":"cm1","text":"Fait."}`},
	{"comment_reply", `{"comment":"cm0","text":"Fait."}`},
	{"revision_list", `{}`},
	{"revision_accept", `{"revisions":["p2/1"]}`},
	{"revision_accept", `{"revisions":["p2/1","p2/1","p2/-1","/","p/","p2/x"]}`},
	{"revision_reject", `{"revisions":["p2/1","p2/2"]}`},
	{"revision_reject", `{"revisions":[]}`},
	{"batch", `{"edits":[{"op":"replace","block":"p2","find":"en avance","text":"à l'heure"},{"op":"comment","block":"p3","find":"a","text":"?"}]}`},
	{"batch", `{"dry_run":true,"edits":[{"op":"delete","block":"p3"},{"op":"insert","block":"p2","text":"x"}]}`},
	{"batch", `{"atomic":false,"edits":[{"op":"delete","block":"p3"},{"op":"delete","block":"p3"}]}`},
	{"batch", `{"version":"nope-1","edits":[{"op":"delete","block":"p3"}]}`},
	{"batch", `{"edits":[{"op":"explode"},{"op":"accept","revision":"p2/1"},{"op":"reply","comment":"cm1"},{"op":"format","block":"p2","find":"x","format":{"b":1}}]}`},
	{"batch", `{"edits":[null,1,"x",[],{}]}`},
	{"batch", `{"edits":{"op":"delete"}}`},
	{"batch", `{"edits":[]}`},
	{"select", `{"block":"p2"}`},
	{"select", `{"block":"zz"}`},
	{"undo_mine", `{}`},
	{"undo_mine", `{"extra":1}`},
	{"nothing", `{}`},
	{"", `null`},
	{"doc_read", `[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]`},
	{"doc_read", `{"doc":"../rapport.docx"}`},
	{"doc_read", `{"on_behalf_of":""}`},
	{"doc_read", `{"on_behalf_of":null,"doc":7}`},
}

// FuzzTools calls every tool with hostile arguments against one document:
// a tool that panics, or leaves the server in no state to answer, fails.
func FuzzTools(f *testing.F) {
	h := newHost(f)
	h.srv.cfg.Timeout = 10 * time.Second
	for _, s := range seeds {
		f.Add(s.tool, s.args)
	}
	f.Fuzz(func(t *testing.T, tool, args string) {
		var arguments any = args
		var object map[string]any
		if json.Unmarshal([]byte(args), &object) == nil && object != nil {
			if _, ok := object["on_behalf_of"]; !ok {
				object["on_behalf_of"] = "u1"
			}
			if _, ok := object["doc"]; !ok {
				object["doc"] = docKey
			}
			arguments = object
		}
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
		if err != nil {
			t.Skip()
		}
		answered(t, h, body)
	})
}

// FuzzMessages sends hostile messages, tools/call included.
func FuzzMessages(f *testing.F) {
	h := newHost(f)
	h.srv.cfg.Timeout = 10 * time.Second
	for _, s := range seeds {
		f.Add(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + s.tool + `","arguments":` + s.args + `}}`)
	}
	f.Add(`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":1}}`)
	f.Add(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":[]}`)
	f.Add(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	f.Add(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":{},"arguments":"x"}}`)
	f.Add(`{"jsonrpc":"2.0","id":{},"method":"ping"}`)
	f.Add(`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`)
	f.Add(`{"jsonrpc":"2.0","id":4,"result":{}}`)
	f.Fuzz(func(t *testing.T, body string) { answered(t, h, []byte(body)) })
}

// answered checks that a message got the answer a client may rely on.
func answered(t *testing.T, h *host, body []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, status := h.srv.handle(ctx, nil, body)
	if status < 200 || status > 499 {
		t.Fatalf("status %d for %s", status, body)
	}
	if resp == nil {
		return
	}
	if data, err := json.Marshal(resp); err != nil || len(data) == 0 {
		t.Fatalf("the answer cannot be sent: %v", err)
	}
	if res, ok := resp.Result.(*result); ok && res.IsError && res.Content[0].Text == internalError {
		t.Fatalf("a tool panicked on %s", body)
	}
	if resp.Error == nil && resp.Result == nil {
		t.Fatalf("an answer with neither result nor error: %+v", resp)
	}
}
