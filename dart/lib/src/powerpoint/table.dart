import 'dart:convert';
import 'dart:math' as math;
import 'dart:ui';

import 'package:trame/trame.dart';

import '../drawing/color.dart';
import '../drawing/paint.dart';
import '../text/text_frame.dart';
import 'deck.dart';

/// A line or fill as the Go package writes it, with the colors it is
/// resolved in.
typedef Styled = (Map<String, Object?>, ColorContext);

/// A table laid out: where its columns and rows are, in points from the
/// corner of its frame, and its cells as they are drawn. Rows grow to hold
/// their text, as PowerPoint grows them.
class TableLayout {
  TableLayout._(this.columns, this.rows, this.fits, this.cells, this.slots, this.fill, this._nodes);

  /// Lays out the table of a frame, in the colors and theme of its slide.
  factory TableLayout.of(Deck deck, Node table, {required ColorContext colors, required DeckTheme theme, String? fonts}) {
    final tree = deck.tree;
    final rowNodes = _rows(tree, table);
    final widths = [
      for (final w in table.attributes['grid'] is List<Object?> ? table.attributes['grid']! as List<Object?> : const <Object?>[])
        if (w is num) w / emuPerPoint,
    ];
    final placed = <_Placed>[];
    final slots = <List<Node?>>[];
    var cols = widths.length;
    for (var r = 0; r < rowNodes.length; r++) {
      final slot = <Node?>[];
      var covered = 0;
      for (final tc in tree.children(rowNodes[r].id)) {
        if (tc.type != 'tc') continue;
        if (covered > 0 && tc.attributes['hMerge'] == true) {
          covered--;
          slot.add(tc);
          continue;
        }
        for (; covered > 0; covered--) {
          slot.add(null);
        }
        final span = _count(tc.attributes['gridSpan']);
        if (tc.attributes['vMerge'] != true) placed.add(_Placed(tc, r, slot.length, _count(tc.attributes['rowSpan']), span));
        slot.add(tc);
        covered = span - 1;
      }
      for (; covered > 0; covered--) {
        slot.add(null);
      }
      slots.add(slot);
      cols = math.max(cols, slot.length);
    }
    for (final slot in slots) {
      while (slot.length < cols) {
        slot.add(null);
      }
    }
    while (widths.length < cols) {
      widths.add(widths.isEmpty ? 72 : widths.last);
    }
    final columns = [0.0];
    for (final w in widths) {
      columns.add(columns.last + w);
    }

    final options = _map(table.attributes['tbl']);
    final style = _Style(deck.tableStyleOf(table), options, rowNodes.length, cols, colors, theme);
    final face = Fonts(theme: theme.typeface, package: fonts);
    final fits = List.filled(rowNodes.length, 0.0);
    final texts = <TextFrame>[];
    final levels = <Props Function(int)>[];
    for (final p in placed) {
      p.rowSpan = math.min(p.rowSpan, rowNodes.length - p.row);
      p.colSpan = math.min(p.colSpan, cols - p.col);
      final width = columns[p.col + p.colSpan] - columns[p.col];
      final inherited = ShapeStyle(levels: deck.cellLevels(p.node)), own = style.text(p);
      levels.add((lvl) => {...inherited.level(lvl), ...own});
      final text = _layout(p.node, levels.last, face, colors, Size(width, 0), measure: true);
      texts.add(text);
      if (p.rowSpan == 1) fits[p.row] = math.max(fits[p.row], _needed(text, p.node));
    }
    final heights = [for (var r = 0; r < rowNodes.length; r++) math.max(fits[r], _number(rowNodes[r].attributes['h']) / emuPerPoint)];
    for (var i = 0; i < placed.length; i++) {
      final p = placed[i];
      if (p.rowSpan == 1) continue;
      final last = p.row + p.rowSpan - 1;
      final have = heights.sublist(p.row, last + 1).fold(0.0, (a, b) => a + b);
      heights[last] += math.max(0, _needed(texts[i], p.node) - have);
    }
    final rows = [0.0];
    for (final h in heights) {
      rows.add(rows.last + h);
    }

    final rtl = options['rtl'] == true;
    final cells = <String, CellLayout>{};
    for (var i = 0; i < placed.length; i++) {
      final p = placed[i];
      var left = columns[p.col], right = columns[p.col + p.colSpan];
      if (rtl) (left, right) = (columns.last - right, columns.last - left);
      final rect = Rect.fromLTRB(left, rows[p.row], right, rows[p.row + p.rowSpan]);
      final anchored = _anchor(p.node) != 't' || _vertical(p.node);
      cells[p.node.id] = CellLayout._(
        p.node,
        p.row,
        p.col,
        p.rowSpan,
        p.colSpan,
        rect,
        anchored ? _layout(p.node, levels[i], face, colors, rect.size) : texts[i],
        levels[i],
        style.fill(p),
        style.lines(p),
      );
    }
    final tableFill = options['fill'] is Map<String, Object?> ? (options['fill']! as Map<String, Object?>, colors) : null;
    return TableLayout._(columns, rows, fits, cells, slots, tableFill, nodesOf(tree, table));
  }

  /// The edges of the columns and of the rows, the first at 0.
  final List<double> columns;
  final List<double> rows;

  /// The height each row needs for the text of its cells, those spanning
  /// rows left out.
  final List<double> fits;

  /// The cells drawn, those merged into another left out, by node id.
  final Map<String, CellLayout> cells;

  /// The cell node of each row at each column, those a span covers
  /// included; null where a row lacks one.
  final List<List<Node?>> slots;

  /// The fill under all the cells.
  final Styled? fill;

  final List<Node> _nodes;

  Size get size => Size(columns.last, rows.last);

  /// The nodes a table is laid out from: itself, its rows and cells.
  static List<Node> nodesOf(Tree tree, Node table) => [
    table,
    for (final r in _rows(tree, table)) ...[r, ...tree.children(r.id)],
  ];

  /// Whether [nodes] are those the table was laid out from.
  bool isOf(List<Node> nodes) {
    if (nodes.length != _nodes.length) return false;
    for (var i = 0; i < nodes.length; i++) {
      if (!identical(nodes[i], _nodes[i])) return false;
    }
    return true;
  }

  /// The cell drawn over a row and column.
  CellLayout? cellOver(int row, int col) {
    for (final c in cells.values) {
      if (row >= c.row && row < c.row + c.rowSpan && col >= c.col && col < c.col + c.colSpan) return c;
    }
    return null;
  }

  /// The cells of the smallest block of rows and columns holding [a] and
  /// [b] that cuts no merged cell, row by row.
  List<CellLayout> range(CellLayout a, CellLayout b) {
    var top = math.min(a.row, b.row), left = math.min(a.col, b.col);
    var bottom = math.max(a.row + a.rowSpan, b.row + b.rowSpan), right = math.max(a.col + a.colSpan, b.col + b.colSpan);
    bool overlaps(CellLayout c) => c.row < bottom && c.row + c.rowSpan > top && c.col < right && c.col + c.colSpan > left;
    for (var grown = true; grown;) {
      grown = false;
      for (final c in cells.values.where(overlaps)) {
        if (c.row < top || c.col < left || c.row + c.rowSpan > bottom || c.col + c.colSpan > right) {
          top = math.min(top, c.row);
          left = math.min(left, c.col);
          bottom = math.max(bottom, c.row + c.rowSpan);
          right = math.max(right, c.col + c.colSpan);
          grown = true;
        }
      }
    }
    return cells.values.where(overlaps).toList()..sort((x, y) => x.row != y.row ? x.row - y.row : x.col - y.col);
  }

  /// The cell at a point, null outside the table.
  CellLayout? cellAt(Offset p) {
    for (final c in cells.values) {
      if (c.rect.contains(p)) return c;
    }
    return null;
  }

  void paint(Canvas canvas, {ImageSource? images}) {
    final all = Offset.zero & size;
    if (fill != null) DrawingPainter(fill!.$2, images: images).fill(canvas, Path()..addRect(all), all, fill!.$1);
    for (final c in cells.values) {
      final f = c.fill;
      if (f != null) DrawingPainter(f.$2, images: images).fill(canvas, Path()..addRect(c.rect), c.rect, f.$1);
    }
    for (final c in cells.values) {
      final r = c.rect;
      for (final MapEntry(key: edge, value: (line, colors)) in c.lines.entries) {
        final (from, to) = switch (edge) {
          'l' => (r.topLeft, r.bottomLeft),
          'r' => (r.topRight, r.bottomRight),
          't' => (r.topLeft, r.topRight),
          'b' => (r.bottomLeft, r.bottomRight),
          'tl2br' => (r.topLeft, r.bottomRight),
          _ => (r.bottomLeft, r.topRight),
        };
        DrawingPainter(colors).line(canvas, Path()
          ..moveTo(from.dx, from.dy)
          ..lineTo(to.dx, to.dy), line);
      }
    }
    for (final c in cells.values) {
      canvas.save();
      canvas.translate(c.rect.left, c.rect.top);
      c.text.paint(canvas);
      canvas.restore();
    }
  }

  static List<Node> _rows(Tree tree, Node table) => [for (final n in tree.children(table.id)) if (n.type == 'tr') n];

  /// The text of a cell laid out in [size]; to [measure] the height it
  /// needs, vertical text is laid out on one line.
  static TextFrame _layout(Node cell, Props Function(int) levels, Fonts fonts, ColorContext colors, Size size, {bool measure = false}) {
    final mar = _map(cell.attributes['mar']);
    final vertical = _vertical(cell);
    return TextFrame.layout(
      cell.text ?? Delta([Op.insert('\n')]),
      box: Offset.zero & size,
      body: {
        'lIns': '${mar['l'] ?? 91440}',
        'rIns': '${mar['r'] ?? 91440}',
        'tIns': '${mar['t'] ?? 45720}',
        'bIns': '${mar['b'] ?? 45720}',
        'anchor': _anchor(cell),
        if (vertical && !measure) 'vert': cell.attributes['vert']! as String,
        if (vertical && measure) 'wrap': 'none',
      },
      levels: levels,
      colors: colors,
      fonts: fonts,
    );
  }

  static bool _vertical(Node cell) => TextFrame.turns(cell.attributes['vert'] is String ? cell.attributes['vert']! as String : null) != 0;

  static String _anchor(Node cell) => cell.attributes['anchor'] is String ? cell.attributes['anchor']! as String : 't';

  /// The height a cell needs for its text and margins: vertical text, its
  /// longest line.
  static double _needed(TextFrame text, Node cell) {
    final mar = _map(cell.attributes['mar']);
    return (_vertical(cell) ? text.width : text.height) + (_number(mar['t'] ?? 45720) + _number(mar['b'] ?? 45720)) / emuPerPoint;
  }
}

/// A cell of a table as it is drawn.
class CellLayout {
  CellLayout._(this.node, this.row, this.col, this.rowSpan, this.colSpan, this.rect, this.text, this.level, this.fill, this.lines);

  final Node node;
  final int row;
  final int col;
  final int rowSpan;
  final int colSpan;

  /// Where the cell is, in points from the corner of the table, its spans
  /// included.
  final Rect rect;

  /// The text laid out in the cell, from its corner.
  final TextFrame text;

  /// The paragraph and run properties a paragraph of a level starts from,
  /// the table style's included.
  final Props Function(int level) level;

  final Styled? fill;

  /// The lines of its edges, "l" "r" "t" "b", and across, "tl2br" "tr2bl".
  final Map<String, Styled> lines;
}

class _Placed {
  _Placed(this.node, this.row, this.col, this.rowSpan, this.colSpan);

  final Node node;
  final int row;
  final int col;
  int rowSpan;
  int colSpan;
}

/// A table style applied to a table: which of its parts a cell takes, in
/// the order PowerPoint lays them over one another.
class _Style {
  _Style(this.parts, this.options, this.rowCount, this.colCount, this.colors, this.theme);

  final Map<String, Map<String, Object?>> parts;
  final Map<String, Object?> options;
  final int rowCount;
  final int colCount;
  final ColorContext colors;
  final DeckTheme theme;

  bool _on(String option) => options[option] == true;

  /// The parts a cell takes, with the rows and columns each covers: top,
  /// left, bottom, right.
  List<(Map<String, Object?>, (int, int, int, int))> _of(_Placed p) {
    final lastRow = rowCount - 1, lastCol = colCount - 1;
    final bottom = p.row + p.rowSpan - 1, right = p.col + p.colSpan - 1;
    final out = <(Map<String, Object?>, (int, int, int, int))>[];
    void add(String name, (int, int, int, int) region) {
      final part = parts[name];
      if (part != null) out.add((part, region));
    }

    add('wholeTbl', (0, 0, lastRow, lastCol));
    final header = _on('firstRow') && p.row == 0, total = _on('lastRow') && bottom == lastRow;
    final first = _on('firstCol') && p.col == 0, last = _on('lastCol') && right == lastCol;
    if (_on('bandCol') && !first && !last) {
      final band = (p.col - (_on('firstCol') ? 1 : 0)).isEven ? 'band1V' : 'band2V';
      add(band, (0, p.col, lastRow, p.col));
    }
    if (_on('bandRow') && !header && !total) {
      final band = (p.row - (_on('firstRow') ? 1 : 0)).isEven ? 'band1H' : 'band2H';
      add(band, (p.row, 0, p.row, lastCol));
    }
    if (last) add('lastCol', (0, lastCol, lastRow, lastCol));
    if (first) add('firstCol', (0, 0, lastRow, 0));
    if (total) add('lastRow', (lastRow, 0, lastRow, lastCol));
    if (header) add('firstRow', (0, 0, 0, lastCol));
    if (total && last) add('seCell', (lastRow, lastCol, lastRow, lastCol));
    if (total && first) add('swCell', (lastRow, 0, lastRow, 0));
    if (header && last) add('neCell', (0, lastCol, 0, lastCol));
    if (header && first) add('nwCell', (0, 0, 0, 0));
    return out;
  }

  /// The run properties the style gives the text of a cell.
  Props text(_Placed p) {
    final out = <String, String>{};
    for (final (part, _) in _of(p)) {
      for (final k in ['b', 'i']) {
        if (part[k] == 'on' || part[k] == 'off') out[k] = part[k] == 'on' ? '1' : '0';
      }
      if (part['font'] is String) out['font'] = part['font']! as String;
      if (part['color'] is Map<String, Object?>) out['fill'] = jsonEncode({'solid': part['color']});
    }
    return out;
  }

  Styled? fill(_Placed p) {
    Styled? out;
    for (final (part, _) in _of(p)) {
      final fill = part['fill'], ref = part['fillRef'];
      if (fill is Map<String, Object?>) {
        out = (fill, colors);
      } else if (ref is Map<String, Object?>) {
        out = _ref(ref, theme.fills) ?? out;
      }
    }
    final own = p.node.attributes['fill'];
    return own is Map<String, Object?> ? (own, colors) : out;
  }

  Map<String, Styled> lines(_Placed p) {
    final out = <String, Styled>{};
    final bottom = p.row + p.rowSpan - 1, right = p.col + p.colSpan - 1;
    for (final (part, (top, left, lastRow, lastCol)) in _of(p)) {
      final borders = part['borders'];
      if (borders is! Map<String, Object?>) continue;
      for (final (edge, key) in [
        ('l', p.col == left ? 'left' : 'insideV'),
        ('r', right == lastCol ? 'right' : 'insideV'),
        ('t', p.row == top ? 'top' : 'insideH'),
        ('b', bottom == lastRow ? 'bottom' : 'insideH'),
        ('tl2br', 'tl2br'),
        ('tr2bl', 'tr2bl'),
      ]) {
        final b = borders[key];
        if (b is! Map<String, Object?>) continue;
        final line = b['ln'], ref = b['ref'];
        final styled = line is Map<String, Object?> ? (line, colors) : (ref is Map<String, Object?> ? _ref(ref, theme.lines) : null);
        if (styled != null) out[edge] = styled;
      }
    }
    for (final (edge, key) in [('l', 'lnL'), ('r', 'lnR'), ('t', 'lnT'), ('b', 'lnB'), ('tl2br', 'lnTlToBr'), ('tr2bl', 'lnBlToTr')]) {
      final own = p.node.attributes[key];
      if (own is Map<String, Object?>) out[edge] = ({...?out[edge]?.$1, ...own}, out[edge]?.$2 ?? colors);
    }
    return out;
  }

  /// The theme's fill or line a reference points to, in its color.
  Styled? _ref(Map<String, Object?> ref, List<Object?> list) {
    final idx = int.tryParse('${ref['idx']}') ?? 0;
    if (idx <= 0 || idx > list.length || list[idx - 1] is! Map<String, Object?>) return null;
    return (list[idx - 1]! as Map<String, Object?>, colors.withPlaceholder(colors.resolve(ref['color'])));
  }
}

Map<String, Object?> _map(Object? v) => v is Map<String, Object?> ? v : const {};

int _count(Object? v) => v is num && v > 1 ? v.toInt() : 1;

double _number(Object? v) => v is num ? v.toDouble() : 0;
