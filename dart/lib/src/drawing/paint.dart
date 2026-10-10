import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';

import '../pdf/canvas.dart';
import 'color.dart';
import 'geometry.dart';

/// Pictures by the name the document gives them, once loaded.
typedef ImageSource = ui.Image? Function(String media);

/// Paints DrawingML fills and lines on a canvas, in points.
class DrawingPainter {
  const DrawingPainter(this.colors, {this.images});

  final ColorContext colors;
  final ImageSource? images;

  /// Fills a path with a fill as the Go package writes it; [mode] darkens or
  /// lightens it, as some paths of preset shapes are.
  void fill(Canvas canvas, Path path, Rect box, Map<String, Object?>? fill, {PathFill mode = PathFill.norm}) {
    if (fill == null || fill['none'] == true || mode == PathFill.none) return;
    final blip = fill['blip'];
    if (blip is Map<String, Object?>) {
      canvas.save();
      canvas.clipPath(path);
      picture(canvas, box, blip);
      canvas.restore();
      return;
    }
    final paint = _paint(fill, box, mode);
    if (paint != null) canvas.drawPath(path, paint);
  }

  /// The color a fill mostly shows, for what paints with one color: text,
  /// bullets.
  Color? color(Map<String, Object?>? fill) {
    if (fill == null) return null;
    if (fill['solid'] != null) return colors.resolve(fill['solid']);
    final grad = fill['grad'];
    if (grad is Map<String, Object?>) {
      final stops = _stops(grad);
      if (stops.isNotEmpty) return stops.first.$2;
    }
    final patt = fill['patt'];
    if (patt is Map<String, Object?>) return colors.resolve(patt['fg']);
    return null;
  }

  Paint? _paint(Map<String, Object?> fill, Rect box, PathFill mode) {
    Color shade(Color c) => switch (mode) {
      PathFill.darken => Color.lerp(c, const Color(0xFF000000), 0.4)!,
      PathFill.darkenLess => Color.lerp(c, const Color(0xFF000000), 0.2)!,
      PathFill.lighten => Color.lerp(c, const Color(0xFFFFFFFF), 0.4)!,
      PathFill.lightenLess => Color.lerp(c, const Color(0xFFFFFFFF), 0.2)!,
      _ => c,
    };
    if (fill['solid'] != null) {
      final c = colors.resolve(fill['solid']);
      return c == null ? null : (Paint()..color = shade(c));
    }
    final grad = fill['grad'];
    if (grad is Map<String, Object?>) {
      final stops = _stops(grad);
      if (stops.isEmpty) return null;
      if (stops.length == 1) return Paint()..color = shade(stops.single.$2);
      final positions = [for (final s in stops) s.$1];
      final list = [for (final s in stops) shade(s.$2)];
      final path = grad['path'];
      if (path is String) {
        final focus = grad['focus'];
        var center = box.center;
        if (focus is List<Object?> && focus.length == 4 && focus.every((x) => x is num)) {
          final f = focus.cast<num>();
          center = Offset(
            box.left + box.width * (f[0] / 100000 + (1 - f[0] / 100000 - f[2] / 100000) / 2),
            box.top + box.height * (f[1] / 100000 + (1 - f[1] / 100000 - f[3] / 100000) / 2),
          );
        }
        final radius = [box.topLeft, box.topRight, box.bottomLeft, box.bottomRight]
            .map((p) => (p - center).distance)
            .reduce(math.max);
        return Paint()..shader = gradient(GradientSpec.radial(center, radius, list, positions));
      }
      var angle = (grad['lin'] is num ? (grad['lin']! as num) / 60000 : 0) * math.pi / 180;
      if (grad['scaled'] == true && box.width > 0 && box.height > 0) {
        angle = math.atan2(math.sin(angle) * box.height, math.cos(angle) * box.width);
      }
      final dx = math.cos(angle), dy = math.sin(angle);
      final half = (box.width * dx.abs() + box.height * dy.abs()) / 2;
      final from = box.center - Offset(dx, dy) * half, to = box.center + Offset(dx, dy) * half;
      return Paint()..shader = gradient(GradientSpec.linear(from, to, list, positions));
    }
    final patt = fill['patt'];
    if (patt is Map<String, Object?>) {
      final fg = colors.resolve(patt['fg']) ?? const Color(0xFF000000);
      final bg = colors.resolve(patt['bg']) ?? const Color(0xFFFFFFFF);
      return Paint()..shader = flat(_pattern(patt['prst'] as String? ?? '', shade(fg), shade(bg)), Color.lerp(shade(bg), shade(fg), 0.5)!);
    }
    return null;
  }

  List<(double, Color)> _stops(Map<String, Object?> grad) => [
    if (grad['stops'] case final List<Object?> stops)
      for (final s in stops)
        if (s is List<Object?> && s.length == 2 && s[0] is num)
          if (colors.resolve(s[1]) case final c?) (((s[0]! as num) / 100000).clamp(0.0, 1.0), c),
  ]..sort((a, b) => a.$1.compareTo(b.$1));

  /// Draws a picture over a box, cropped as its blip says; nothing until it
  /// is loaded.
  void picture(Canvas canvas, Rect box, Map<String, Object?> blip) {
    final media = blip['media'];
    final image = media is String ? images?.call(media) : null;
    if (image == null) return;
    var src = Rect.fromLTWH(0, 0, image.width.toDouble(), image.height.toDouble());
    final crop = blip['rect'];
    if (crop is List<Object?> && crop.length == 4 && crop.every((x) => x is num)) {
      final c = crop.cast<num>().map((v) => v / 100000).toList();
      src = Rect.fromLTRB(
        src.width * c[0],
        src.height * c[1],
        src.width * (1 - c[2]),
        src.height * (1 - c[3]),
      );
    }
    final paint = Paint()..filterQuality = FilterQuality.medium;
    final alpha = blip['alpha'];
    if (alpha is num) paint.color = Color.fromRGBO(0, 0, 0, (alpha / 100000).clamp(0, 1));
    if (blip['tile'] == true) {
      paint.shader = ImageShader(image, TileMode.repeated, TileMode.repeated, Float64List.fromList([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));
      canvas.drawRect(box, paint);
      return;
    }
    if (src.width > 0 && src.height > 0) canvas.drawImageRect(image, src, box, paint);
  }

  /// Strokes a path with a line as the Go package writes it.
  void line(Canvas canvas, Path path, Map<String, Object?>? line) {
    if (line == null) return;
    final fill = line['fill'];
    if (fill is! Map<String, Object?> || fill['none'] == true) return;
    final width = line['w'] is num ? (line['w']! as num) / 12700 : 0.75;
    final color = this.color(fill);
    if (color == null) return;
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = math.max(width, 0.25)
      ..color = color
      ..strokeCap = switch (line['cap']) {
        'rnd' => StrokeCap.round,
        'sq' => StrokeCap.square,
        _ => StrokeCap.butt,
      }
      ..strokeJoin = switch (line['join']) {
        'bevel' => StrokeJoin.bevel,
        'miter' => StrokeJoin.miter,
        _ => StrokeJoin.round,
      };
    final dash = _dashes[line['dash']];
    canvas.drawPath(dash == null ? path : _dashed(path, [for (final d in dash) d * paint.strokeWidth]), paint);
    for (final (key, atEnd) in [('head', false), ('tail', true)]) {
      final end = line[key];
      if (end is Map<String, Object?> && end['type'] is String && end['type'] != 'none') {
        _arrow(canvas, path, end, paint, atEnd: atEnd);
      }
    }
  }

  void _arrow(Canvas canvas, Path path, Map<String, Object?> end, Paint stroke, {required bool atEnd}) {
    final metrics = path.computeMetrics().toList();
    if (metrics.isEmpty) return;
    final metric = atEnd ? metrics.last : metrics.first;
    final tangent = metric.getTangentForOffset(atEnd ? metric.length : 0);
    if (tangent == null) return;
    double factor(Object? v) => switch (v) {
      'sm' => 2,
      'lg' => 5,
      _ => 3,
    };
    final w = stroke.strokeWidth * factor(end['w']), l = stroke.strokeWidth * factor(end['len']);
    // pointing out of the line
    final dir = atEnd ? tangent.vector : -tangent.vector;
    final angle = math.atan2(dir.dy, dir.dx);
    canvas.save();
    canvas.translate(tangent.position.dx, tangent.position.dy);
    canvas.rotate(angle);
    final fill = Paint()..color = stroke.color;
    switch (end['type']) {
      case 'triangle':
        canvas.drawPath(Path()..moveTo(0, 0)..lineTo(-l, -w / 2)..lineTo(-l, w / 2)..close(), fill);
      case 'stealth':
        canvas.drawPath(Path()..moveTo(0, 0)..lineTo(-l, -w / 2)..lineTo(-l * 0.6, 0)..lineTo(-l, w / 2)..close(), fill);
      case 'diamond':
        canvas.drawPath(Path()..moveTo(l / 2, 0)..lineTo(0, -w / 2)..lineTo(-l / 2, 0)..lineTo(0, w / 2)..close(), fill);
      case 'oval':
        canvas.drawOval(Rect.fromCenter(center: Offset.zero, width: l, height: w), fill);
      case 'arrow':
        canvas.drawPath(Path()..moveTo(-l, -w / 2)..lineTo(0, 0)..lineTo(-l, w / 2), stroke);
    }
    canvas.restore();
  }
}

/// The preset dashes, in widths of the line: dash, space, dash…
const _dashes = <String, List<double>>{
  'dash': [4, 3],
  'dashDot': [4, 3, 1, 3],
  'dot': [1, 3],
  'lgDash': [8, 3],
  'lgDashDot': [8, 3, 1, 3],
  'lgDashDotDot': [8, 3, 1, 3, 1, 3],
  'sysDash': [3, 1],
  'sysDot': [1, 1],
  'sysDashDot': [3, 1, 1, 1],
  'sysDashDotDot': [3, 1, 1, 1, 1, 1],
};

Path _dashed(Path path, List<double> pattern) {
  final out = Path();
  final total = pattern.fold(0.0, (a, b) => a + b);
  if (total <= 0) return path;
  for (final metric in path.computeMetrics()) {
    var at = 0.0, i = 0;
    while (at < metric.length) {
      final len = pattern[i % pattern.length];
      if (i.isEven) out.addPath(metric.extractPath(at, math.min(at + len, metric.length)), Offset.zero);
      at += len;
      i++;
    }
  }
  return out;
}

final _patterns = <String, Shader>{};

/// A shader of a pattern fill, eight pixels a side as Office draws them.
Shader _pattern(String preset, Color fg, Color bg) =>
    _patterns.putIfAbsent('$preset ${fg.toARGB32()} ${bg.toARGB32()}', () {
      final bits = _patternBits[preset] ?? _patternBits['pct50']!;
      final recorder = ui.PictureRecorder();
      final canvas = Canvas(recorder);
      canvas.drawRect(const Rect.fromLTWH(0, 0, 8, 8), Paint()..color = bg);
      final paint = Paint()..color = fg;
      for (var y = 0; y < 8; y++) {
        for (var x = 0; x < 8; x++) {
          if (bits[y] & (0x80 >> x) != 0) canvas.drawRect(Rect.fromLTWH(x.toDouble(), y.toDouble(), 1, 1), paint);
        }
      }
      final image = recorder.endRecording().toImageSync(8, 8);
      // a pixel of the pattern is a point of the slide at 100 %
      return ImageShader(image, TileMode.repeated, TileMode.repeated, Float64List.fromList([0.75, 0, 0, 0, 0, 0.75, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));
    });

/// The patterns, row by row.
const _patternBits = <String, List<int>>{
  'pct5': [0x80, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00],
  'pct10': [0x80, 0x00, 0x08, 0x00, 0x80, 0x00, 0x08, 0x00],
  'pct20': [0x88, 0x00, 0x22, 0x00, 0x88, 0x00, 0x22, 0x00],
  'pct25': [0x88, 0x22, 0x88, 0x22, 0x88, 0x22, 0x88, 0x22],
  'pct30': [0xAA, 0x44, 0xAA, 0x11, 0xAA, 0x44, 0xAA, 0x11],
  'pct40': [0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x51],
  'pct50': [0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55],
  'pct60': [0xEE, 0x55, 0xBB, 0x55, 0xEE, 0x55, 0xBB, 0x55],
  'pct70': [0xEE, 0x77, 0xBB, 0xDD, 0xEE, 0x77, 0xBB, 0xDD],
  'pct75': [0xEE, 0xFF, 0xBB, 0xFF, 0xEE, 0xFF, 0xBB, 0xFF],
  'pct80': [0xF7, 0xFF, 0x7F, 0xFF, 0xF7, 0xFF, 0x7F, 0xFF],
  'pct90': [0xFF, 0xF7, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF, 0xFF],
  'horz': [0xFF, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00],
  'vert': [0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88],
  'ltHorz': [0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
  'ltVert': [0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80],
  'dkHorz': [0xFF, 0xFF, 0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00],
  'dkVert': [0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC],
  'cross': [0xFF, 0x88, 0x88, 0x88, 0xFF, 0x88, 0x88, 0x88],
  'dnDiag': [0x88, 0x44, 0x22, 0x11, 0x88, 0x44, 0x22, 0x11],
  'upDiag': [0x11, 0x22, 0x44, 0x88, 0x11, 0x22, 0x44, 0x88],
  'ltDnDiag': [0x80, 0x40, 0x20, 0x10, 0x08, 0x04, 0x02, 0x01],
  'ltUpDiag': [0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80],
  'dkDnDiag': [0xCC, 0x66, 0x33, 0x99, 0xCC, 0x66, 0x33, 0x99],
  'dkUpDiag': [0x33, 0x66, 0xCC, 0x99, 0x33, 0x66, 0xCC, 0x99],
  'wdDnDiag': [0xC1, 0xE0, 0x70, 0x38, 0x1C, 0x0E, 0x07, 0x83],
  'wdUpDiag': [0x83, 0x07, 0x0E, 0x1C, 0x38, 0x70, 0xE0, 0xC1],
  'diagCross': [0x81, 0x42, 0x24, 0x18, 0x18, 0x24, 0x42, 0x81],
  'smCheck': [0x99, 0x66, 0x66, 0x99, 0x99, 0x66, 0x66, 0x99],
  'lgCheck': [0xF0, 0xF0, 0xF0, 0xF0, 0x0F, 0x0F, 0x0F, 0x0F],
  'smGrid': [0xFF, 0x88, 0x88, 0x88, 0xFF, 0x88, 0x88, 0x88],
  'lgGrid': [0xFF, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80],
  'dotGrid': [0xAA, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x00],
};
