# Changelog

## 0.3.0 — 2026-09-27

- `Grid`: the cells of sheets, as the Go package `ot` has them.
- `SpreadsheetEditor`: Excel in French: the ribbon (Home, Insert,
  Formulas, Data, View), the formula bar with the French names of
  functions, the cells drawn as Excel draws them with their formats,
  borders, merged cells and frozen panes, values typed as French Excel
  reads them, rows, columns and sheets inserted and removed, the fill
  handle, copy and paste, sort, and the status bar's sum, average and
  count.
- Number formats written as the Go engine writes them.
- `DocSelection.cells`: the cells others select.

## 0.2.0 — 2026-09-26

- `Tree` and `Edit`: documents as trees of nodes, as the Go package `ot` has
  them; `DocSession` edits trees, selections name the node they are in.
- Undoing a deletion brings the nodes back under new ids.
- Drawing: the geometry of the 187 preset shapes and custom ones, colors
  resolved against themes as Office does, fills, lines and arrowheads.
- Presentations: slides drawn as PowerPoint does, what they inherit from
  their layout and master included; text laid out paragraph by paragraph
  with bullets, numbering, spacing and autofit, in metric-compatible fonts.
- `PresentationEditor`: PowerPoint in French: the ribbon (Home, Insert,
  Slide Show, View), slides at the left, reordered by dragging, shapes
  selected, moved, resized and rotated, text typed with its formatting,
  bullets and levels, colors of the theme, notes, slide sorter, slide show
  (F5) with fades, the state of saving always in sight.
- `LofficeStrings.refused` takes the server's reason, for an app to word it.

## 0.1.0 — 2026-09-26

- `Delta`: text flows and their edits, the algorithms of the Go package `ot`,
  checked against its vectors.
- `DocSession`: the link to the hub, one edit in flight, offline edits rebased
  on reconnection, undo of one's own edits.
- `PlainTextEditor`: plain text documents, with the selections of others.
