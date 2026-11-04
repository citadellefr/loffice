import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import 'workbook.dart';

/// What the conditional formats of a sheet give its cells, as the server
/// calculates it into the grid of the sheet's "looks" node: the rules
/// whose styles a cell takes, where it lies on a color scale, the length
/// of its data bar and its icon.
class SheetLooks {
  SheetLooks(this.book, this.sheet, Node? looks) : grid = looks?.grid ?? const Grid.empty() {
    final all = sheet.attributes['cf'];
    if (all is List<Object?>) _rules = [for (final r in all) r is Map<String, Object?> ? r : const {}];
  }

  final Workbook book;
  final Node sheet;
  final Grid grid;
  var _rules = const <Map<String, Object?>>[];
  final _styles = <String, CellStyle>{};

  bool get isEmpty => grid.length == 0;

  Map<String, Object?> _rule(Object? i) => i is int && i >= 0 && i < _rules.length ? _rules[i] : const {};

  /// The format of a cell once its conditional formats change it: the
  /// styles of its rules, the first first, and the color of its scale.
  CellStyle style(CellStyle base, String? id, int row, int col) {
    final look = grid.cell(row, col);
    if (look == null) return base;
    final key = '$id ${look['r']} ${look['s']}';
    return _styles[key] ??= _style(base, look);
  }

  CellStyle _style(CellStyle base, Map<String, Object?> look) {
    final json = {...base.json};
    int? filledBy;
    final rules = look['r'] is List<Object?> ? look['r']! as List<Object?> : const <Object?>[];
    for (final i in rules.reversed) {
      final s = _rule(i)['style'];
      if (s is! Map<String, Object?>) continue;
      if (s['font'] is Map<String, Object?>) {
        json['font'] = {...?json['font'] as Map<String, Object?>?, ...s['font']! as Map<String, Object?>};
      }
      if (s['fill'] != null && i is int) {
        json['fill'] = s['fill'];
        filledBy = i;
      }
      if (s['border'] is Map<String, Object?>) {
        json['border'] = {...?json['border'] as Map<String, Object?>?, ...s['border']! as Map<String, Object?>};
      }
      if (s['fmt'] is String) json['fmt'] = s['fmt'];
    }
    final style = CellStyle.fromJson(json, book);
    final scale = look['s'];
    if (scale is! List<Object?> || scale.length != 2 || scale[0] is! int || scale[1] is! num) return style;
    if (filledBy != null && filledBy < (scale[0]! as int)) return style;
    final colors = [for (final c in _rule(scale[0])['colors'] as List<Object?>? ?? const []) book.color(c) ?? const Color(0xFFFFFFFF)];
    final fill = scaleColor(colors, (scale[1]! as num).toDouble());
    return fill == null ? style : style.copyWith(fill: fill);
  }

  /// The data bar of a cell: its length in percent of the cell, and its
  /// color.
  (double, Color)? bar(int row, int col) {
    final b = grid.cell(row, col)?['b'];
    if (b is! List<Object?> || b.length != 2 || b[1] is! num) return null;
    final colors = _rule(b[0])['colors'];
    final color = colors is List<Object?> && colors.isNotEmpty ? book.color(colors.first) : null;
    return ((b[1]! as num).toDouble(), color ?? const Color(0xFF638EC6));
  }

  /// The icon of a cell: its set and its place in the set, the lowest 0.
  (String, int)? icon(int row, int col) {
    final i = grid.cell(row, col)?['i'];
    if (i is! List<Object?> || i.length != 2 || i[1] is! int) return null;
    return ('${_rule(i[0])['icons'] ?? '3TrafficLights1'}', i[1]! as int);
  }

  /// Whether a bar or icon shows in place of the cell's value.
  bool hidesValue(int row, int col) {
    final look = grid.cell(row, col);
    if (look == null) return false;
    for (final k in ['b', 'i']) {
      final v = look[k];
      if (v is List<Object?> && v.isNotEmpty && _rule(v[0])['noValue'] == true) return true;
    }
    return false;
  }
}

/// The color at [t] of a scale of two or three colors, the middle one at
/// 0.5.
Color? scaleColor(List<Color> colors, double t) {
  if (colors.length == 2) return Color.lerp(colors[0], colors[1], t);
  if (colors.length == 3) return t <= 0.5 ? Color.lerp(colors[0], colors[1], t * 2) : Color.lerp(colors[1], colors[2], (t - 0.5) * 2);
  return null;
}

const _red = Color(0xFFE0443A);
const _yellow = Color(0xFFF2C12E);
const _green = Color(0xFF3E9E4A);
const _gray = Color(0xFF8C8C8C);
const _black = Color(0xFF3A3A3A);
const _blue = Color(0xFF4472C4);
const _gold = Color(0xFFF0B400);

/// The color of the icon [i] of a set of [n]: red, yellow and green from
/// the lowest, black before red in the sets of traffic lights.
Color _levelColor(String set, int n, int i) {
  if (set.contains('Gray')) return _gray;
  if (set.contains('RedToBlack')) return Color.lerp(_black, _red, i / (n - 1))!;
  return switch (n) {
    4 when set.contains('Arrows') => [_red, _yellow, _yellow, _green][i],
    4 => [_black, _red, _yellow, _green][i],
    5 => [_red, _yellow, _yellow, _yellow, _green][i],
    _ => [_red, _yellow, _green][i],
  };
}

/// The direction of the arrow [i] of a set of [n], in degrees from the
/// right, clockwise.
double _arrowAngle(int n, int i) => switch (n) {
  4 => [90.0, 45.0, -45.0, -90.0][i],
  5 => [90.0, 45.0, 0.0, -45.0, -90.0][i],
  _ => [90.0, 0.0, -90.0][i],
};

Path _star(Offset c, double radius) {
  final p = Path();
  for (var k = 0; k < 10; k++) {
    final a = -math.pi / 2 + k * math.pi / 5;
    final d = k.isEven ? radius : radius * 0.42;
    final at = c + Offset(math.cos(a), math.sin(a)) * d;
    k == 0 ? p.moveTo(at.dx, at.dy) : p.lineTo(at.dx, at.dy);
  }
  return p..close();
}

/// Paints the icon [index] of a set of Excel's in [r].
void paintIcon(Canvas canvas, Rect r, String set, int index) {
  final n = int.tryParse(set.substring(0, 1)) ?? 3;
  final i = index.clamp(0, n - 1);
  final level = i / (n - 1);
  final color = _levelColor(set, n, i);
  final fill = Paint()
    ..isAntiAlias = true
    ..color = color;
  final c = r.center;
  final radius = r.shortestSide / 2;
  if (set.contains('Arrows')) {
    final s = radius;
    canvas.save();
    canvas.translate(c.dx, c.dy);
    canvas.rotate(_arrowAngle(n, i) * math.pi / 180);
    canvas.drawPath(
      Path()
        ..moveTo(s, 0)
        ..lineTo(0, -s)
        ..lineTo(0, -s * 0.4)
        ..lineTo(-s, -s * 0.4)
        ..lineTo(-s, s * 0.4)
        ..lineTo(0, s * 0.4)
        ..lineTo(0, s)
        ..close(),
      fill,
    );
    canvas.restore();
  } else if (set.contains('Triangles')) {
    final s = radius * 0.8;
    if (i == 1) {
      canvas.drawRect(Rect.fromCenter(center: c, width: s * 2, height: s * 0.5), fill);
    } else {
      final up = i == 2;
      canvas.drawPath(
        Path()
          ..moveTo(c.dx - s, c.dy + (up ? s * 0.6 : -s * 0.6))
          ..lineTo(c.dx + s, c.dy + (up ? s * 0.6 : -s * 0.6))
          ..lineTo(c.dx, c.dy + (up ? -s : s))
          ..close(),
        fill,
      );
    }
  } else if (set.contains('Flags')) {
    final s = radius;
    canvas.drawRect(Rect.fromLTWH(c.dx - s * 0.6, c.dy - s, s * 0.15, s * 2), Paint()..color = _black);
    canvas.drawRect(Rect.fromLTWH(c.dx - s * 0.45, c.dy - s, s * 1.2, s * 0.9), fill);
  } else if (set.contains('Stars')) {
    fill.color = _gold;
    final star = _star(c, radius);
    // empty, half and full
    canvas.save();
    canvas.clipRect(Rect.fromLTRB(r.left, r.top, i == 1 ? c.dx : r.right, r.bottom));
    if (i > 0) canvas.drawPath(star, fill);
    canvas.restore();
    canvas.drawPath(
      star,
      Paint()
        ..color = _gold
        ..style = PaintingStyle.stroke,
    );
  } else if (set.contains('Quarters')) {
    final outline = Paint()
      ..color = _black
      ..style = PaintingStyle.stroke;
    fill.color = _black;
    final circle = Rect.fromCircle(center: c, radius: radius * 0.9);
    canvas.drawOval(circle, outline);
    canvas.drawArc(circle, -math.pi / 2, 2 * math.pi * level, true, fill);
  } else if (set.contains('Rating')) {
    fill.color = _blue;
    final outline = Paint()
      ..color = _blue
      ..style = PaintingStyle.stroke;
    final bars = n - 1;
    final w = r.width / (bars * 1.5);
    for (var k = 0; k < bars; k++) {
      final h = r.height * (k + 1) / bars;
      canvas.drawRect(Rect.fromLTWH(r.left + k * w * 1.5, r.bottom - h, w, h), k < i ? fill : outline);
    }
  } else if (set.contains('Boxes')) {
    fill.color = _blue;
    final box = Rect.fromCenter(center: c, width: radius * 1.8, height: radius * 1.8);
    // the quarters of the box fill one after the other, clockwise
    for (var k = 0; k < i; k++) {
      canvas.drawRect(Rect.fromLTWH(k == 0 || k == 3 ? box.left : c.dx, k < 2 ? box.top : c.dy, box.width / 2, box.height / 2), fill);
    }
    canvas.drawRect(
      box,
      Paint()
        ..color = _blue
        ..style = PaintingStyle.stroke,
    );
  } else if (set.contains('Signs')) {
    final s = radius * 0.9;
    final shape = switch (i) {
      0 => (Path()..addPolygon([c + Offset(0, -s), c + Offset(s, 0), c + Offset(0, s), c + Offset(-s, 0)], true)),
      1 => (Path()..addPolygon([c + Offset(0, -s), c + Offset(s, s * 0.8), c + Offset(-s, s * 0.8)], true)),
      _ => (Path()..addOval(Rect.fromCircle(center: c, radius: s))),
    };
    canvas.drawPath(shape, fill);
  } else if (set.contains('Symbols')) {
    final circled = !set.endsWith('2');
    final s = radius * (circled ? 0.4 : 0.6);
    final mark = Paint()
      ..color = circled ? const Color(0xFFFFFFFF) : color
      ..strokeWidth = math.max(1.5, radius / (circled ? 4 : 3))
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;
    if (circled) canvas.drawCircle(c, radius * 0.9, fill);
    switch (i) {
      case 0:
        canvas.drawLine(c + Offset(-s, -s), c + Offset(s, s), mark);
        canvas.drawLine(c + Offset(-s, s), c + Offset(s, -s), mark);
      case 1:
        canvas.drawLine(c + Offset(0, -s * 1.2), c + Offset(0, s * 0.3), mark);
        canvas.drawCircle(c + Offset(0, s * 1.1), mark.strokeWidth / 2, mark..style = PaintingStyle.fill);
      default:
        canvas.drawPath(
          Path()
            ..moveTo(c.dx - s, c.dy)
            ..lineTo(c.dx - s * 0.2, c.dy + s * 0.8)
            ..lineTo(c.dx + s, c.dy - s * 0.8),
          mark,
        );
    }
  } else {
    // traffic lights and the circles from red to black
    if (set.endsWith('2')) {
      canvas.drawRRect(RRect.fromRectAndRadius(Rect.fromCircle(center: c, radius: radius), Radius.circular(radius * 0.4)), Paint()..color = _black);
      canvas.drawCircle(c, radius * 0.65, fill);
    } else {
      canvas.drawCircle(c, radius * 0.9, fill);
    }
  }
}
