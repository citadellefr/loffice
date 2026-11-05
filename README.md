# L'Office

**Word, Excel and PowerPoint documents for Go servers and Flutter apps.**

L'Office opens, edits and saves `.docx`, `.xlsx` and `.pptx` files natively, with
real-time collaboration and an interface that feels familiar to Microsoft
Office users. It is developed by [Citadelle](https://github.com/citadellefr),
where it replaces a LibreOffice-based editor, and is released under the MIT
license.

> L'Office is in early development. Nothing here is ready for use yet.

## Principles

- **Nothing is lost.** Whatever L'Office does not understand in a file is written
  back exactly as it was read. Opening and saving a document without editing it
  gives back the same document.
- **The server owns the file.** The Go package reads and writes Office Open
  XML; the Flutter package only ever sees a document model.
- **Small servers.** No dependency outside the Go standard library and
  trame, and budgets measured in benchmarks.

## Packages

| Package | Role |
|---|---|
| [`loffice`](.) | The hub, [trame](https://github.com/citadellefr/trame)'s: one room per open document, edits rebased and relayed to everyone connected, saves after a pause. Serves Word documents (`.docx`, `.docm`, `.dotx`, `.dotm`), presentations (`.pptx`, `.pptm`, `.ppsx`, `.potx`, `.potm`), workbooks (`.xlsx`, `.xlsm`, `.xltx`, `.xltm`) and CSV files (`.csv`), whose formulas it calculates, plain text files (`.txt`), and the pictures of documents. |
| [`dart`](dart) | The Flutter package: the editors of Word, Excel and PowerPoint documents, Word's with its own page layout; the free fonts with the metrics of Office's; and the export of documents and presentations as PDF files of vectors and text, written by the package itself. |
| [`drawingml`](drawingml) | The DrawingML of all three formats: colors, fills, lines, geometries, positions and text bodies, read as JSON and text flows, written back as patches of the XML they came from. |
| [`pptx`](pptx) | Presentations as trees: masters, layouts, slides, shapes, tables of rows and cells, and notes; table styles, Office's built-in ones included. Only what changed is written back; a copied shape keeps its pictures and links. |
| [`xlsx`](xlsx) | Workbooks as trees: sheets, grids of cells and cell formats. Only the sheets that changed are written back; what the file keeps as XML moves with the rows and columns inserted and removed. CSV files are read as workbooks of one sheet and written back as they were read. |
| [`agent`](agent) | An AI in a room, as one more participant: it reads a document as Markdown or CriticMarkup, and edits it by batches of revisions signed with its name; it edits presentations and workbooks directly, in view of everyone, and takes back only its own edits. |
| [`mcp`](mcp) | The agent as an MCP server (Model Context Protocol, 2025-06-18) that a host mounts beside its hub as an `http.Handler`, and that `cmd/loffice-mcp` speaks on standard input and output: thirty tools to read, search, edit, comment, review and undo in documents, presentations and workbooks, each call authorized by the host. |
| [`docx`](docx) | Word documents as trees: the body and the headers and footers as blocks, paragraphs as flows whose attributes are their formatting and that of their characters, tables of rows and cells; styles, lists and sections read for the editor. Only the parts that changed are written back, and what the model does not cover rides along in the nodes. |
| [`legacy`](legacy) | The documents of Office 97 to 2003, converted once into those of today: Excel workbooks (`.xls`) with their cells, formulas, formats, sizes, merged cells, frozen panes and names. What a file held that its conversion leaves out is told, never lost in silence. |
| [`formula`](formula) | Excel formulas parsed and calculated in the French locale, 223 functions, number formats, and an engine that calculates again only what a change reaches. |
| [`opc`](opc) | The zip container of Office documents: parts, content types, relationships. Untouched parts are copied without being decompressed. Guards against zip bombs, unsafe paths and forged sizes. |
| `internal/xmltok` | An XML tokenizer that allocates nothing per token and keeps the exact bytes of every element, several times faster than `encoding/xml` and checked against it. |
| `internal/prototype/pagination` | A measure, not a feature: how often a page laid out with metric-compatible free fonts ends where Word ended it. |
| `internal/partrel` | Relationships named across the parts of a package, so that what points to a picture or a link can move from a part to another. |
| `internal/xmldom` | A tree of XML elements that keep the bytes they were read from: only what changed is written anew. |
| `internal/xmlcanon` | Whether two XML parts mean the same thing to Office, whatever their prefixes, quoting or layout: how rewritten parts are checked. |

## AI participants

`agent.Join` puts a program in a document's room through the hub, as anyone
else: its edits pass the same checks, are rebased over what people type
meanwhile, and its cursor shows. It reads a Word document as Markdown, or as
CriticMarkup with its revisions and comments, every block given an id
(`[p3]`, `[t1]`), and finds headings and text. It edits by batches applied
whole or not at all, which `Preview` shows first: replacing text, inserting
paragraphs written in Markdown, deleting, formatting, commenting, accepting
and rejecting revisions. Its changes of text are revisions signed with its
name unless the batch is direct. Each batch names the user the agent acts
for, and `UndoMine` rejects what it did for one of them. A batch prepared on
a version whose blocks changed since fails without writing; the host's
`Allow` can refuse any write before it is sent. The agent keeps up with the
room however long its model thinks.

Presentations and workbooks have no revisions: the agent edits them
directly, in view of everyone. It lists the slides and layouts, reads a slide
with the ids of its shapes, its notes and comments, sets the text of a shape,
adds text boxes, slides of a layout and comments, deletes slides and shapes,
and sets notes; it lists the sheets, reads cells with their values, formulas
and number formats, as Markdown or JSON, sets values and formulas, which the
hub calculates again with what depends on them, and inserts and deletes rows
and columns. The same batches, previews and versions hold. The room sees the
agent's selection move to what it edits, and the list of its recent edits,
for editors to highlight a while. `UndoMine` takes back its edits for a user
by edits that undo them, rebased over what others did since, the last first,
and refuses, saying what, when someone else changed the same content
meanwhile. Formulas, validations, conditional formats, filters, tables,
print breaks and names that deleting rows or columns broke are set back with
them. The notes of a slide, the comments of others and the charts of a
sheet, which nobody may make again, come back in `Record.Lost` instead.

## MCP server

Package `mcp` lets an AI model that speaks MCP read and edit the documents
of a hub: the protocol by hand (JSON-RPC 2.0, `initialize`, `tools/list`,
`tools/call`, notifications), no dependency. A host mounts the server beside
its hub and a consumer such as Héraut calls it over the network; the same
server speaks on standard input and output for a client started locally.

### Mounting it

```go
hub := loffice.NewHub(store, loffice.Options{}, loffice.Limits{})
srv, err := mcp.New(mcp.Config{
	Hub:  hub,
	Name: "Héraut", // the author of its revisions and comments in Word
	Authorize: func(ctx context.Context, a mcp.Access) error {
		service, err := services.Authenticate(a.Request) // who calls, by its credentials
		if err != nil {
			return mcp.ErrUnauthenticated
		}
		return rights.Check(ctx, service, a.User, a.Doc, a.Write)
	},
	List: func(ctx context.Context, a mcp.Access) ([]mcp.Document, error) {
		return documentsOf(ctx, a.User)
	},
})
if err != nil {
	return err
}
defer srv.Close()
mux.Handle("/mcp", srv)
```

A host that does not mount the server exposes nothing, and nothing in the
package is global: each server holds its own rooms.

### What the host decides

`Config.Authorize` is called **on every request**, before anything is read or
written. The AI has no right of its own: the host checks that the caller is a
service it authorized, taken from `Access.Request` (nil on standard input),
and that the user in `Access.User`, the `on_behalf_of` of the call, may read
the document in `Access.Doc` (`Access.Write` false) or edit it
(`Access.Write` true), and has agreed to have the AI do it for them. It
also sees `Method` (`tools/call`…) and `Tool`.

| Authorize returns | A tool call | Any other request |
|---|---|---|
| `nil` | goes on | goes on |
| `mcp.ErrUnauthenticated` | HTTP 401 | HTTP 401 |
| another error | a result with `isError`, `not allowed: <the message>`, which the model reads: write messages that tell it what to do | HTTP 403, JSON-RPC error `-32001` |

A write the host refuses is refused before it is sent to the room: the
document is not touched. A call without `on_behalf_of` reaches `Authorize`
with an empty `User`, for the host to check the caller alone, and is refused
after. `doc_list` exists only when `Config.List` is set; the host gives the
documents the user may open, and `Authorize` is not asked about each.

The author in Word is always `Config.Name`, which stays the same; the user a
batch was made for is its metadata, with which `undo_mine` finds what to take
back. The AI stays in the room of a document between calls, and leaves
after `Config.Idle` (ten minutes) without a call: the versions it gave are
void then, and a call that holds one fails cleanly.

### Calls

Every tool takes `on_behalf_of` and `doc` (the key the hub serves the
document under); `tools/list` gives the JSON schema of each. The tools are
`doc_list`, `doc_outline`, `doc_read` (Markdown or CriticMarkup, a range of
block ids), `doc_search`, `text_replace`, `block_insert`, `block_delete`,
`text_format`, `comment_add`, `comment_reply`, `revision_list`,
`revision_accept`, `revision_reject`, `batch`, `select` and `undo_mine` for
Word documents; `slide_list`, `slide_read`, `shape_text_set`, `shape_add`,
`slide_add`, `slide_delete`, `notes_set` and `comment_add` for presentations;
`sheet_list`, `range_read` (Markdown or JSON), `cells_set`, `rows_insert`,
`rows_delete`, `columns_insert` and `columns_delete` for workbooks. `batch`,
`dry_run`, `version` and `undo_mine` work on all three.

Reads give a `version`, an ETag of the document. A write may hold the
`version` it was prepared on: if the blocks it touches changed since, it
fails with nothing written, and the message says to read again. `batch` is
atomic by default, applied whole or not at all; `dry_run` shows the result
as CriticMarkup without changing anything, and `atomic: false` applies the
edits one by one, stopping at the first that fails.

A tool that fails is a result with `isError` whose text says what to do, not
a JSON-RPC error, which is kept for a malformed message (`-32700`, `-32600`),
an unknown method (`-32601`), an unknown tool (`-32602`) or a refusal
(`-32001`). The HTTP handler takes only POST (405 otherwise), refuses a
body over 4 MiB (413) and an `Origin` that is not in
`Config.AllowedOrigins` (403), and a protocol version it does not speak (400).

A call may take tens of seconds, the model or the hub being slow:
`Config.Timeout` (two minutes) bounds it. When it lasts more than fifteen
seconds and the client accepts `text/event-stream`, the answer comes on a
stream with a comment, or a `notifications/progress` if the client gave a
`progressToken`, every fifteen seconds to keep the connection up.

### Standard input and output

```sh
go build -o loffice-mcp ./cmd/loffice-mcp
claude mcp add loffice -- ./loffice-mcp -root ~/documents -name Assistant
```

or, in the configuration of any client that starts MCP servers:

```json
{"mcpServers": {"loffice": {"command": "/path/to/loffice-mcp", "args": ["-root", "/home/me/documents", "-name", "Assistant"]}}}
```

The documents are the Word files under `-root`, named by their path from it
(`doc_list` gives them). A path that leaves the root, by `..`, an absolute
path or a link, is refused and nothing outside is read or written. Any user
is accepted: the process is the user's own. Edits are saved after a pause,
and when the client closes the input.

## Protocol

Edits and how they are reconciled are those of
[trame](https://github.com/citadellefr/trame#protocol), which also holds the
session of the Dart package. A document is a tree of nodes; what each format
makes of its file is described by the package that reads it.

## Limits

`loffice.NewHub(store, options, limits)` takes the `loffice.Limits` of the
files it opens, and `loffice.Convert` those of the files it converts; the
format packages take theirs as an argument of `Open` (`opc.Limits`,
`xlsx.Limits`, `legacy.Limits`). A zero field takes its default, which holds
every file of the test corpus (but its zip bomb): a file of 32 MiB as stored,
4096 parts, 64 MiB for a part and 128 MiB for all of them once decompressed,
64 MiB of cell data in a workbook, 4 Mi cells in a workbook of Office 97 to
2003. A file over a limit is refused with `loffice.ErrTooLarge`.

## Tests

```sh
go run ./tools/corpus   # test files from Apache POI, LibreOffice, python-docx and python-pptx
go test -race -short ./...        # without the corpus
go test ./...
(cd dart && flutter test)
go test ./opc -run '^$' -fuzz FuzzOpen
go test ./legacy -run '^$' -fuzz FuzzWorkbook
go test ./internal/xmltok -run '^$' -fuzz FuzzSameAsEncodingXML
go test ./docx -run '^$' -fuzz FuzzOpen        # a part of a document, opened, rewritten and edited
go test ./pptx -run '^$' -fuzz FuzzOpen        # the same for a presentation
go test ./drawingml -run '^$' -fuzz FuzzReadWrite
go test ./xlsx -run '^$' -fuzz '^FuzzOpen$'    # a workbook: open, save, calculate
go test ./xlsx -run '^$' -fuzz '^FuzzParts$'   # the sheet, styles and strings of a valid workbook
go test ./chart -run '^$' -fuzz FuzzRead
go test ./formula -run '^$' -fuzz FuzzFormula  # parse, calculate, write again
```

The corpus is downloaded from pinned commits and never committed. The inputs that
made a fuzz target fail are kept in `testdata/fuzz` of its package and run with
the tests.

The corpus tests write the packages they rewrite to `$LOFFICE_OUT`, which CI then
checks with the Open XML SDK: a rewrite must add no error to those of the
original file.

```sh
LOFFICE_OUT=/tmp/out go test ./pptx -run Corpus
dotnet run --project tools/validate -- corpus/files /tmp/out
LOFFICE_EDITED=/tmp/edited go test ./pptx -run Edits   # a document edited
LOFFICE_EDITED=1 dotnet run --project tools/validate -- corpus/files /tmp/edited
```
