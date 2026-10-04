import 'dart:math' as math;

import 'package:trame/trame.dart';

import 'number_format.dart';
import 'workbook.dart';

/// The filter of a sheet, as the Go package reads it into the sheet's
/// "filter": arrows on the first row of [area], and the values its
/// filtered columns show.
class SheetFilter {
  const SheetFilter(this.area, [this.columns = const {}]);

  static SheetFilter? of(Node sheet) {
    final json = sheet.attributes['filter'];
    if (json is! Map<String, Object?> || json['ref'] is! String) return null;
    final area = parseArea(json['ref']! as String);
    if (area == null) return null;
    final columns = <int, FilterColumn>{};
    final cols = json['cols'];
    if (cols is List<Object?>) {
      for (final c in cols) {
        if (c is! Map<String, Object?> || c['col'] is! int) continue;
        final values = c['vals'] is List<Object?> ? {for (final v in c['vals']! as List<Object?>) '$v'} : <String>{};
        columns[c['col']! as int] = FilterColumn(values: values, blank: c['blank'] == true, kept: c['kept'] is int ? c['kept']! as int : 0);
      }
    }
    return SheetFilter(area, columns);
  }

  final CellArea area;

  /// The filtered columns, by their index in [area] from 0.
  final Map<int, FilterColumn> columns;

  /// The rows the filter shows or hides.
  (int, int) get rows => (area.top + 1, area.bottom);

  bool filters(int col) => columns.containsKey(col - area.left);

  SheetFilter withColumn(int col, FilterColumn? c) {
    final out = {...columns};
    if (c == null) {
      out.remove(col - area.left);
    } else {
      out[col - area.left] = c;
    }
    return SheetFilter(area, out);
  }

  Map<String, Object?> toJson() => {
    'ref': area.name,
    if (columns.isNotEmpty)
      'cols': [
        for (final e in (columns.entries.toList()..sort((a, b) => a.key.compareTo(b.key))))
          {
            'col': e.key,
            if (e.value.kept > 0) 'kept': e.value.kept,
            if (e.value.kept == 0) 'vals': e.value.values.toList(),
            if (e.value.blank) 'blank': true,
          },
      ],
  };
}

/// A filtered column: the values it shows, blank cells when [blank]; or
/// a filter of another kind the file keeps, the [kept]-th, which only the
/// server knows how to read.
class FilterColumn {
  const FilterColumn({this.values = const {}, this.blank = false, this.kept = 0});

  final Set<String> values;
  final bool blank;
  final int kept;

  bool shows(String text) => text.isEmpty ? blank : values.contains(text);
}

/// The text a cell shows, which filters compare.
String shownText(Workbook book, Map<String, Object?>? fields, NumberLocale locale) {
  if (fields == null || fields['v'] == null && fields['e'] == null) return '';
  return cellText(fields, book.style(fields['s'] as String?), locale, date1904: book.date1904).text;
}

/// The values a column of a filter shows, in order, and whether it has
/// blank cells.
(List<String>, bool) columnValues(Workbook book, Node sheet, SheetFilter f, int col, NumberLocale locale) {
  final grid = sheet.grid ?? const Grid.empty();
  final (first, last) = f.rows;
  final numbers = <String, double>{};
  final texts = <String>{};
  var cells = 0;
  for (final c in grid.rows(first, last)) {
    if (c.col != col) continue;
    final t = shownText(book, c.fields, locale);
    if (t.isEmpty) continue;
    cells++;
    texts.add(t);
    if (c.fields['v'] is num) numbers[t] = (c.fields['v']! as num).toDouble();
  }
  final values = texts.toList()
    ..sort((a, b) {
      final x = numbers[a], y = numbers[b];
      if (x != null && y != null) return x.compareTo(y);
      if (x != null || y != null) return x != null ? -1 : 1;
      return a.toLowerCase().compareTo(b.toLowerCase());
    });
  return (values, cells < last - first + 1);
}

/// The block of cells around a cell that are not empty, which a filter
/// set there takes: bounded by empty rows and columns.
CellArea currentRegion(Node sheet, int row, int col) {
  final grid = sheet.grid ?? const Grid.empty();
  bool filled(int r, int c) {
    if (r < 1 || c < 1 || r > maxRows || c > maxCols) return false;
    final f = grid.cell(r, c);
    return f != null && (f['v'] != null || f['f'] != null || f['e'] != null);
  }

  bool any(int r1, int c1, int r2, int c2) {
    for (var r = r1; r <= r2; r++) {
      for (var c = c1; c <= c2; c++) {
        if (filled(r, c)) return true;
      }
    }
    return false;
  }

  var (top, left, bottom, right) = (row, col, row, col);
  for (var grew = true; grew;) {
    grew = false;
    if (top > 1 && any(top - 1, left - 1, top - 1, right + 1)) (top, grew) = (top - 1, true);
    if (bottom < maxRows && any(bottom + 1, left - 1, bottom + 1, right + 1)) (bottom, grew) = (bottom + 1, true);
    if (left > 1 && any(top, left - 1, bottom, left - 1)) (left, grew) = (left - 1, true);
    if (right < maxCols && any(top, right + 1, bottom, right + 1)) (right, grew) = (right + 1, true);
  }
  return CellArea(top, left, math.max(bottom, top + 1), right);
}

/// Sets the filter of a sheet, none when [f] is null, and hides the rows
/// it leaves out, showing the others. The filter takes the rows of data
/// that follow its area. Rows a filter of another kind hid stay hidden.
Edit filterEdit(Workbook book, Node sheet, SheetFilter? f, NumberLocale locale) {
  final grid = sheet.grid ?? const Grid.empty();
  final old = SheetFilter.of(sheet);
  final cells = <Cell>[];
  bool hidden(int r) => grid.cell(r, 0)?['hide'] == true;
  void show(int r, bool visible) {
    if (visible == !hidden(r)) return;
    cells.add(Cell(r, 0, {'hide': visible ? null : true}));
  }

  if (f == null) {
    if (old != null) {
      for (var r = old.rows.$1; r <= old.rows.$2; r++) {
        show(r, true);
      }
    }
    return Edit([Change.set(sheet.id, attributes: {'filter': null}), if (cells.isNotEmpty) Change.cells(sheet.id, cells)]);
  }
  var bottom = f.area.bottom;
  bool filledRow(int r) => [for (var c = f.area.left; c <= f.area.right; c++) grid.cell(r, c)].any((x) => x != null && x['v'] != null);
  while (bottom < maxRows && filledRow(bottom + 1)) {
    bottom++;
  }
  final filter = SheetFilter(CellArea(f.area.top, f.area.left, bottom, f.area.right), f.columns);
  final kept = filter.columns.values.any((c) => c.kept > 0);
  for (var r = filter.rows.$1; r <= filter.rows.$2; r++) {
    var visible = !(kept && hidden(r));
    for (final e in filter.columns.entries) {
      if (!visible) break;
      if (e.value.kept > 0) continue;
      visible = e.value.shows(shownText(book, grid.cell(r, filter.area.left + e.key), locale));
    }
    show(r, visible);
  }
  return Edit([Change.set(sheet.id, attributes: {'filter': filter.toJson()}), if (cells.isNotEmpty) Change.cells(sheet.id, cells)]);
}
