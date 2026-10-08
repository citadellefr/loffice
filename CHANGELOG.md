# Changelog

## 0.8.7 — 2026-10-09

- Dart: `commands` on the Word, PowerPoint and plain text editors are the
  keywords of the host an `@` typed in the text starts. A `Command` with a
  `search` proposes what to mention for what is typed after it; what is
  picked is written as a link (by its name in plain text). One with an
  `answer` takes the rest of the paragraph as a question and writes its
  answer in its place: the editor is held meanwhile, Escape gives up.
- Dart: links open — `onOpenLink`, on a click with Ctrl or, in a document
  only read, alone. PowerPoint draws them in the color of the theme,
  underlined.
- `pptx` writes the links a client asks for (`link`, `{"url":"…"}`), as `docx`
  does. Both take an address of the web, of mail, or a name ("urn:") that
  only the host that wrote it knows how to open.

## 0.8.6 — 2026-10-08

- Dart: `hosted` on the three editors. A host that draws undo, redo and who
  else is in the document in a bar of its own asks the ribbon to leave them
  out.
- `pptx` keeps the comments part of a slide that never had a comment.

## 0.8.5 — 2026-10-05

- On trame 0.3.0: a workbook follows the edits as trame now asks, told who
  made them (without it the formulas would stop being calculated again). A
  compile-time check keeps `Follower` honest.

## 0.8.4 — 2026-10-05

- `pptx` reads, adds, edits and deletes the comments of PowerPoint
  ("comment" nodes under their slide: author, initials, date, place,
  the comment they answer, their text), through `p:cmLst` and the list of
  authors. The hub signs the comments a client adds with its name, as in
  Word; `Document.Check` takes the author. The modern comments of
  Microsoft 365 are left as they are.
- Dart: Review > New Comment and Show Comments open the comments of the
  slide beside it, with balloons where they sit on the slide. The pane is
  Word's, shared in `chrome/comments_pane.dart`; PowerPoint's comments
  cannot be resolved. The notes are called "Notes" in the ribbon, as
  PowerPoint calls them.

## 0.8.3 — 2026-10-05

- `pptx` reads the background of a table style, "tblBg", which Themed
  Style 2 and its accents fill the table with. `tablestyles.json` is also
  written for the Dart package (`builtin_styles.dart`).
- Dart: Table Design > Styles shows PowerPoint's gallery: the styles of
  the presentation, then Office's light, medium and dark ones, drawn with
  the options of the table. The built-in styles are drawn even when the
  presentation does not hold them.
- Dart: Layout > Split Cells splits a cell that is not merged into the
  columns and rows asked for, sharing its column and row, the cells of the
  other rows and columns merged across them. Layout > Cell Size types the
  height and width of a cell in centimeters, the table growing with them.
- Dart: a group keeps the box of its shapes when one is moved, resized or
  turned, as PowerPoint keeps it, growing and shrinking around them; the
  shapes stay where they are on the slide, the group turned or not.
- `pptx` reads the transition of a slide into "transition" (`Transition`:
  its effect and options, its duration, when the show goes on), and
  writes it back when a client changes it, under the mc:AlternateContent
  of PowerPoint 2010 for a duration no speed gives or an effect it adds.
- Dart: the slide show plays the transitions (fade, through black, push,
  wipe, split, cover, uncover, zoom; the others fade), goes on by itself
  after the time a slide gives, and not on a click when a slide says so;
  nothing is drawn outside the slide. Slides without a transition no
  longer fade. The Transitions tab picks the effect, its options, its
  duration, applies it to all slides, and sets when the show goes on;
  a transition picked plays over the slide, as Preview plays it again.
- Dart: Slide Show > Presenter View (Alt+F5) shows the slide with, beside
  it, the next one and the notes, the time spoken above, which pauses and
  restarts, and the slide's number below.

## 0.8.1 — 2026-10-05

- PowerPoint tables are read, drawn and edited. `pptx` gives a table's
  frame "tbl" (its style and options) and "grid" (the widths of its
  columns), and nodes for its rows ("tr", "h") and cells ("tc"): their
  text as a shape's, their spans and merges, fill, borders, margins and
  anchor. Rows and cells created or deleted by clients are written back,
  a row short of the grid's columns filled with empty cells. The deck
  carries the table styles of the presentation in "tblStyles" and
  "tblStyleDef", and Office's 74 built-in styles those it names without
  holding them (`tablestyles.json`, made from the corpus by
  `go test ./pptx -run BuiltinStyles -update`).
- Dart: tables are drawn as PowerPoint lays them out, rows growing with
  their text, cells taking the parts of their style in Office's order;
  a click in a cell types into it, Tab and Shift+Tab go from cell to
  cell, Tab in the last adds a row, Escape selects the table. Insert >
  Table picks the size of a new table; the Layout tab inserts and deletes
  rows and columns, merged cells growing and shrinking with them, and
  turns the table style options on and off. The shape fill colors a cell,
  the outline its borders.
- Dart: a drag from one cell to another, or Shift and a click, selects a
  block of cells, grown to hold the merged cells it cuts; the formatting
  and fill go to all of them, Delete empties them. Layout > Merge merges
  them into the first, which takes their text, and splits a merged cell
  back. The edges of columns and rows are dragged with the mouse: an
  inner edge shares the width of its two columns, the last widens the
  table, a row goes no lower than its text.
- Dart: the Table Design tab holds the table style options, Shading and
  Borders, and the pen Borders draws with, its weight and color: on the
  edges named of the cells selected, or of the whole table, the cells
  across an outer edge taking the line on their side. Filling or
  outlining a table selected goes to its cells, where the server took
  neither for the table itself.
- `pptx` gives a cell "vert", the direction of its text, and writes it
  back.
- Dart: vertical text ("vert", "vert270", and East Asian and Mongolian
  vertical text as "vert") is drawn turned a quarter, in shapes and cells,
  and edited as drawn: clicks, carets and selections. A cell of vertical
  text is as high as its longest line. Text Direction, in Home >
  Paragraph and in the Layout tab, turns the text of shapes and cells;
  the Layout tab aligns the text of cells left, center and right, and to
  their top, middle and bottom.
- Dart: a click on a group selected, or on another shape of the group
  whose shape is selected, selects the shape of the group under it, which
  then moves, resizes, rotates and nudges in its group, and takes text
  typed; the group is outlined around it. Its order among the shapes of
  the group changes as on a slide.
- Fixed: shapes moved or resized with the mouse went back where they were
  when the button was released; only rotations were kept (since 0.2.0).

## 0.8.0 — 2026-10-04

- The hub, `ot` and the Dart session moved to
  [trame](https://github.com/citadellefr/trame), shared with Bref. The API of
  `loffice` is unchanged: `Peer`, `Options`, `Store` and the errors are
  trame's, `Hub` adds `Media` and `AddPicture` to trame's. `ot` is now
  `github.com/citadellefr/trame/ot`.
- Dart: `ot` and `DocSession` come from `package:trame`, still exported by
  `package:loffice/loffice.dart`. The package depends on trame by git and is
  no longer publishable (`publish_to: none`).

## 0.7.0 — 2026-10-02

- Renamed L'Office: the module is `github.com/citadellefr/loffice`, its
  root package `loffice`, the Dart package `loffice` (`LofficeStrings`),
  and the test variables `LOFFICE_*`. Versions up to 0.6.0 were published
  as `github.com/citadellefr/bref`, a name that now belongs to a markdown
  editor.

## 0.6.0 — 2026-10-01

- Charts are drawn in the three editors, not yet edited. The package
  `chart` reads a chart part into what a client draws it from: plots,
  series with the values Office cached and the cells they come from, axes,
  title, legend, labels, DrawingML fills and lines. `pptx` gives it to a
  frame as "chart", `docx` to a drawing in the "chart" of its "img", and
  `xlsx` to a sheet in "charts", with the anchors, which the server's
  revision moves with rows and columns and whose references follow
  renamed sheets; a chart sheet's "kept" node carries its "chart".
- Dart: `ChartPainter` draws bars, lines, areas, scatters, bubbles, pies,
  doughnuts and radars as Office lays them out, value axes scaled as Excel
  scales them (`Scale.auto`). Slides, Word pages and sheets show their
  charts; Excel reads the series from its cells as they change, hidden
  rows left out, but for pivot charts.

## 0.5.4 — 2026-09-30

- The Dart editors no longer freeze at the first keystroke on the web:
  deltas were composed in an endless loop, `1 << 53` being 0 once
  compiled to JavaScript.

## 0.5.3 — 2026-09-29

- Tracked changes of Word documents: text and paragraph marks inserted or
  deleted read as "ins"/"del" (author) and "insd"/"deld" (date); a
  revision whose keys are removed was accepted or rejected, its element
  dropped. Revisions a client makes are written by `docx` and signed:
  `Check` refuses one by another author than the peer, but for a copy of
  one the document holds. `trackRevisions` of settings.xml is the "track"
  attribute of the document node.
- `WordEditor` tracks what is typed, deleted, pasted and replaced when the
  document does, accepts and rejects changes (one, all, and go to the
  next), steps through them, bars changed lines in the margin and tells
  in its status bar whether changes are tracked; the Review tab and
  Ctrl+Shift+E.

## 0.5.2 — 2026-09-29

- Comments of Word documents: `docx` reads them into "comment" nodes
  (author, initials, date, resolved, the comment answered) with their
  anchors in the flows, and writes those changed, added, answered,
  resolved and deleted. The hub signs a comment added with the name of
  its peer: `hello` tells a client its name (`DocSession.name`).
  `WordEditor` lists them beside the pages, in the order of the text:
  written, answered, edited, resolved and deleted there, their ranges
  highlighted; the Review tab and Ctrl+Alt+M.
- `WordEditor` joins the paragraphs a deletion across flows meets in.
- Fixed: `WordEditor` did not lay the document out again after an edit.

## 0.5.1 — 2026-09-28

- Pictures inserted in Word documents: the hub keeps a picture a client
  sends (`Hub.AddPicture`, PNG, JPEG or GIF of 20 MB at most) and the
  document writes it with its drawing when one shows it; `WordEditor`
  inserts it, picked and sent by the host (`onPicture`), no wider than
  the text.

## 0.5.0 — 2026-09-28

- `docx`: Word documents read into trees and written back, only the parts
  that changed: blocks of the body, headers and footers; paragraphs as
  flows (formatting of paragraphs and characters as keys, the XML read
  riding along, hyperlinks, fields, revisions and content controls around
  runs kept); tables, rows and cells; sections; styles, lists, theme and
  settings for the editor; footnotes and endnotes; pictures and text
  boxes described. 2 285
  documents of the corpus rewritten and 2 257 edited: no error added to
  the Open XML SDK validator's. The hub serves `.docx`, `.docm` and
  `.dotx`.
- `docx`: what a client asks for without XML is written by the server:
  page breaks, page numbers and other fields, links; the styles and lists
  of Word a file lacks (Titre 1, puces, numéros) offered, and written into
  it when a paragraph takes them.
- Dart: `WordEditor`. Pages laid out as Word lays them out: styles and
  their inheritance, lists, tab stops and leaders, tables with their
  styles, merged cells and borders, sections, columns, headers and
  footers with their page numbers, pictures inline and floating, text
  boxes, page breaks, widows and orphans, paragraphs kept together.
  Typing and the keys of Word, formatting of characters and paragraphs,
  the style gallery, bullets and numbers, tables inserted and edited,
  margins, orientation, paper size and columns, headers, footers and page
  numbers, zoom; selections across paragraphs and tables, Ctrl+A; find
  and replace; footnotes at the bottom of their page and endnotes after
  the text; the carets of the others. A keystroke lays out again
  only the paragraphs it changed: 30 ms in 150 pages. The 2 276 documents
  of the corpus laid out without error.
- The relationships named across parts are shared by `pptx` and `docx`
  (`internal/partrel`); the validator compares errors wherever they moved.

## 0.4.0 — 2026-09-28

- `xlsx`: list validations shown in the sheets' "lists", moved and
  renamed with the cells and sheets they read.
- `xlsx`: conditional formats read into the sheets' "cf" and calculated
  by the server into a "looks" node under each sheet: formulas, top and
  bottom values, averages, duplicates, color scales, data bars and icon
  sets, calculated again after each edit on the sheets they read.
- `xlsx`: filters of sheets read into their "filter", which the editor
  sets; filters of other kinds kept as the file has them, moved with
  their columns.
- `formula`: formulas calculated in cells other than the one they were
  written for; `Context.Result`.
- `formula`: SUBTOTAL leaves out the rows a filter hides, and those hidden
  by hand from 101 on, and the subtotals it reads; AGGREGATE and its
  options. A source may tell the rows it hides (`Rows`); a change in
  column 0 calculates again the subtotals reading the row.
- `xlsx`: ranges read the cells typed below those they read last, which
  they left out once the sheet's size was known.
- Dart: lists of values offered in cells, values out of them refused as
  Excel does; conditional formats painted; filters set, applied and
  cleared from arrows on their first row, Ctrl+Shift+L.
- Dart: the editors laid out for phones: a ribbon of one row whose tab is
  chosen from a list, slides in a strip under the slide, pinch to zoom.
- Dart: AutoSum of a filtered list writes SUBTOTAL; the status bar counts
  the cells shown; AGREGAT.

## 0.3.1 — 2026-09-28

- `xlsx`: CSV files read as workbooks of one sheet, fields read as the
  French version of Excel reads them (leading zeros kept), written back
  with their separator, quotes, encoding and line breaks: a file saved
  unedited is the same. The hub serves `.csv`.
- Dart: the Excel editor keeps a CSV file to its one sheet and says that
  only the values shown are saved.
- `xlsx`, `formula`, `ot`: a sheet of 100 000 × 20 opens with 40 % less
  memory and in a third less time; 10 000 formulas are calculated again
  in less than half the time. The targets are measured in CI.
- `internal/charset`: Windows-1252, shared by text and CSV files.

## 0.3.0 — 2026-09-27

- `ot`: grids of cells in nodes: cells set field by field, rows and
  columns inserted and removed; cells checked and kept without decoding
  them.
- `xlsx`: workbooks read into trees of sheets, grids and cell formats, and
  written back, only the sheets that changed; shared strings appended,
  formats derived from those of the file, sheets added, deleted, moved
  and renamed, defined names.
- `xlsx`: what the file keeps as XML (conditional formats, validations,
  links, filters, drawings, tables, comments, pivot tables, charts) moves
  with the rows and columns inserted and removed.
- `formula`: formulas parsed and calculated with the coercions and errors
  of Excel, 223 functions, number formats written in the workbook's
  locale, French by default; an engine that calculates again only what a
  change reaches.
- `bref`: the hub serves workbooks and follows each edit with its own:
  formulas moved and renamed, values calculated again.
- `internal/xmldom`: `Clone`; `Insert` passes over the children a schema
  order does not name.
- `opc`: `MarshalRelationships`.
- `tools/validate`: the calculation chain, which Excel makes again, may be
  dropped; parts under names with backslashes bring no new errors.
- corpus: Calc test documents from LibreOffice.

## 0.2.0 — 2026-09-26

- `ot`: documents are trees of nodes with attributes and text, changed by
  edits; the hub, its protocol and plain text files use them.
- `ot`: no edit deletes the final paragraph mark of a flow.
- `internal/xmldom`: elements that keep their bytes, patched rather than rebuilt.
- `drawingml`: colors, fills, lines, geometries, positions, paragraph, run
  and body properties as JSON; text bodies as flows and back.
- `tools/presets`: the preset shapes of DrawingML, for Go and Dart.
- `pptx`: presentations read into trees and written back, only the parts
  that changed; slides added, copied, moved and deleted; notes.
- `bref`: the hub serves presentations and their pictures (`Hub.Media`).
- `pptx`: pictures in backgrounds and theme fills; trees of test
  presentations in `testdata/pptx` for the Dart package.
- `tools/validate`: edited documents may lose parts, those added must be
  valid.
- corpus: Impress test documents from LibreOffice.

## 0.1.0 — 2026-09-26

- `opc`: read and write OPC packages, keeping untouched parts byte for byte.
- `internal/xmltok`: allocation-free XML tokenizer for document parts.
- `internal/xmlcanon`: canonical comparison of XML parts.
- `opc`: `[Content_Types].xml` keeps the order of its rules when rewritten.
- `tools/validate`: Open XML SDK validation of rewritten packages, in CI.
- `opc`: an entry renamed from backslashes takes the spelling of its override.
- corpus: Writer test documents from LibreOffice.
- `internal/prototype/pagination`: measures how closely Word's page breaks are reproduced.
- `ot`: text flows, deltas, composition and transformation, with vectors for the Dart package.
- `bref`: the hub, which orders, rebases and relays edits, catches up reconnecting clients and saves plain text files.
- `dart`: the Flutter package, with the session and a plain text editor.
