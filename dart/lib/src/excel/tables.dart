import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import 'filter.dart';
import 'number_format.dart';
import 'workbook.dart';

/// A table of a sheet, as the Go package reads it into the sheet's
/// "tables": its area, the options of its style and its columns. [json]
/// keeps all of it, to write the table back as it came.
class SheetTable {
  SheetTable._(this.json, this.area);

  static SheetTable? _read(Object? json) {
    if (json is! Map<String, Object?> || json['ref'] is! String) return null;
    final area = parseArea(json['ref']! as String);
    return area == null ? null : SheetTable._(json, area);
  }

  static List<SheetTable> of(Node sheet) {
    final all = sheet.attributes['tables'];
    return all is List<Object?> ? [for (final t in all) ?_read(t)] : const [];
  }

  final Map<String, Object?> json;

  /// The cells of the table: its header row, its data and its total row.
  final CellArea area;

  String get name => '${json['name']}';
  bool get header => json['noHeader'] != true;
  bool get totals => json['totals'] == true;
  bool get filter => json['filter'] == true;
  bool get rows => json['rows'] == true;
  bool get cols => json['cols'] == true;
  bool get first => json['first'] == true;
  bool get last => json['last'] == true;
  String? get style => json['style'] as String?;

  /// The first and the last rows of data.
  int get dataTop => area.top + (header ? 1 : 0);
  int get dataBottom => area.bottom - (totals ? 1 : 0);

  /// The table with [changes] made to it, a field set to null left out.
  SheetTable change(Map<String, Object?> changes) {
    final out = {...json};
    for (final e in changes.entries) {
      if (e.value == null) {
        out.remove(e.key);
      } else {
        out[e.key] = e.value;
      }
    }
    return SheetTable._(out, parseArea('${out['ref']}') ?? area);
  }

  SheetTable resized(CellArea to) => change({'ref': to.name});

  /// The table once a row of the sheet is inserted at [at].
  SheetTable withRowInserted(int at) {
    if (area.top >= at) return resized(CellArea(area.top + 1, area.left, area.bottom + 1, area.right));
    return area.bottom >= at ? resized(CellArea(area.top, area.left, area.bottom + 1, area.right)) : this;
  }
}

/// The edit that sets the tables of a sheet to [tables], after the row
/// [insert]ed in the sheet, which the server moves the tables for.
Edit tablesEdit(Node sheet, List<SheetTable> tables, {Change? insert}) => Edit([
  ?insert,
  Change.set(
    sheet.id,
    attributes: {
      'tables': tables.isEmpty ? null : [for (final t in tables) t.json],
    },
  ),
]);

/// The edit that changes [table] into [changed], or removes it for null.
Edit changeTable(Node sheet, SheetTable table, SheetTable? changed) => tablesEdit(sheet, [
  for (final t in SheetTable.of(sheet))
    if (t.name != table.name) t else ?changed,
]);

/// The edit that turns the total row of a table on or off. The row below
/// the table takes it, a row of the sheet inserted first when it holds
/// something in the columns of the table; the server grows the table.
Edit totalsEdit(Node sheet, SheetTable table, bool on) {
  final grid = sheet.grid ?? const Grid.empty();
  final below = table.area.bottom + 1;
  final busy = on && [for (var c = table.area.left; c <= table.area.right; c++) grid.cell(below, c)].any((f) => f != null && (f['v'] != null || f['f'] != null || f['e'] != null));
  return tablesEdit(sheet, [
    for (final t in SheetTable.of(sheet))
      if (t.name == table.name) t.change({'totals': on ? true : null}) else (busy ? t.withRowInserted(below) : t),
  ], insert: busy ? Change.insert(sheet.id, dimRows, below, 1) : null);
}

/// The edit that makes the table a value typed under its last row joins,
/// as Excel does, or null when there is none.
Edit? growTableEdit(Node sheet, int row, int col) {
  final tables = SheetTable.of(sheet);
  for (final t in tables) {
    if (t.totals || row != t.area.bottom + 1 || col < t.area.left || col > t.area.right) continue;
    final grown = CellArea(t.area.top, t.area.left, row, t.area.right);
    if (tables.any((o) => o != t && o.area.intersects(grown))) return null;
    return changeTable(sheet, t, t.resized(grown));
  }
  return null;
}

enum TableProblem { range, overlap, merged }

/// What stops [area] from being a table, if anything.
TableProblem? tableProblem(Workbook book, Node sheet, CellArea? area, {required bool headers}) {
  if (area == null || area.wholeRows || area.wholeCols || area.bottom >= maxRows) return TableProblem.range;
  final taken = _tableArea(area, headers);
  if (SheetTable.of(sheet).any((t) => t.area.intersects(taken))) return TableProblem.overlap;
  if (book.layout(sheet).merges.any((m) => m.intersects(taken))) return TableProblem.merged;
  return null;
}

/// A row of data is added to a table of a header row alone.
CellArea _tableArea(CellArea area, bool headers) => headers && area.rows == 1 ? CellArea(area.top, area.left, area.top + 1, area.right) : area;

/// The edit that makes a table of [area]. Its first row holds the names of
/// the columns when [headers]; else a row is inserted above, with
/// "Colonne1"…
Edit insertTableEdit(Workbook book, Node sheet, CellArea area, {required bool headers, required NumberLocale locale}) {
  final grid = sheet.grid ?? const Grid.empty();
  final tables = SheetTable.of(sheet);
  final taken = _tableArea(area, headers);
  final ref = headers ? taken : CellArea(area.top, area.left, area.bottom + 1, area.right);
  final shown = [for (var c = ref.left; c <= ref.right; c++) headers ? shownText(book, grid.cell(ref.top, c), locale) : ''];
  final used = <String>{};
  final names = [for (var i = 0; i < shown.length; i++) _unique(shown[i].isEmpty ? 'Colonne${i + 1}' : shown[i], used)];
  final insert = headers ? null : Change.insert(sheet.id, dimRows, area.top, 1);
  final table = SheetTable._({
    'id': 0,
    'name': _freeName(book, tables.length),
    'ref': ref.name,
    'filter': true,
    'style': 'TableStyleMedium2',
    'rows': true,
    'columns': [
      for (var i = 0; i < names.length; i++) {'id': i + 1, 'name': names[i]},
    ],
  }, ref);
  final named = [
    for (var i = 0; i < names.length; i++)
      if (names[i] != shown[i]) Cell(ref.top, ref.left + i, {'v': names[i], 'f': null, 'e': null}),
  ];
  final edit = tablesEdit(sheet, [for (final t in tables) insert == null ? t : t.withRowInserted(area.top), table], insert: insert);
  return named.isEmpty ? edit : Edit([...edit.changes.take(insert == null ? 0 : 1), Change.cells(sheet.id, named), ...edit.changes.skip(insert == null ? 0 : 1)]);
}

/// [name] with 2, 3… after it until [used] does not have it, whatever the
/// case.
String _unique(String name, Set<String> used) {
  var out = name;
  for (var n = 2; !used.add(out.toLowerCase()); n++) {
    out = '$name$n';
  }
  return out;
}

/// A name no table or defined name of the workbook has: Tableau1,
/// Tableau2…
String _freeName(Workbook book, int count) {
  final defined = book.tree['book']?.attributes['names'];
  final taken = {
    for (final s in book.sheets)
      for (final t in SheetTable.of(s)) t.name.toLowerCase(),
    for (final n in defined is List<Object?> ? defined : const <Object?>[])
      if (n is Map<String, Object?>) '${n['name']}'.toLowerCase(),
  };
  for (var n = count + 1; ; n++) {
    if (!taken.contains('tableau$n')) return 'Tableau$n';
  }
}

/// What a built-in table style draws, from the colors of the theme.
class TableLook {
  const TableLook({this.header, this.headerText, this.whole, this.band, this.text, required this.line, this.rules = false, this.grid = false});

  /// The fill and the text color of the header row.
  final Color? header, headerText;

  /// The fill of the data, and of its odd rows and columns.
  final Color? whole, band;
  final Color? text;

  /// The color of lines: [rules] above and below the header and under the
  /// last row, a [grid] around every cell; a double line over the total
  /// row.
  final Color line;
  final bool rules, grid;
}

/// The look of a style of Excel's: TableStyleLight1 to 21, Medium1 to 28,
/// Dark1 to 11, or null for the others. In each family the colors go by
/// seven, black then the six accents of the theme.
TableLook? tableLook(String? style, List<Color> theme) {
  final m = RegExp(r'^TableStyle(Light|Medium|Dark)(\d+)$').firstMatch(style ?? '');
  if (m == null || theme.length < 10) return null;
  final n = int.parse(m[2]!);
  final group = (n - 1) ~/ 7, k = (n - 1) % 7;
  final (white, black) = (theme[0], theme[1]);
  final accent = k == 0 ? black : theme[3 + k];
  Color tone(double tint) => tinted(accent, tint);
  return switch ((m[1], group)) {
    ('Light', 0) => TableLook(band: tone(0.8), line: accent, rules: true),
    ('Light', 1) => TableLook(header: accent, headerText: white, band: tone(0.8), line: accent, rules: true),
    ('Light', _) => TableLook(band: tone(0.8), line: accent, grid: true),
    ('Medium', 0) => TableLook(header: accent, headerText: white, band: tone(0.8), line: accent, rules: true),
    ('Medium', 1) => TableLook(header: accent, headerText: white, whole: tone(0.8), band: tone(0.6), line: white, grid: true),
    ('Medium', 2) => TableLook(header: black, headerText: white, whole: tone(0.8), band: tone(0.6), line: white, grid: true),
    ('Medium', _) => TableLook(header: tone(-0.25), headerText: white, whole: tone(0.8), band: tone(0.6), line: white, grid: true),
    ('Dark', 0) => TableLook(header: black, headerText: white, whole: k == 0 ? tone(0.35) : tone(-0.25), band: k == 0 ? tone(0.25) : tone(-0.4), text: white, line: white),
    _ => TableLook(header: tone(-0.5), headerText: white, whole: tone(0.8), band: tone(0.6), line: white, grid: true),
  };
}

/// The tables of a sheet and what their styles give the cells they hold.
class SheetTables {
  SheetTables(Workbook book, Node sheet) : tables = SheetTable.of(sheet) {
    looks = [for (final t in tables) tableLook(t.style, book.themeColors)];
  }

  final List<SheetTable> tables;
  late final List<TableLook?> looks;
  final _styles = <(CellStyle, int, int, bool, bool), CellStyle>{};

  bool get isEmpty => looks.every((l) => l == null);

  /// The cells of the tables whose styles draw, in [area].
  Iterable<(int, int)> cells(CellArea area) sync* {
    for (var i = 0; i < tables.length; i++) {
      final a = tables[i].area;
      if (looks[i] == null || !a.intersects(area)) continue;
      for (var r = a.top; r <= a.bottom; r++) {
        if (r < area.top || r > area.bottom) continue;
        for (var c = a.left; c <= a.right; c++) {
          if (c >= area.left && c <= area.right) yield (r, c);
        }
      }
    }
  }

  /// The format of a cell, with what the style of its table gives it where
  /// the cell does not have its own fill, text color, font weight or lines.
  CellStyle style(CellStyle base, int row, int col) {
    for (var i = 0; i < tables.length; i++) {
      final t = tables[i], look = looks[i];
      if (look == null || !t.area.contains(row, col)) continue;
      final part = t.header && row == t.area.top
          ? _header
          : t.totals && row == t.area.bottom
          ? _total
          : row == t.area.bottom
          ? _lastRow
          : _body;
      final c = col - t.area.left;
      final band = part != _header && part != _total && (t.rows && (row - t.dataTop).isEven || t.cols && c.isEven);
      final bold = t.first && c == 0 || t.last && c == t.area.cols - 1;
      return _styles[(base, i, part, band, bold)] ??= _apply(base, look, part, band, bold);
    }
    return base;
  }

  static const _header = 0, _body = 1, _total = 2, _lastRow = 3;

  static CellStyle _apply(CellStyle base, TableLook look, int part, bool band, bool bold) {
    final header = part == _header, total = part == _total;
    final f = base.font;
    final font = base.json['font'];
    final color = font is Map<String, Object?> ? font['color'] : null;
    final own = color is Map<String, Object?> && color['auto'] != true && color['theme'] != 1;
    Edge? edge(Edge? own, bool on, [String style = 'thin']) => own ?? (on ? Edge(style, look.line) : null);
    return base.copyWith(
      fill: base.fill ?? (header ? look.header : (band ? look.band : look.whole)),
      font: CellFont(
        name: f.name,
        size: f.size,
        bold: f.bold || header || total || bold,
        italic: f.italic,
        underline: f.underline,
        strike: f.strike,
        color: own ? f.color : (header ? look.headerText : look.text) ?? f.color,
      ),
      top: edge(base.top, look.grid || look.rules && header || total, total ? 'double' : 'thin'),
      bottom: edge(base.bottom, look.grid || look.rules && (header || part == _lastRow || total)),
      left: edge(base.left, look.grid),
      right: edge(base.right, look.grid),
    );
  }
}
