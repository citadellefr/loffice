import 'dart:ui' as ui;

import 'package:flutter/painting.dart';

import '../chart/chart.dart';
import '../chart/chart_painter.dart';
import '../drawing/color.dart';
import '../drawing/diagram.dart';
import '../text/text_frame.dart';
import 'blocks.dart';
import 'document.dart';
import 'layout.dart';
import 'paragraph.dart';

/// Draws the pages of a laid out document, in points from the page's
/// top-left.
class PagePainter {
  const PagePainter({this.images, this.context});

  /// The pictures of the document, by name, when they are loaded.
  final ui.Image? Function(String media)? images;

  /// The document's theme and fonts, which charts are drawn with.
  final WordContext? context;

  void paint(Canvas canvas, WordPage page, {bool dimHeaders = false}) {
    canvas.drawRect(Offset.zero & page.size, Paint()..color = const Color(0xFFFFFFFF));
    for (final p in page.pictures.where((p) => p.behind)) {
      _picture(canvas, p);
    }
    for (final c in page.cells) {
      final shading = c.cell.shading;
      if (shading != null) canvas.drawRect(c.rect, Paint()..color = shading);
    }
    for (final l in page.lines) {
      _band(canvas, l);
    }
    for (final l in page.lines.where((l) => l.area != PageArea.drawing)) {
      final dim = dimHeaders && l.area != PageArea.body;
      if (dim) canvas.saveLayer(null, Paint()..color = const Color(0x80FFFFFF));
      l.box.paint(canvas, l.origin, l.from, l.to, objects: _object);
      if (dim) canvas.restore();
    }
    for (final c in page.cells) {
      _borders(canvas, c);
    }
    final rule = Paint()
      ..color = const Color(0xFF000000)
      ..strokeWidth = 0.5;
    for (final (a, b) in page.rules) {
      canvas.drawLine(a, b, rule);
    }
    for (final p in page.pictures.where((p) => !p.behind)) {
      _picture(canvas, p);
    }
    // the text of text boxes, over their fill
    for (final l in page.lines.where((l) => l.area == PageArea.drawing)) {
      l.box.paint(canvas, l.origin, l.from, l.to, objects: _object);
    }
  }

  /// Draws a chart, a diagram or a picture once loaded; tells whether it could.
  bool _object(Canvas canvas, Map<String, Object?> picture, Rect rect) {
    final chart = _charts[picture] ??= ChartSpec.fromJson(picture['chart']);
    if (chart != null) {
      _chart(canvas, chart, rect);
      return true;
    }
    final diagram = _diagrams[picture] ??= _diagram(picture['diagram']);
    if (diagram != null) {
      canvas.save();
      canvas.translate(rect.left, rect.top);
      diagram.paint(canvas);
      canvas.restore();
      return true;
    }
    final media = picture['media'] as String?;
    final image = media == null ? null : images?.call(media);
    if (image == null) return false;
    drawPicture(canvas, image, rect);
    return true;
  }

  void _picture(Canvas canvas, PlacedPicture p) {
    if (_object(canvas, p.picture, p.rect)) return;
    if (p.picture['text'] != null) {
      final fill = hexColor(p.picture['fill']), line = hexColor(p.picture['line']);
      if (fill != null) canvas.drawRect(p.rect, Paint()..color = fill);
      if (line != null) {
        canvas.drawRect(
          p.rect,
          Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = 0.75
            ..color = line,
        );
      }
      return;
    }
    canvas.drawRect(
      p.rect,
      Paint()
        ..style = PaintingStyle.stroke
        ..color = const Color(0xFFB0B0B0),
    );
  }

  ColorContext get _colors => ColorContext(
    scheme: {
      for (final e in (context?.doc.themeColors ?? const <String, String>{}).entries)
        if (hexColor(e.value) != null) e.key: hexColor(e.value)!,
    },
  );

  Fonts get _fonts {
    final doc = context?.doc, fonts = context?.fonts;
    return Fonts(theme: (name) => doc?.typeface(name.startsWith('+mj') ? '+major' : '+minor') ?? 'Calibri', substitutes: fonts?.substitutes ?? Fonts.defaultSubstitutes, package: fonts?.package);
  }

  DiagramPainter? _diagram(Object? json) => DiagramPainter.of(json, colors: _colors, images: images, fonts: _fonts);

  void _chart(Canvas canvas, ChartSpec chart, Rect rect) {
    canvas.save();
    canvas.translate(rect.left, rect.top);
    ChartPainter(chart, colors: _colors, fonts: _fonts).paint(canvas, rect.size);
    canvas.restore();
  }

  /// The shading and borders of a paragraph behind its lines.
  void _band(Canvas canvas, PlacedLines l) {
    final para = l.box.para;
    final shading = hexColor(para['pshd']);
    final borders = para['pbdr'] == null ? null : jsonBorders(para['pbdr']!);
    if (shading == null && borders == null) return;
    final (left, right) = l.box.band;
    final rect = Rect.fromLTRB(l.origin.dx + left, l.top, l.origin.dx + right, l.bottom);
    if (shading != null) canvas.drawRect(rect, Paint()..color = shading);
    if (borders == null) return;
    final first = l.from == 0, last = l.to == l.box.lines.length;
    for (final e in borders.entries) {
      final (paint, width, space) = e.value;
      final pad = space + width / 2;
      switch (e.key) {
        case 'top' when first:
          canvas.drawLine(Offset(rect.left, rect.top - pad), Offset(rect.right, rect.top - pad), paint);
        case 'bottom' when last:
          canvas.drawLine(Offset(rect.left, rect.bottom + pad), Offset(rect.right, rect.bottom + pad), paint);
        case 'left':
          canvas.drawLine(Offset(rect.left - pad, rect.top), Offset(rect.left - pad, rect.bottom), paint);
        case 'right':
          canvas.drawLine(Offset(rect.right + pad, rect.top), Offset(rect.right + pad, rect.bottom), paint);
      }
    }
  }

  void _borders(Canvas canvas, PlacedCell c) {
    final r = c.rect;
    for (final e in c.cell.borders.entries) {
      final b = border(e.value);
      if (b == null) continue;
      final (paint, _, _) = b;
      switch (e.key) {
        case 'top':
          canvas.drawLine(r.topLeft, r.topRight, paint);
        case 'bottom':
          canvas.drawLine(r.bottomLeft, r.bottomRight, paint);
        case 'left':
          canvas.drawLine(r.topLeft, r.bottomLeft, paint);
        case 'right':
          canvas.drawLine(r.topRight, r.bottomRight, paint);
      }
    }
  }
}

/// The paint of a border, its width and its space from the text, in points;
/// null when it draws nothing.
(Paint, double, double)? border(Map<String, Object?> b) {
  final val = b['val'];
  if (val == null || val == 'nil' || val == 'none') return null;
  final width = ((b['sz'] as num?) ?? 4) / 8;
  final color = hexColor(b['color']) ?? const Color(0xFF000000);
  final paint = Paint()
    ..color = color
    ..strokeWidth = width.clamp(0.25, 6.0)
    ..style = PaintingStyle.stroke;
  return (paint, width, ((b['space'] as num?) ?? 0).toDouble());
}

/// The borders of a paragraph's "pbdr".
Map<String, (Paint, double, double)>? jsonBorders(String json) {
  final sides = jsonObject(json);
  if (sides == null) return null;
  final out = <String, (Paint, double, double)>{};
  for (final e in sides.entries) {
    final v = e.value;
    if (v is Map<String, Object?>) {
      final b = border(v);
      if (b != null) out[e.key] = b;
    }
  }
  return out.isEmpty ? null : out;
}

final _charts = Expando<ChartSpec>();
final _diagrams = Expando<DiagramPainter>();
