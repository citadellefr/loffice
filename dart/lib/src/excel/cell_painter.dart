import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

import '../pdf/canvas.dart';
import 'conditional.dart';
import 'number_format.dart';
import 'workbook.dart';

const _gridColor = Color(0xFFD9D9D9);

/// Text laid out once for as long as it is drawn the same.
class TextCache {
  final _cache = <(String, TextStyle, double, bool), TextPainter>{};

  TextPainter get(String text, TextStyle style, double width, bool wrap) {
    final key = (text, style, wrap ? width : 0.0, wrap);
    final hit = _cache[key];
    if (hit != null) return hit;
    if (_cache.length > 4000) _cache.clear();
    final p = TextPainter(text: TextSpan(text: text, style: style), textDirection: TextDirection.ltr, maxLines: wrap ? null : 1)
      ..layout(maxWidth: wrap ? math.max(width, 1) : double.infinity);
    return _cache[key] = p;
  }
}

/// Paints the cells of a sheet as Excel draws them: fills, gridlines, data
/// bars and icons, texts and borders. [x] and [y] tell where a column and
/// a row start on the canvas, the sheet shown at [zoom].
class CellPainter {
  CellPainter({
    required this.book,
    required this.sheet,
    required this.locale,
    required this.x,
    required this.y,
    this.zoom = 1,
    this.fonts,
    this.gridlines = true,
    this.gridColor = _gridColor,
    double Function(int col)? width,
    double Function(int row)? height,
    TextCache? texts,
  }) : layout = book.layout(sheet),
       texts = texts ?? TextCache() {
    this.width = width ?? layout.width;
    this.height = height ?? layout.height;
  }

  final Workbook book;
  final Node sheet;
  final SheetLayout layout;
  final NumberLocale locale;
  final double Function(int col) x;
  final double Function(int row) y;
  final double zoom;
  final String? fonts;
  final bool gridlines;
  final Color gridColor;
  final TextCache texts;

  /// The size of a column and of a row, in pixels at 100 %.
  late final double Function(int col) width;
  late final double Function(int row) height;

  late final looks = book.looks(sheet);

  /// The format of a cell, with what its conditional formats change.
  CellStyle _style(Cell c) {
    final id = c.fields['s'] as String?;
    return looks.style(book.style(id), id, c.row, c.col);
  }

  Rect _rect(int r1, int c1, int r2, int c2) => Rect.fromLTRB(
    x(c1),
    y(r1),
    x(c2) + width(c2) * zoom,
    y(r2) + height(r2) * zoom,
  );

  /// Paints the cells of [area], those merged from before it included.
  void paint(Canvas canvas, CellArea area) {
    final grid = sheet.grid ?? const Grid.empty();
    final l = layout;
    // cells whose merged area starts before the pane still show
    final merges = [for (final m in l.merges) if (m.intersects(area)) m];
    final covered = <(int, int)>{};
    for (final m in merges) {
      for (var r = m.top; r <= m.bottom; r++) {
        for (var c = m.left; c <= m.right; c++) {
          if (r != m.top || c != m.left) covered.add((r, c));
        }
      }
    }
    // the style of whole rows and columns
    final rowStyles = <int, String>{};
    final colStyles = <int, String>{};
    for (final c in grid.rows(0, 0)) {
      if (c.fields['s'] is String) colStyles[c.col] = c.fields['s']! as String;
    }

    // fills
    final cells = <Cell>[];
    for (final c in grid.rows(area.top, area.bottom)) {
      if (c.col == 0) {
        if (c.fields['s'] is String) rowStyles[c.row] = c.fields['s']! as String;
        continue;
      }
      if (c.col < area.left || c.col > area.right) continue;
      cells.add(c);
    }
    for (final m in merges) {
      if (!area.contains(m.top, m.left)) {
        final f = grid.cell(m.top, m.left);
        if (f != null) cells.add(Cell(m.top, m.left, f));
      }
    }
    // empty cells a conditional format fills
    if (!looks.isEmpty) {
      final held = {for (final c in cells) (c.row, c.col)};
      for (final c in looks.grid.rows(area.top, area.bottom)) {
        if (c.col >= area.left && c.col <= area.right && !held.contains((c.row, c.col))) cells.add(Cell(c.row, c.col, const {}));
      }
    }
    for (final e in colStyles.entries) {
      if (e.key < area.left || e.key > area.right) continue;
      final fill = book.style(e.value).fill;
      if (fill != null) canvas.drawRect(_rect(area.top, e.key, area.bottom, e.key), Paint()..color = fill);
    }
    for (final e in rowStyles.entries) {
      final fill = book.style(e.value).fill;
      if (fill != null) canvas.drawRect(_rect(e.key, area.left, e.key, area.right), Paint()..color = fill);
    }
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final fill = _style(c).fill;
      if (fill == null) continue;
      final m = l.mergeAt(c.row, c.col);
      canvas.drawRect(m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right), Paint()..color = fill);
    }

    // gridlines, which merged cells and fills hide
    if (gridlines) {
      final p = Paint()
        ..color = gridColor
        ..strokeWidth = 1;
      final y1 = y(area.top), y2 = y(area.bottom) + height(area.bottom) * zoom;
      final x1 = x(area.left), x2 = x(area.right) + width(area.right) * zoom;
      for (var c = area.left; c <= area.right + 1; c++) {
        final at = x(c) - 0.5;
        canvas.drawLine(Offset(at, y1), Offset(at, y2), p);
      }
      for (var r = area.top; r <= area.bottom + 1; r++) {
        final at = y(r) - 0.5;
        canvas.drawLine(Offset(x1, at), Offset(x2, at), p);
      }
      final white = Paint()..color = Colors.white;
      for (final m in merges) {
        final f = grid.cell(m.top, m.left);
        if (book.style(f?['s'] as String?).fill == null) canvas.drawRect(_rect(m.top, m.left, m.bottom, m.right).deflate(0.5), white);
      }
    }

    // data bars and icons
    final icons = <(int, int)>{};
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final bar = looks.bar(c.row, c.col);
      final icon = looks.icon(c.row, c.col);
      if (bar == null && icon == null) continue;
      final r = _rect(c.row, c.col, c.row, c.col);
      if (r.height < 6 || r.width < 6) continue;
      if (bar != null) {
        final b = Rect.fromLTWH(r.left + 1, r.top + 2, (r.width - 2) * bar.$1 / 100, r.height - 4);
        final fade = GradientSpec.linear(b.centerLeft, b.centerRight, [bar.$2, Color.lerp(bar.$2, Colors.white, 0.85)!], const [0, 1]);
        canvas.drawRect(b, Paint()..shader = gradient(fade));
      }
      if (icon != null) {
        final side = math.min(r.height - 4, 14 * zoom);
        paintIcon(canvas, Rect.fromLTWH(r.left + 2 * zoom, r.bottom - side - 2, side, side), icon.$1, icon.$2);
        icons.add((c.row, c.col));
      }
    }

    // texts
    final occupied = <(int, int)>{for (final c in cells) if (_shows(c.fields)) (c.row, c.col)};
    for (final c in cells) {
      if (covered.contains((c.row, c.col)) || !_shows(c.fields) || looks.hidesValue(c.row, c.col)) continue;
      _text(canvas, c, occupied, area, icon: icons.contains((c.row, c.col)));
    }

    // borders
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final s = _style(c);
      if (s.left == null && s.right == null && s.top == null && s.bottom == null) continue;
      final m = l.mergeAt(c.row, c.col);
      final r = m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right);
      _edge(canvas, s.top, r.topLeft, r.topRight);
      _edge(canvas, s.bottom, r.bottomLeft, r.bottomRight);
      _edge(canvas, s.left, r.topLeft, r.bottomLeft);
      _edge(canvas, s.right, r.topRight, r.bottomRight);
    }
  }

  static bool _shows(Map<String, Object?> f) => f['v'] != null || f['e'] != null;

  void _edge(Canvas canvas, Edge? e, Offset a, Offset b) {
    if (e == null) return;
    final p = Paint()
      ..color = e.color
      ..strokeWidth = e.width * math.min(zoom, 1.5);
    if (e.style == 'double') {
      final n = a.dx == b.dx ? const Offset(1.5, 0) : const Offset(0, 1.5);
      p.strokeWidth = 1;
      canvas.drawLine(a - n, b - n, p);
      canvas.drawLine(a + n, b + n, p);
      return;
    }
    if (e.style.contains('ash') || e.style.contains('ot') || e.style == 'hair') {
      final dash = e.style == 'dotted' || e.style == 'hair' ? 1.5 : 4.0;
      final len = (b - a).distance;
      final dir = (b - a) / len;
      for (var t = 0.0; t < len; t += dash * 2) {
        canvas.drawLine(a + dir * t, a + dir * math.min(t + dash, len), p);
      }
      return;
    }
    canvas.drawLine(a, b, p);
  }

  /// Paints the text of a cell, past the icon a conditional format gives
  /// it if any.
  void _text(Canvas canvas, Cell c, Set<(int, int)> occupied, CellArea area, {bool icon = false}) {
    final l = layout;
    final style = _style(c);
    final shown = cellText(c.fields, style, locale, date1904: book.date1904);
    if (shown.text.isEmpty) return;
    final m = l.mergeAt(c.row, c.col);
    final rect = m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right);
    final f = style.font;
    final ts = TextStyle(
      fontFamily: substituteFont(f.name),
      fontFamilyFallback: fallbackFonts,
      package: fonts,
      fontSize: f.size * 4 / 3 * zoom,
      fontWeight: f.bold ? FontWeight.bold : FontWeight.normal,
      fontStyle: f.italic ? FontStyle.italic : FontStyle.normal,
      decoration: TextDecoration.combine([
        if (f.underline) TextDecoration.underline,
        if (f.strike) TextDecoration.lineThrough,
      ]),
      color: shown.color ?? f.color ?? Colors.black,
      height: 1.2,
    );
    final pad = 2 * zoom + style.indent * 9 * zoom;
    var text = shown.text;
    var painter = texts.get(text, ts, rect.width - 2 * pad, style.wrap);
    // numbers that do not fit show ###
    if (shown.number && !style.wrap && painter.width > rect.width - 2 * pad && m == null) {
      final hash = texts.get('#', ts, 0, false).width;
      text = '#' * math.max(1, ((rect.width - 2 * pad) / math.max(hash, 1)).floor());
      painter = texts.get(text, ts, 0, false);
    }
    // text runs over the empty cells next to it
    var clip = rect;
    if (!style.wrap && m == null && !shown.number && painter.width > rect.width - 2 * pad) {
      if (shown.align == 'left' || shown.align == 'center') {
        var col = c.col + 1;
        while (col <= area.right + 1 && !occupied.contains((c.row, col)) && clip.right < rect.left + painter.width + 2 * pad) {
          clip = Rect.fromLTRB(clip.left, clip.top, x(col) + width(col) * zoom, clip.bottom);
          col++;
        }
      }
      if (shown.align == 'right' || shown.align == 'center') {
        var col = c.col - 1;
        while (col >= 1 && !occupied.contains((c.row, col)) && clip.left > rect.right - painter.width - 2 * pad) {
          clip = Rect.fromLTRB(x(col), clip.top, clip.right, clip.bottom);
          col--;
        }
      }
    }
    final dx = switch (shown.align) {
      'right' => rect.right - pad - painter.width,
      'center' => rect.center.dx - painter.width / 2,
      _ => rect.left + pad + (icon ? 16 * zoom : 0),
    };
    final dy = switch (style.vertical) {
      'top' => rect.top + 1,
      'center' || 'justify' || 'distributed' => rect.center.dy - painter.height / 2,
      _ => rect.bottom - painter.height - 1,
    };
    canvas.save();
    canvas.clipRect(clip.deflate(0.5));
    paintText(canvas, painter, Offset(dx, dy));
    canvas.restore();
  }
}
