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
- **Small servers.** No dependency outside the Go standard library, and budgets
  measured in benchmarks.

## Packages

| Package | Role |
|---|---|
| [`loffice`](.) | The hub: one room per open document, edits rebased and relayed to everyone connected, saves after a pause. Serves Word documents (`.docx`, `.docm`, `.dotx`), presentations (`.pptx`, `.pptm`, `.ppsx`), workbooks (`.xlsx`, `.xlsm`, `.xltx`) and CSV files (`.csv`), whose formulas it calculates, plain text files (`.txt`), and the pictures of documents. |
| [`dart`](dart) | The Flutter package: the session with the hub, the same `ot` algorithms, and the editors of Word, Excel and PowerPoint documents, Word's with its own page layout. |
| [`ot`](ot) | Edits and how concurrent edits are reconciled. A document is a tree of nodes (slides, shapes, the body of a text file), each with a type, attributes and possibly text; text is a flow of characters and paragraph marks, changed by deltas. The Dart package runs the same algorithms, checked against shared vectors. |
| [`drawingml`](drawingml) | The DrawingML of all three formats: colors, fills, lines, geometries, positions and text bodies, read as JSON and text flows, written back as patches of the XML they came from. |
| [`pptx`](pptx) | Presentations as trees: masters, layouts, slides, shapes and notes. Only what changed is written back; a copied shape keeps its pictures and links. |
| [`xlsx`](xlsx) | Workbooks as trees: sheets, grids of cells and cell formats. Only the sheets that changed are written back; what the file keeps as XML moves with the rows and columns inserted and removed. CSV files are read as workbooks of one sheet and written back as they were read. |
| [`docx`](docx) | Word documents as trees: the body and the headers and footers as blocks, paragraphs as flows whose attributes are their formatting and that of their characters, tables of rows and cells; styles, lists and sections read for the editor. Only the parts that changed are written back, and what the model does not cover rides along in the nodes. |
| [`formula`](formula) | Excel formulas parsed and calculated in the French locale, 223 functions, number formats, and an engine that calculates again only what a change reaches. |
| [`opc`](opc) | The zip container of Office documents: parts, content types, relationships. Untouched parts are copied without being decompressed. Guards against zip bombs, unsafe paths and forged sizes. |
| `internal/xmltok` | An XML tokenizer that allocates nothing per token and keeps the exact bytes of every element, several times faster than `encoding/xml` and checked against it. |
| `internal/prototype/pagination` | A measure, not a feature: how often a page laid out with metric-compatible free fonts ends where Word ended it. |
| `internal/partrel` | Relationships named across the parts of a package, so that what points to a picture or a link can move from a part to another. |
| `internal/xmldom` | A tree of XML elements that keep the bytes they were read from: only what changed is written anew. |
| `internal/xmlcanon` | Whether two XML parts mean the same thing to Office, whatever their prefixes, quoting or layout: how rewritten parts are checked. |

## Protocol

A client connects over a WebSocket the host application has authorized, and
exchanges JSON frames with the hub:

| From | Frame | Meaning |
|---|---|---|
| hub | `hello` | who the client is (`sid`), who else is there, which stay in memory of the document (`epoch`) |
| client | `sync` | the `epoch` and revision `v` of the document it holds, if any |
| hub | `doc` | the whole document at revision `v`, as the edit `d` that creates its nodes, and `ack`, the last edit of this client applied |
| hub | `op`, `ack` … `ready` | or else the edits it missed since `v`, its own acknowledged |
| client | `op` | an edit `d`, numbered `n`, made on revision `v` |
| hub | `op` | someone's edit, rebased, with the revision `v` it made |
| hub | `ack`, `nack` | the client's edit `n` applied as revision `v`, or refused and why |
| both | `eph` | cursors and selections, relayed as they are |
| hub | `join`, `leave`, `saved`, `error` | people coming and going, saves and why one failed |

An edit is a list of changes applied together:

```json
[{"o":"new","id":"s2","t":"slide","k":"V","a":{"hidden":true}},
 {"o":"set","id":"s1","k":"F","a":{"hidden":null}},
 {"o":"txt","id":"title","x":[{"r":5},{"i":"!"}]},
 {"o":"del","id":"s3"}]
```

Nodes are ordered among their siblings by key (`k`), then id. A change to a
node that no longer exists does nothing, and an id is never used again once
its node is deleted. A client keeps one edit in flight and holds the next ones
until it is acknowledged.

## Tests

```sh
corpus/fetch.sh   # test files from Apache POI, LibreOffice, python-docx and python-pptx
go test -race -short ./...        # without the corpus
go test ./...
go test ./ot -run Vectors -update   # after changing the ot algorithms
(cd dart && flutter test)          # replays the same vectors
go test ./opc -run '^$' -fuzz FuzzOpen
go test ./internal/xmltok -run '^$' -fuzz FuzzSameAsEncodingXML
```

The corpus is downloaded from pinned commits and never committed.

The corpus tests write the packages they rewrite to `$LOFFICE_OUT`, which CI then
checks with the Open XML SDK: a rewrite must add no error to those of the
original file.

```sh
LOFFICE_OUT=/tmp/out go test ./pptx -run Corpus
dotnet run --project tools/validate -- corpus/files /tmp/out
LOFFICE_EDITED=/tmp/edited go test ./pptx -run Edits   # a document edited
LOFFICE_EDITED=1 dotnet run --project tools/validate -- corpus/files /tmp/edited
```
