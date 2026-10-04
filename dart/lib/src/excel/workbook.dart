import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../drawing/color.dart';
import 'conditional.dart';
import 'formula_text.dart';
import 'lists.dart';
import 'number_format.dart';

/// The theme colors of a workbook in the order its cells count them.
const themeNames = ['lt1', 'dk1', 'lt2', 'dk2', 'accent1', 'accent2', 'accent3', 'accent4', 'accent5', 'accent6', 'hlink', 'folHlink'];

/// A workbook as the Go package reads it into a tree: its sheets, the
/// formats of its cells and the colors of its theme.
class Workbook {
  Workbook(this.tree) {
    final theme = tree['book']?.attributes['theme'];
    if (theme is Map<String, Object?>) {
      final colors = theme['colors'];
      if (colors is List<Object?>) {
        themeColors = [for (final c in colors) _rgb(c) ?? const Color(0xFF000000)];
      }
      if (theme['minor'] is String) minorFont = theme['minor']! as String;
      if (theme['major'] is String) majorFont = theme['major']! as String;
    }
    date1904 = tree['book']?.attributes['date1904'] == true;
    csv = tree['book']?.attributes['csv'] == true;
  }

  final Tree tree;
  var themeColors = const [
    Color(0xFFFFFFFF), Color(0xFF000000), Color(0xFFE7E6E6), Color(0xFF44546A), Color(0xFF4472C4), Color(0xFFED7D31), //
    Color(0xFFA5A5A5), Color(0xFFFFC000), Color(0xFF5B9BD5), Color(0xFF70AD47), Color(0xFF0563C1), Color(0xFF954F72),
  ];
  var minorFont = 'Calibri';
  var majorFont = 'Calibri Light';
  var date1904 = false;

  /// Whether the workbook is a CSV file: one sheet, whose values alone are
  /// saved.
  var csv = false;
  final _styles = <String, CellStyle>{};
  final _layouts = <String, (Node, SheetLayout)>{};
  final _looks = <String, (Node, Node?, SheetLooks)>{};

  /// The sheets in order, chart sheets included.
  List<Node> get sheets => [for (final n in tree.children('book')) if (n.type == 'sheet' || n.type == 'kept') n];

  /// The format of cells whose "s" field is [id], the default one for null.
  CellStyle style(String? id) {
    final key = id ?? 'x0';
    final cached = _styles[key];
    if (cached != null) return cached;
    final node = tree[key];
    final json = node?.attributes['style'];
    return _styles[key] = CellStyle.fromJson(json is Map<String, Object?> ? json : const {}, this);
  }

  /// The layout of a sheet, kept until the sheet changes.
  SheetLayout layout(Node sheet) {
    final cached = _layouts[sheet.id];
    if (cached != null && identical(cached.$1, sheet)) return cached.$2;
    final l = SheetLayout(sheet);
    _layouts[sheet.id] = (sheet, l);
    return l;
  }

  /// What the conditional formats of a sheet give its cells, kept until
  /// the sheet or its looks change.
  SheetLooks looks(Node sheet) {
    final node = tree.children(sheet.id).where((n) => n.type == 'looks').firstOrNull;
    final cached = _looks[sheet.id];
    if (cached != null && identical(cached.$1, sheet) && identical(cached.$2, node)) return cached.$3;
    final l = SheetLooks(this, sheet, node);
    _looks[sheet.id] = (sheet, node, l);
    return l;
  }

  /// The sheet and area a reference or a defined name points to, the
  /// sheet named or [sheet].
  (Node, CellArea)? resolve(Node sheet, String ref) => _area(sheet, ref) ?? _named(sheet, ref);

  (Node, CellArea)? _area(Node sheet, String ref) {
    var on = sheet;
    var a = ref;
    final bang = ref.lastIndexOf('!');
    if (bang > 0) {
      var name = ref.substring(0, bang);
      if (name.length >= 2 && name.startsWith("'") && name.endsWith("'")) {
        name = name.substring(1, name.length - 1).replaceAll("''", "'");
      }
      final found = sheets.where((n) => '${n.attributes['name']}'.toLowerCase() == name.toLowerCase()).firstOrNull;
      if (found == null) return null;
      on = found;
      a = ref.substring(bang + 1);
    }
    final area = parseArea(a);
    return area == null ? null : (on, area);
  }

  /// The sheet and area of a defined name, the name of [sheet] first.
  (Node, CellArea)? _named(Node sheet, String name) {
    final names = tree['book']?.attributes['names'];
    if (names is! List<Object?>) return null;
    Map<String, Object?>? best;
    for (final n in names) {
      if (n is! Map<String, Object?> || '${n['name']}'.toLowerCase() != name.toLowerCase()) continue;
      if (n['sheet'] == sheet.id || n['sheet'] == null && best == null) best = n;
    }
    final ref = best?['ref'];
    return ref is String ? _area(sheet, ref) : null;
  }

  /// The colors of the theme by the names DrawingML gives them.
  ColorContext get colors => ColorContext(scheme: {for (var i = 0; i < themeNames.length && i < themeColors.length; i++) themeNames[i]: themeColors[i]});

  /// A color as the Go package writes those of cells: RGB, a color of the
  /// theme lightened or darkened by a tint, automatic.
  Color? color(Object? json) {
    if (json is! Map<String, Object?>) return null;
    if (json['auto'] == true) return null;
    final rgb = _rgb(json['rgb']);
    final theme = json['theme'];
    var base = rgb ?? (theme is int && theme >= 0 && theme < themeColors.length ? themeColors[theme] : null);
    if (base == null) return null;
    final tint = json['tint'];
    if (tint is num && tint != 0) base = tinted(base, tint.toDouble());
    return base;
  }

  static Color? _rgb(Object? s) {
    if (s is! String || s.length != 6) return null;
    final v = int.tryParse(s, radix: 16);
    return v == null ? null : Color(0xFF000000 | v);
  }
}

/// A color lightened, or darkened when [tint] is negative, as Excel
/// applies tints to the colors of a theme.
Color tinted(Color c, double tint) {
  final hsl = HSLColor.fromColor(c);
  var l = hsl.lightness;
  l = tint < 0 ? l * (1 + tint) : l * (1 - tint) + tint;
  return hsl.withLightness(l.clamp(0, 1)).toColor();
}

/// A color the palette of the ribbon offers, as a cell's color: the theme
/// colors' shades are tints.
Map<String, Object?>? cellColor(Map<String, Object?>? picked) {
  if (picked == null) return null;
  if (picked['rgb'] is String) return {'rgb': picked['rgb']};
  final index = themeNames.indexOf('${picked['scheme']}');
  if (index < 0) return null;
  var tint = 0.0;
  final mods = picked['mods'];
  if (mods is List<Object?>) {
    var mod = 1.0, off = 0.0;
    for (final m in mods) {
      if (m is List<Object?> && m.length == 2 && m[1] is num) {
        if (m[0] == 'lumMod') mod = (m[1]! as num) / 100000;
        if (m[0] == 'lumOff') off = (m[1]! as num) / 100000;
      }
    }
    tint = off > 0 ? off : mod - 1;
  }
  return {'theme': index, if (tint != 0) 'tint': double.parse(tint.toStringAsFixed(4))};
}

/// A border of a cell: its style name and color.
class Edge {
  const Edge(this.style, this.color);

  final String style;
  final Color color;

  double get width => switch (style) {
    'medium' || 'mediumDashed' || 'mediumDashDot' || 'mediumDashDotDot' || 'slantDashDot' => 2,
    'thick' => 3,
    'double' => 3,
    _ => 1,
  };
}

/// The format of a cell: its number format, font, fill, borders and
/// alignment.
class CellStyle {
  CellStyle._(this.json, this.formatCode, this.font, this.fill, this.left, this.right, this.top, this.bottom, this.align);

  factory CellStyle.fromJson(Map<String, Object?> json, Workbook book) {
    Map<String, Object?> m(String key) => json[key] is Map<String, Object?> ? json[key]! as Map<String, Object?> : const {};
    final font = m('font');
    final fill = m('fill');
    final border = m('border');
    Edge? edge(String side) {
      final e = border[side];
      if (e is! Map<String, Object?> || e['style'] is! String || e['style'] == 'none') return null;
      return Edge(e['style']! as String, book.color(e['color']) ?? const Color(0xFF000000));
    }

    Color? background;
    final pattern = fill['pattern'];
    if (pattern == 'solid') {
      background = book.color(fill['fg']);
    } else if (pattern is String && pattern != 'none') {
      background = book.color(fill['fg']) ?? book.color(fill['bg']);
    } else if (fill['gradient'] is List<Object?> && (fill['gradient']! as List<Object?>).isNotEmpty) {
      background = book.color((fill['gradient']! as List<Object?>).first);
    }
    final scheme = font['scheme'];
    final name = scheme == 'major' ? book.majorFont : (scheme == 'minor' ? book.minorFont : '${font['name'] ?? book.minorFont}');
    final size = font['sz'] is num ? (font['sz']! as num).toDouble() : 11.0;
    return CellStyle._(
      json,
      json['fmt'] is String ? json['fmt']! as String : 'General',
      CellFont(
        name: name,
        size: size,
        bold: font['b'] == true,
        italic: font['i'] == true,
        underline: font['u'] is String,
        strike: font['strike'] == true,
        color: book.color(font['color']),
      ),
      background,
      edge('l'),
      edge('r'),
      edge('t'),
      edge('b'),
      json['align'] is Map<String, Object?> ? json['align']! as Map<String, Object?> : const {},
    );
  }

  /// The format as the Go package writes it, to derive others from.
  final Map<String, Object?> json;
  final String formatCode;
  final CellFont font;
  final Color? fill;
  final Edge? left, right, top, bottom;
  final Map<String, Object?> align;

  NumberFormat get format => NumberFormat(formatCode);

  CellStyle withFill(Color fill) => CellStyle._(json, formatCode, font, fill, left, right, top, bottom, align);

  String? get horizontal => align['h'] as String?;

  String get vertical => align['v'] as String? ?? 'bottom';

  bool get wrap => align['wrap'] == true;

  int get indent => align['indent'] is int ? align['indent']! as int : 0;
}

class CellFont {
  const CellFont({
    required this.name,
    required this.size,
    this.bold = false,
    this.italic = false,
    this.underline = false,
    this.strike = false,
    this.color,
  });

  final String name;
  final double size;
  final bool bold, italic, underline, strike;
  final Color? color;
}

/// The fonts a system is sure to have in place of Office's: those of the
/// same widths first.
String substituteFont(String name) => switch (name) {
  'Calibri' || 'Calibri Light' || 'Aptos' || 'Aptos Narrow' => 'Carlito',
  'Cambria' => 'Caladea',
  'Arial' || 'Helvetica' => 'Liberation Sans',
  'Times New Roman' => 'Liberation Serif',
  'Courier New' => 'Liberation Mono',
  _ => name,
};

/// The size of the rows and columns of a sheet, in pixels at 100 %, its
/// merged cells and its frozen panes.
class SheetLayout {
  SheetLayout(this.sheet) {
    final attrs = sheet.attributes;
    final dh = attrs['dh'];
    defaultHeight = (dh is num && dh > 0 ? dh.toDouble() : 15) * 4 / 3;
    final dw = attrs['dw'];
    defaultWidth = dw is num && dw > 0 ? ((dw * 7) / 8).ceilToDouble() * 8 : 64;
    final frozen = attrs['frozen'];
    if (frozen is Map<String, Object?>) {
      frozenRows = frozen['r'] is int ? frozen['r']! as int : 0;
      frozenCols = frozen['c'] is int ? frozen['c']! as int : 0;
    }
    gridlines = attrs['grid'] != false;
    final tail = attrs['tail'];
    if (tail is Map<String, Object?> && tail['from'] is int) {
      _tailFrom = tail['from']! as int;
      _tailWidth = tail['hide'] == true ? 0 : (tail['w'] is num ? _px((tail['w']! as num).toDouble()) : defaultWidth);
    }
    final grid = sheet.grid ?? const Grid.empty();
    for (final c in grid.rows(0, 0)) {
      if (c.col > 0) _widths[c.col] = c.fields['hide'] == true ? 0 : (c.fields['w'] is num ? _px((c.fields['w']! as num).toDouble()) : defaultWidth);
    }
    for (final row in grid.rowNumbers) {
      if (row == 0) continue;
      final f = grid.cell(row, 0);
      if (f == null) continue;
      if (f['hide'] == true) {
        _heights[row] = 0;
      } else if (f['h'] is num) {
        _heights[row] = (f['h']! as num).toDouble() * 4 / 3;
      }
      used = math.max(used, row);
    }
    for (final row in grid.rowNumbers) {
      if (row > 0) used = math.max(used, row);
    }
    for (final c in grid.cells) {
      if (c.row == 0 || c.col == 0) continue;
      usedCols = math.max(usedCols, c.col);
      final m = c.fields['m'];
      if (m is List<Object?> && m.length == 2 && m[0] is int && m[1] is int) {
        final area = CellArea(c.row, c.col, c.row + (m[0]! as int) - 1, c.col + (m[1]! as int) - 1);
        _merges.add(area);
      }
    }
    _rowKeys = _heights.keys.toList()..sort();
    _rowOffsets = [];
    var sum = 0.0;
    for (final r in _rowKeys) {
      _rowOffsets.add(sum);
      sum += _heights[r]! - defaultHeight;
    }
    _rowDeltaTotal = sum;
  }

  final Node sheet;
  late final double defaultHeight;
  late final double defaultWidth;
  var frozenRows = 0;
  var frozenCols = 0;
  var gridlines = true;

  /// The last row and column holding cells.
  var used = 0;
  var usedCols = 0;

  final _widths = <int, double>{};
  final _heights = <int, double>{};
  final _merges = <CellArea>[];
  int? _tailFrom;
  double? _tailWidth;
  late final List<int> _rowKeys;
  late final List<double> _rowOffsets;
  late final double _rowDeltaTotal;
  List<double>? _colStarts;

  static double _px(double chars) => (chars * 7).roundToDouble();

  double width(int col) {
    final w = _widths[col];
    if (w != null) return w;
    if (_tailFrom != null && col >= _tailFrom!) return _tailWidth!;
    return defaultWidth;
  }

  double height(int row) => _heights[row] ?? defaultHeight;

  /// Where column [col] starts, column 1 at 0.
  double x(int col) {
    final starts = _colStarts ??= () {
      final out = List.filled(maxCols + 2, 0.0);
      for (var c = 1; c <= maxCols + 1; c++) {
        out[c] = out[c - 1] + (c > 1 ? width(c - 1) : 0);
      }
      return out;
    }();
    return starts[col.clamp(1, maxCols + 1)];
  }

  /// Where row [row] starts, row 1 at 0.
  double y(int row) {
    // the rows of their own height before this one
    var lo = 0, hi = _rowKeys.length;
    while (lo < hi) {
      final mid = (lo + hi) >> 1;
      if (_rowKeys[mid] < row) {
        lo = mid + 1;
      } else {
        hi = mid;
      }
    }
    final delta = lo < _rowKeys.length ? _rowOffsets[lo] : _rowDeltaTotal;
    return (row - 1) * defaultHeight + delta;
  }

  /// The column at [x], the last one past the sheet.
  int colAt(double x) {
    var lo = 1, hi = maxCols;
    while (lo < hi) {
      final mid = (lo + hi + 1) >> 1;
      if (this.x(mid) <= x) {
        lo = mid;
      } else {
        hi = mid - 1;
      }
    }
    return lo;
  }

  int rowAt(double y) {
    var lo = 1, hi = maxRows;
    while (lo < hi) {
      final mid = (lo + hi + 1) >> 1;
      if (this.y(mid) <= y) {
        lo = mid;
      } else {
        hi = mid - 1;
      }
    }
    return lo;
  }

  double get totalWidth => x(maxCols) + width(maxCols);

  double get totalHeight => y(maxRows) + height(maxRows);

  /// The merged area a cell belongs to, if any.
  CellArea? mergeAt(int row, int col) {
    for (final m in _merges) {
      if (m.contains(row, col)) return m;
    }
    return null;
  }

  List<CellArea> get merges => _merges;

  late final List<ListRule> lists = ListRule.of(sheet);

  /// The list validating a cell, if any.
  ListRule? listAt(int row, int col) => lists.where((l) => l.contains(row, col)).firstOrNull;
}

/// A rectangle of cells, from [top] [left] to [bottom] [right] included.
class CellArea {
  const CellArea(int r1, int c1, int r2, int c2)
    : top = r1 < r2 ? r1 : r2,
      left = c1 < c2 ? c1 : c2,
      bottom = r1 < r2 ? r2 : r1,
      right = c1 < c2 ? c2 : c1;

  const CellArea.cell(int row, int col) : this(row, col, row, col);

  final int top, left, bottom, right;

  bool contains(int row, int col) => top <= row && row <= bottom && left <= col && col <= right;

  bool intersects(CellArea o) => o.left <= right && o.right >= left && o.top <= bottom && o.bottom >= top;

  CellArea union(CellArea o) =>
      CellArea(math.min(top, o.top), math.min(left, o.left), math.max(bottom, o.bottom), math.max(right, o.right));

  int get rows => bottom - top + 1;

  int get cols => right - left + 1;

  bool get single => top == bottom && left == right;

  bool get wholeRows => left == 1 && right == maxCols;

  bool get wholeCols => top == 1 && bottom == maxRows;

  /// The area as a formula writes it: A1, A1:B3, A:C, 1:3.
  String get name {
    if (wholeCols && wholeRows) return '1:$maxRows';
    if (wholeCols) return '${columnName(left)}:${columnName(right)}';
    if (wholeRows) return '$top:$bottom';
    final a = cellName(top, left);
    return single ? a : '$a:${cellName(bottom, right)}';
  }

  @override
  bool operator ==(Object other) =>
      other is CellArea && other.top == top && other.left == left && other.bottom == bottom && other.right == right;

  @override
  int get hashCode => Object.hash(top, left, bottom, right);

  @override
  String toString() => name;
}

/// The letters of a column: 1 is A, 27 is AA.
String columnName(int col) {
  final out = <int>[];
  while (col > 0) {
    col--;
    out.add(0x41 + col % 26);
    col ~/= 26;
  }
  return String.fromCharCodes(out.reversed);
}

String cellName(int row, int col) => '${columnName(col)}$row';

/// Reads an A1 cell name, "$" allowed.
(int, int)? parseCell(String s) {
  final m = RegExp(r'^\$?([A-Za-z]{1,3})\$?([0-9]{1,7})$').firstMatch(s.trim());
  if (m == null) return null;
  var col = 0;
  for (final c in m[1]!.toUpperCase().codeUnits) {
    col = col * 26 + c - 0x40;
  }
  final row = int.parse(m[2]!);
  if (col < 1 || col > maxCols || row < 1 || row > maxRows) return null;
  return (row, col);
}

/// Reads an area: A1, A1:B3, A:C, 1:3.
CellArea? parseArea(String s) {
  final parts = s.split(':');
  if (parts.length > 2) return null;
  if (parts.length == 2) {
    final (a, b) = (parts[0].trim(), parts[1].trim());
    if (_letters.hasMatch(a) && _letters.hasMatch(b)) {
      final (x, y) = (parseCell('${a}1'), parseCell('${b}1'));
      return x == null || y == null ? null : CellArea(1, x.$2, maxRows, y.$2);
    }
    if (_digits.hasMatch(a) && _digits.hasMatch(b)) {
      final (x, y) = (parseCell('A$a'), parseCell('A$b'));
      return x == null || y == null ? null : CellArea(x.$1, 1, y.$1, maxCols);
    }
  }
  final a = parseCell(parts.first);
  final b = parts.length == 2 ? parseCell(parts.last) : a;
  if (a == null || b == null) return null;
  return CellArea(a.$1, a.$2, b.$1, b.$2);
}

final _letters = RegExp(r'^\$?[A-Za-z]{1,3}$');
final _digits = RegExp(r'^\$?[0-9]{1,7}$');

/// What a cell shows: its text, where it goes, and its color.
class CellText {
  const CellText(this.text, this.align, this.color, {this.number = false});

  final String text;

  /// 'left', 'center' or 'right'.
  final String align;
  final Color? color;

  /// Whether the text is a number, which shows ### when it does not fit.
  final bool number;
}

/// The text a cell with [fields] shows in [style].
CellText cellText(Map<String, Object?> fields, CellStyle style, NumberLocale locale, {bool date1904 = false}) {
  final named = _colorsByName;
  final e = fields['e'];
  final h = style.horizontal;
  String place(String natural) => switch (h) {
    'left' || 'right' || 'center' => h!,
    'centerContinuous' || 'distributed' => 'center',
    'justify' || 'fill' => 'left',
    _ => natural,
  };
  if (e is String) return CellText(errorText(e), place('center'), null);
  final v = fields['v'];
  if (v is num) {
    final f = style.format.format(v.toDouble(), locale, date1904: date1904);
    return CellText(f.text, place('right'), named[f.color], number: true);
  }
  if (v is bool) return CellText(v ? locale.trueText : locale.falseText, place('center'), null);
  if (v is String) {
    final f = style.format.formatText(v);
    return CellText(f.text, place('left'), named[f.color]);
  }
  return CellText('', place('left'), null);
}

const _colorsByName = <String?, Color>{
  'black': Color(0xFF000000),
  'blue': Color(0xFF0000FF),
  'cyan': Color(0xFF00FFFF),
  'green': Color(0xFF00FF00),
  'magenta': Color(0xFFFF00FF),
  'red': Color(0xFFFF0000),
  'white': Color(0xFFFFFFFF),
  'yellow': Color(0xFFFFFF00),
};
