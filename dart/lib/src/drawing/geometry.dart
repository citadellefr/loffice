import 'dart:convert';
import 'dart:math' as math;
import 'dart:ui';

import 'presets.dart';

/// How a path of a geometry is filled.
enum PathFill { none, norm, lighten, lightenLess, darken, darkenLess }

/// A path of a shape, laid out for its size.
class ShapePath {
  const ShapePath(this.path, this.fill, this.stroke);

  final Path path;
  final PathFill fill;
  final bool stroke;
}

/// A shape outline in the terms of DrawingML: adjust values, guides
/// computed from them and the shape's size, a text rectangle, and paths.
/// The preset shapes and the custom geometries of documents are both one.
class Geometry {
  Geometry._(this._av, this._gd, this._rect, this._paths);

  /// A geometry as the Go package writes it: the "cust" of a geometry, or
  /// a preset definition.
  static Geometry? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final paths = <_PathDef>[];
    for (final raw in json['paths'] is List<Object?> ? json['paths']! as List<Object?> : const []) {
      if (raw is! Map<String, Object?>) return null;
      final commands = <List<String>>[];
      for (final c in raw['d'] is List<Object?> ? raw['d']! as List<Object?> : const []) {
        if (c is! List<Object?> || c.isEmpty || c.any((x) => x is! String)) return null;
        commands.add(c.cast<String>());
      }
      paths.add(_PathDef(_num(raw['w']), _num(raw['h']), PathFill.values.asNameMap()[raw['fill']] ?? PathFill.norm, raw['stroke'] != false, commands));
    }
    final rect = json['rect'];
    return Geometry._(_guides(json['av']), _guides(json['gd']), rect is List<Object?> && rect.length == 4 && rect.every((x) => x is String) ? rect.cast<String>() : null, paths);
  }

  static final _presets = <String, Geometry?>{};

  /// The preset shape of that name, null for an unknown one.
  static Geometry? preset(String name) => _presets.putIfAbsent(name, () {
    final def = presetGeometries[name];
    return def == null ? null : Geometry.fromJson(jsonDecode(def));
  });

  final List<(String, String)> _av;
  final List<(String, String)> _gd;
  final List<String>? _rect;
  final List<_PathDef> _paths;

  /// The paths of the shape at that size, its adjust values overridden by
  /// [adjust] ("adj" → "val 25000").
  List<ShapePath> paths(Size size, [Map<String, String> adjust = const {}]) {
    final g = _Guides(size, _av, _gd, adjust);
    return [for (final p in _paths) ShapePath(_build(p, g, size), p.fill, p.stroke)];
  }

  /// Where the text of the shape goes, at that size.
  Rect textRect(Size size, [Map<String, String> adjust = const {}]) {
    final r = _rect;
    if (r == null) return Offset.zero & size;
    final g = _Guides(size, _av, _gd, adjust);
    return Rect.fromLTRB(g.value(r[0]), g.value(r[1]), g.value(r[2]), g.value(r[3]));
  }

  static Path _build(_PathDef p, _Guides g, Size size) {
    final sx = p.w > 0 ? size.width / p.w : 1.0;
    final sy = p.h > 0 ? size.height / p.h : 1.0;
    final path = Path();
    var at = Offset.zero;
    Offset point(String x, String y) => Offset(g.value(x) * sx, g.value(y) * sy);
    for (final c in p.commands) {
      switch (c) {
        case ['M', final x, final y]:
          at = point(x, y);
          path.moveTo(at.dx, at.dy);
        case ['L', final x, final y]:
          at = point(x, y);
          path.lineTo(at.dx, at.dy);
        case ['Q', final x1, final y1, final x, final y]:
          final c1 = point(x1, y1);
          at = point(x, y);
          path.quadraticBezierTo(c1.dx, c1.dy, at.dx, at.dy);
        case ['C', final x1, final y1, final x2, final y2, final x, final y]:
          final c1 = point(x1, y1), c2 = point(x2, y2);
          at = point(x, y);
          path.cubicTo(c1.dx, c1.dy, c2.dx, c2.dy, at.dx, at.dy);
        case ['A', final wR, final hR, final stAng, final swAng]:
          at = _arc(path, at, g.value(wR) * sx, g.value(hR) * sy, g.value(stAng), g.value(swAng));
        case ['Z']:
          path.close();
      }
    }
    return path;
  }

  /// Draws an arc of the ellipse of radii [rx] and [ry] from [at], which is
  /// on it at the angle [start], sweeping [sweep]; angles are in 60000ths of
  /// a degree, as seen on the ellipse. Returns where the arc ends.
  static Offset _arc(Path path, Offset at, double rx, double ry, double start, double sweep) {
    if (rx <= 0 || ry <= 0) return at;
    final t0 = _parametric(start * _radian, rx, ry);
    final t1 = _parametric((start + sweep) * _radian, rx, ry);
    final center = at - Offset(rx * math.cos(t0), ry * math.sin(t0));
    final oval = Rect.fromCenter(center: center, width: 2 * rx, height: 2 * ry);
    var remaining = t1 - t0;
    var from = t0;
    // Path.arcTo draws at most a full turn per call
    while (remaining.abs() > 2 * math.pi) {
      final step = remaining.sign * 2 * math.pi;
      path.arcTo(oval, from, step, false);
      from += step;
      remaining -= step;
    }
    path.arcTo(oval, from, remaining, false);
    return center + Offset(rx * math.cos(t1), ry * math.sin(t1));
  }

  /// The parametric angle of the point of an ellipse seen at [angle] from
  /// its center, continuous in [angle].
  static double _parametric(double angle, double rx, double ry) {
    final seen = math.atan2(math.sin(angle), math.cos(angle));
    var t = math.atan2(rx * math.sin(angle), ry * math.cos(angle)) - seen;
    if (t > math.pi) t -= 2 * math.pi;
    if (t < -math.pi) t += 2 * math.pi;
    return angle + t;
  }
}

const _radian = math.pi / 180 / 60000;

class _PathDef {
  const _PathDef(this.w, this.h, this.fill, this.stroke, this.commands);

  final double w;
  final double h;
  final PathFill fill;
  final bool stroke;
  final List<List<String>> commands;
}

double _num(Object? v) => v is num ? v.toDouble() : 0;

List<(String, String)> _guides(Object? json) => [
  if (json is List<Object?>)
    for (final g in json)
      if (g is List<Object?> && g.length == 2 && g[0] is String && g[1] is String) (g[0]! as String, g[1]! as String),
];

/// The values of the guides of a geometry at a size: built-in ones, then
/// the adjust values, then the guides in order.
class _Guides {
  _Guides(Size size, List<(String, String)> av, List<(String, String)> gd, Map<String, String> adjust) {
    final w = size.width, h = size.height;
    final ss = math.min(w, h), ls = math.max(w, h);
    _values.addAll({
      'w': w,
      'h': h,
      'l': 0,
      't': 0,
      'r': w,
      'b': h,
      'hc': w / 2,
      'vc': h / 2,
      'ss': ss,
      'ls': ls,
      'cd2': 10800000,
      'cd4': 5400000,
      'cd8': 2700000,
      '3cd4': 16200000,
      '3cd8': 8100000,
      '5cd8': 13500000,
      '7cd8': 18900000,
      for (final d in [2, 3, 4, 5, 6, 8, 10, 12, 16, 32]) ...{'wd$d': w / d, 'hd$d': h / d, 'ssd$d': ss / d},
    });
    for (final (name, formula) in av) {
      _values[name] = _evaluate(adjust[name] ?? formula);
    }
    for (final (name, formula) in gd) {
      _values[name] = _evaluate(formula);
    }
  }

  final _values = <String, double>{};

  double value(String s) => _values[s] ?? double.tryParse(s) ?? 0;

  double _evaluate(String formula) {
    final parts = formula.split(' ').where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return 0;
    double arg(int i) => i < parts.length ? value(parts[i]) : 0;
    final x = arg(1), y = arg(2), z = arg(3);
    double angle(double v) => v * _radian;
    return switch (parts[0]) {
      '*/' => z == 0 ? 0 : x * y / z,
      '+-' => x + y - z,
      '+/' => z == 0 ? 0 : (x + y) / z,
      '?:' => x > 0 ? y : z,
      'abs' => x.abs(),
      'at2' => math.atan2(y, x) / _radian,
      'cat2' => x * math.cos(math.atan2(z, y)),
      'cos' => x * math.cos(angle(y)),
      'max' => math.max(x, y),
      'min' => math.min(x, y),
      'mod' => math.sqrt(x * x + y * y + z * z),
      'pin' => y < x ? x : (y > z ? z : y),
      'sat2' => x * math.sin(math.atan2(z, y)),
      'sin' => x * math.sin(angle(y)),
      'sqrt' => math.sqrt(math.max(x, 0)),
      'tan' => x * math.tan(angle(y)),
      'val' => x,
      _ => 0,
    };
  }
}
