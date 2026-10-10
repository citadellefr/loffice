import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import 'charts.dart';
import 'workbook.dart';

/// The names Excel gives what a sheet prints and what its pages repeat.
const printAreaName = '_xlnm.Print_Area';
const printTitlesName = '_xlnm.Print_Titles';

/// The sizes of paper in points, by the numbers Excel gives them.
const _papers = <int, Size>{
  1: Size(612, 792),
  3: Size(792, 1224),
  5: Size(612, 1008),
  7: Size(522, 756),
  8: Size(841.89, 1190.55),
  9: Size(595.28, 841.89),
  11: Size(419.53, 595.28),
  12: Size(708.66, 1000.63),
  13: Size(498.9, 708.66),
};

const paperLetter = 1, paperLegal = 5, paperA3 = 8, paperA4 = 9, paperA5 = 11;

/// The pixels of a sheet at 100 % in a point of paper.
const pixelsPerPoint = 96 / 72;

/// How a sheet goes on paper, as the Go package reads it into its "print".
class PrintSetup {
  const PrintSetup(this.json);

  factory PrintSetup.of(Node sheet) {
    final json = sheet.attributes['print'];
    return PrintSetup(json is Map<String, Object?> ? json : const {});
  }

  final Map<String, Object?> json;

  /// The paper, A4 when the sheet names none or one that is not known.
  int get paper => _papers.containsKey(json['paper']) ? json['paper']! as int : paperA4;

  bool get landscape => json['land'] == true;

  /// The size of a page, in points.
  Size get page {
    final size = _papers[paper]!;
    return landscape ? Size(size.height, size.width) : size;
  }

  /// The scale of the sheet when it is not fitted to pages.
  double get scale => (json['scale'] is int ? json['scale']! as int : 100) / 100;

  /// The pages the sheet is shrunk to, wide and tall, 0 for as many as it
  /// takes; null when it prints at its [scale].
  (int, int)? get fit {
    final f = json['fit'];
    return f is List<Object?> && f.length == 2 && f[0] is int && f[1] is int ? (f[0]! as int, f[1]! as int) : null;
  }

  /// Whether the pages of a row are numbered before those under them.
  bool get across => json['across'] == true;

  /// The number of the first page, null when it goes on from the sheet
  /// before.
  int? get first => json['first'] as int?;

  double _margin(String side, double normal) {
    final m = json['margins'];
    final v = m is Map<String, Object?> ? m[side] : null;
    return (v is num ? v.toDouble() : normal) * 72;
  }

  /// The margins of the page, in points.
  EdgeInsets get margins => EdgeInsets.fromLTRB(_margin('l', 0.7), _margin('t', 0.75), _margin('r', 0.7), _margin('b', 0.75));

  /// How far the header and the footer are from the edges of the page.
  double get headerMargin => _margin('head', 0.3);
  double get footerMargin => _margin('foot', 0.3);

  bool get centerH => json['ch'] == true;
  bool get centerV => json['cv'] == true;

  /// Whether the gridlines are printed, and the letters and numbers of the
  /// columns and rows.
  bool get gridlines => json['grid'] == true;
  bool get headings => json['heads'] == true;

  /// The rows and the columns a page starts at.
  List<int> get rowBreaks => _ints(json['rows']);
  List<int> get colBreaks => _ints(json['cols']);

  static List<int> _ints(Object? v) => v is List<Object?> ? [for (final i in v) if (i is int) i] : const [];

  /// The header and the footer of page [number], the [first] of its sheet
  /// or not, in the codes of Excel.
  (String, String) headerFooter(int number, {required bool first}) {
    (String, String)? own(String key) {
      final v = json[key];
      return v is List<Object?> && v.length == 2 ? ('${v[0] ?? ''}', '${v[1] ?? ''}') : null;
    }

    final firstPage = first ? own('firstPage') : null;
    final even = number.isEven ? own('even') : null;
    return firstPage ?? even ?? ('${json['head'] ?? ''}', '${json['foot'] ?? ''}');
  }

  /// The settings with some changed, those set to null, false, 0 or
  /// nothing left out as the Go package leaves them out.
  Map<String, Object?> change(Map<String, Object?> changes) {
    final out = {...json, ...changes};
    out.removeWhere((_, v) => v == null || v == false || v == 0 || v == '' || v is List<Object?> && v.isEmpty);
    return out;
  }
}

/// The size of the letters and numbers printed beside the cells, in pixels
/// at 100 %.
const headingWidth = 40.0, headingHeight = 20.0;

/// A page of a sheet: the cells it shows, under and beside those every
/// page repeats.
class SheetPage {
  const SheetPage(this.cells, {this.titleRows, this.titleCols});

  final CellArea cells;

  /// The rows repeated over the cells, in the columns of the page, and the
  /// columns repeated beside them, in its rows.
  final (int, int)? titleRows, titleCols;
}

/// The pages a sheet prints on: its cells cut where the paper ends and
/// where its breaks are, at the scale it asks for or the one that fits.
class SheetPages {
  SheetPages(this.book, this.sheet) : setup = PrintSetup.of(sheet), layout = book.layout(sheet) {
    _titles();
    final areas = _areas();
    final fit = setup.fit;
    scale = fit == null ? setup.scale : _fitting(areas, fit);
    pages = [for (final a in areas) ..._cut(a, scale, breaks: fit == null)];
  }

  final Workbook book;
  final Node sheet;
  final PrintSetup setup;
  final SheetLayout layout;

  /// The scale the cells are printed at.
  late final double scale;
  late final List<SheetPage> pages;

  (int, int)? _titleRows, _titleCols;

  /// The size of the page left to the cells, in pixels of the sheet at
  /// 100 % once scaled by [scale].
  Size _room(double scale) {
    final inner = setup.margins.deflateSize(setup.page) * pixelsPerPoint / scale;
    final heads = setup.headings ? const Offset(headingWidth, headingHeight) : Offset.zero;
    return Size(math.max(inner.width - heads.dx, 1), math.max(inner.height - heads.dy, 1));
  }

  String? _name(String name) {
    final names = book.tree['book']?.attributes['names'];
    if (names is! List<Object?>) return null;
    for (final n in names) {
      if (n is Map<String, Object?> && n['sheet'] == sheet.id && '${n['name']}'.toLowerCase() == name.toLowerCase()) {
        return n['ref'] as String?;
      }
    }
    return null;
  }

  /// The areas of this sheet a name lists, between commas.
  List<CellArea> _named(String name) => [
    for (final part in _split(_name(name) ?? ''))
      if (book.resolve(sheet, part) case (final on, final area) when on.id == sheet.id) area,
  ];

  /// The parts of a list of references: the commas of a sheet name in
  /// quotes are its own.
  static List<String> _split(String refs) {
    final out = <String>[];
    var quoted = false, start = 0;
    for (var i = 0; i < refs.length; i++) {
      if (refs[i] == "'") quoted = !quoted;
      if (refs[i] == ',' && !quoted) {
        out.add(refs.substring(start, i));
        start = i + 1;
      }
    }
    return [...out, refs.substring(start)].map((s) => s.trim()).where((s) => s.isNotEmpty).toList();
  }

  void _titles() {
    for (final a in _named(printTitlesName)) {
      if (a.wholeRows && !a.wholeCols) _titleRows = (a.top, a.bottom);
      if (a.wholeCols && !a.wholeRows) _titleCols = (a.left, a.right);
    }
  }

  /// What the sheet prints: the areas it names, or its cells from the
  /// first one to the last that holds or shows something.
  List<CellArea> _areas() {
    final named = [
      for (final a in _named(printAreaName))
        CellArea(a.top, a.left, math.min(a.bottom, math.max(layout.used, a.top)), math.min(a.right, math.max(layout.usedCols, a.left))),
    ];
    if (named.isNotEmpty) return named;
    var bottom = 0, right = 0;
    void reach(int row, int col) {
      bottom = math.max(bottom, row);
      right = math.max(right, col);
    }

    final grid = sheet.grid ?? const Grid.empty();
    for (final c in grid.cells) {
      if (c.row == 0 || c.col == 0) continue;
      final f = c.fields;
      final m = layout.mergeAt(c.row, c.col);
      if (m != null && m.top == c.row && m.left == c.col) reach(m.bottom, m.right);
      if (f['v'] != null || f['e'] != null) {
        reach(c.row, c.col);
      } else if (f['s'] is String) {
        final s = book.style(f['s']! as String);
        if (s.fill != null || s.left != null || s.right != null || s.top != null || s.bottom != null) reach(c.row, c.col);
      }
    }
    for (final c in book.looks(sheet).grid.cells) {
      reach(c.row, c.col);
    }
    for (final chart in SheetChart.of(sheet)) {
      final r = chart.rect(layout);
      reach(layout.rowAt(r.bottom - 1), layout.colAt(r.right - 1));
    }
    return bottom == 0 ? const [] : [CellArea(1, 1, bottom, right)];
  }

  double _span(double Function(int) size, (int, int)? lines) {
    var sum = 0.0;
    if (lines != null) {
      for (var i = lines.$1; i <= lines.$2; i++) {
        sum += size(i);
      }
    }
    return sum;
  }

  /// The lines from [first] to [last] cut into runs no longer than [room],
  /// a new one at each of [breaks]; the runs past the lines every page
  /// repeats leave them their place.
  List<(int, int)> _runs(int first, int last, double Function(int) size, double room, List<int> breaks, (int, int)? titles) {
    final out = <(int, int)>[];
    final repeated = _span(size, titles);
    var start = first;
    var used = 0.0;
    double left() => titles != null && start > titles.$2 && repeated < room ? room - repeated : room;
    for (var i = first; i <= last; i++) {
      final s = size(i);
      if (i > start && (breaks.contains(i) || used + s > left() + 0.01)) {
        out.add((start, i - 1));
        start = i;
        used = 0;
      }
      used += s;
    }
    return [...out, (start, last)];
  }

  List<SheetPage> _cut(CellArea area, double scale, {required bool breaks}) {
    final room = _room(scale);
    final cols = _runs(area.left, area.right, layout.width, room.width, breaks ? setup.colBreaks : const [], _titleCols);
    final rows = _runs(area.top, area.bottom, layout.height, room.height, breaks ? setup.rowBreaks : const [], _titleRows);
    SheetPage page((int, int) r, (int, int) c) => SheetPage(
      CellArea(r.$1, c.$1, r.$2, c.$2),
      titleRows: _titleRows != null && r.$1 > _titleRows!.$2 ? _titleRows : null,
      titleCols: _titleCols != null && c.$1 > _titleCols!.$2 ? _titleCols : null,
    );
    // what is hidden to the last line prints nothing
    cols.removeWhere((c) => _span(layout.width, c) == 0);
    rows.removeWhere((r) => _span(layout.height, r) == 0);
    return setup.across
        ? [for (final r in rows) for (final c in cols) page(r, c)]
        : [for (final c in cols) for (final r in rows) page(r, c)];
  }

  /// The largest scale, in hundredths and no more than 1, at which the
  /// areas take no more pages than [fit] allows.
  double _fitting(List<CellArea> areas, (int, int) fit) {
    bool fits(int percent) {
      final room = _room(percent / 100);
      for (final a in areas) {
        if (fit.$1 > 0 && _runs(a.left, a.right, layout.width, room.width, const [], _titleCols).length > fit.$1) return false;
        if (fit.$2 > 0 && _runs(a.top, a.bottom, layout.height, room.height, const [], _titleRows).length > fit.$2) return false;
      }
      return true;
    }

    var low = 10, high = 100;
    while (low < high) {
      final mid = (low + high + 1) >> 1;
      if (fits(mid)) {
        low = mid;
      } else {
        high = mid - 1;
      }
    }
    return low / 100;
  }
}
