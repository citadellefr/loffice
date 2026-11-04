import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../text/text_frame.dart';
import 'color.dart';
import 'geometry.dart';
import 'paint.dart';

const _emuPerPoint = 12700.0;

/// Paints the drawing recorded of a SmartArt, in points from the corner of
/// the graphic: the shapes as the file lays them out, not the diagram laid
/// out again.
class DiagramPainter {
  DiagramPainter._(this._shapes, this.colors, this.images, this.fonts, this.themeFills, this.themeLines, this.backgroundFills);

  /// The painter of a diagram as the Go package writes it, null when it
  /// has no shape.
  static DiagramPainter? of(
    Object? json, {
    required ColorContext colors,
    ImageSource? images,
    Fonts fonts = const Fonts(),
    List<Object?> themeFills = const [],
    List<Object?> themeLines = const [],
    List<Object?> backgroundFills = const [],
  }) {
    final shapes = json is Map<String, Object?> ? json['shapes'] : null;
    if (shapes is! List<Object?>) return null;
    final list = [
      for (final s in shapes)
        if (s is Map<String, Object?>) s,
    ];
    return list.isEmpty ? null : DiagramPainter._(list, colors, images, fonts, themeFills, themeLines, backgroundFills);
  }

  final List<Map<String, Object?>> _shapes;
  final ColorContext colors;
  final ImageSource? images;
  final Fonts fonts;
  final List<Object?> themeFills;
  final List<Object?> themeLines;
  final List<Object?> backgroundFills;

  final _texts = <Map<String, Object?>, TextFrame?>{};

  void paint(Canvas canvas) {
    for (final s in _shapes) {
      _shape(canvas, s);
    }
  }

  void _shape(Canvas canvas, Map<String, Object?> s) {
    final xfrm = _map(s['xfrm']);
    final box = _rect(xfrm);
    if (xfrm == null || box == null) return;
    final size = box.size;
    final geometry = Geometry.of(_map(s['geom']));
    final adjust = Geometry.adjustments(_map(s['geom']));
    final paths = geometry.paths(size, adjust);
    canvas.save();
    _place(canvas, xfrm, box, flips: true);
    final style = _map(s['style']);
    final (fill, fillColors) = styledFill(_map(s['fill']), style, colors, themeFills: themeFills, backgroundFills: backgroundFills);
    final painter = DrawingPainter(fillColors, images: images);
    for (final p in paths) {
      painter.fill(canvas, p.path, Offset.zero & size, fill, mode: p.fill);
    }
    final blip = _map(s['blip']);
    if (blip != null) {
      final outline = Path();
      for (final p in paths) {
        outline.addPath(p.path, Offset.zero);
      }
      canvas.save();
      canvas.clipPath(outline);
      DrawingPainter(colors, images: images).picture(canvas, Offset.zero & size, blip);
      canvas.restore();
    }
    final (line, lineColors) = styledLine(_map(s['line']), style, colors, themeLines: themeLines);
    for (final p in paths) {
      if (p.stroke) DrawingPainter(lineColors).line(canvas, p.path, line);
    }
    canvas.restore();
    final text = _texts.putIfAbsent(s, () => _layout(s, geometry.textRect(size, adjust)));
    if (text == null) return;
    canvas.save();
    final own = _map(s['txXfrm']);
    final ownBox = _rect(own);
    _place(canvas, own ?? xfrm, ownBox ?? box, flips: false);
    text.paint(canvas);
    canvas.restore();
  }

  TextFrame? _layout(Map<String, Object?> s, Rect textRect) {
    final flow = Delta.fromJson(s['text']);
    if (flow == null || flow.length == 0) return null;
    final own = _rect(_map(s['txXfrm']));
    final lst = _map(s['lst']);
    Props level(int lvl) => {..._strings(_map(lst?['0'])), ..._strings(_map(lst?['${lvl + 1}']))};
    final fontRef = _map(_map(s['style'])?['font']);
    return TextFrame.layout(
      flow,
      box: own == null ? textRect : Offset.zero & own.size,
      body: _strings(_map(s['body'])),
      levels: level,
      colors: colors,
      fonts: fonts,
      defaultColor: fontRef == null ? null : colors.resolve(fontRef['color']),
    );
  }

  /// Moves the canvas to the corner of a box, turned about its center.
  void _place(Canvas canvas, Map<String, Object?> xfrm, Rect box, {required bool flips}) {
    final degrees = xfrm['rot'] is num ? (xfrm['rot']! as num) / 60000 : 0;
    canvas.translate(box.center.dx, box.center.dy);
    canvas.rotate(degrees * math.pi / 180);
    if (flips) canvas.scale(xfrm['flipH'] == true ? -1 : 1, xfrm['flipV'] == true ? -1 : 1);
    canvas.translate(-box.width / 2, -box.height / 2);
  }

  Rect? _rect(Map<String, Object?>? xfrm) {
    if (xfrm == null) return null;
    double v(String k) => (xfrm[k] is num ? (xfrm[k]! as num).toDouble() : 0) / _emuPerPoint;
    return Rect.fromLTWH(v('x'), v('y'), v('w'), v('h'));
  }
}

Map<String, Object?>? _map(Object? v) => v is Map<String, Object?> ? v : null;

Map<String, String> _strings(Map<String, Object?>? v) => {
  if (v != null)
    for (final e in v.entries)
      if (e.value is String) e.key: e.value! as String,
};
