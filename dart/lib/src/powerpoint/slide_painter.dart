import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../chart/chart.dart';
import '../chart/chart_painter.dart';
import '../drawing/color.dart';
import '../drawing/geometry.dart';
import '../drawing/paint.dart';
import '../text/text_frame.dart';
import 'deck.dart';
import 'table.dart';

/// Fetches the bytes of a picture of the document by its name.
typedef MediaFetcher = Future<Uint8List> Function(String media);

/// The pictures of a document, decoded once and kept; listeners hear when
/// one arrives.
class MediaCache extends ChangeNotifier {
  MediaCache(this._fetch);

  final MediaFetcher _fetch;
  final _images = <String, ui.Image?>{};

  /// The picture if it is loaded; asking for one starts loading it.
  ui.Image? operator [](String media) {
    if (_images.containsKey(media)) return _images[media];
    _images[media] = null;
    _load(media);
    return null;
  }

  Future<void> _load(String media) async {
    try {
      final codec = await ui.instantiateImageCodec(await _fetch(media));
      final frame = await codec.getNextFrame();
      _images[media] = frame.image;
      notifyListeners();
    } on Object {
      // a picture that cannot be read stays blank
    }
  }

  @override
  void dispose() {
    for (final image in _images.values) {
      image?.dispose();
    }
    super.dispose();
  }
}

/// Paints the slides of a deck, in points, and keeps the layout of the text
/// of their shapes.
class SlidePainter {
  SlidePainter(this.deck, {this.images, this.fonts});

  final Deck deck;
  final ImageSource? images;

  /// The package bundling the free fonts that stand in for Office's.
  final String? fonts;

  final _frames = Expando<TextFrame>();
  final _charts = Expando<ChartSpec>();
  final _tables = Expando<TableLayout>();

  void paint(Canvas canvas, Node slide) {
    final size = deck.size;
    final colors = deck.colorsOf(slide);
    final theme = deck.themeOf(slide);
    canvas.save();
    canvas.clipRect(Offset.zero & size);
    _background(canvas, slide, size, colors, theme);
    for (final shape in deck.backdropOf(slide)) {
      _shape(canvas, shape, colors, theme, null);
    }
    for (final shape in deck.shapesOf(slide)) {
      _shape(canvas, shape, colors, theme, null);
    }
    canvas.restore();
  }

  void _background(Canvas canvas, Node slide, Size size, ColorContext colors, DeckTheme theme) {
    final rect = Offset.zero & size;
    canvas.drawRect(rect, Paint()..color = const Color(0xFFFFFFFF));
    final bg = deck.backgroundOf(slide);
    if (bg == null) return;
    var fill = bg['fill'] as Map<String, Object?>?;
    var ctx = colors;
    final ref = bg['ref'];
    if (fill == null && ref is num && ref > 0) {
      final list = ref >= 1001 ? theme.backgroundFills : theme.fills;
      final i = (ref >= 1001 ? ref - 1001 : ref - 1).toInt();
      fill = i < list.length ? list[i] as Map<String, Object?>? : null;
      ctx = colors.withPlaceholder(colors.resolve(bg['color']));
    }
    DrawingPainter(ctx, images: images).fill(canvas, Path()..addRect(rect), rect, fill);
  }

  /// Where a shape is drawn: the transform from its own box to the slide,
  /// rotations and flips of its groups included.
  Matrix4Like transformOf(Node shape) {
    final chain = <Node>[];
    for (Node? n = shape; n != null && n.type != 'slide' && n.type != 'layout' && n.type != 'master'; n = deck.tree[n.parent]) {
      chain.insert(0, n);
    }
    var m = Matrix4Like.identity();
    for (final n in chain) {
      m = m.multiply(_placement(n));
    }
    return m;
  }

  Matrix4Like _placement(Node n) {
    if (n.type == 'tr') return Matrix4Like.identity();
    if (n.type == 'tc') {
      final cell = cellOf(n);
      return cell == null ? Matrix4Like.identity() : Matrix4Like.translation(cell.rect.left, cell.rect.top);
    }
    final style = deck.styleOf(n);
    final box = style.bounds;
    if (box == null) return Matrix4Like.identity();
    final x = style.xfrm!;
    var m = Matrix4Like.translation(box.left, box.top)
        .multiply(_rotation(box.size, (x['rot'] is num ? (x['rot']! as num) / 60000 : 0).toDouble(), x['flipH'] == true, x['flipV'] == true));
    if (n.type == 'grp') {
      double v(String k) => (x[k] is num ? (x[k]! as num).toDouble() : 0) / emuPerPoint;
      final cw = v('cw'), ch = v('ch');
      m = m.multiply(Matrix4Like.scale(cw > 0 ? box.width / cw : 1, ch > 0 ? box.height / ch : 1))
          .multiply(Matrix4Like.translation(-v('cx'), -v('cy')));
    }
    return m;
  }

  Matrix4Like _rotation(Size size, double degrees, bool flipH, bool flipV) {
    if (degrees == 0 && !flipH && !flipV) return Matrix4Like.identity();
    final c = Offset(size.width / 2, size.height / 2);
    return Matrix4Like.translation(c.dx, c.dy)
        .multiply(Matrix4Like.rotation(degrees * math.pi / 180))
        .multiply(Matrix4Like.scale(flipH ? -1 : 1, flipV ? -1 : 1))
        .multiply(Matrix4Like.translation(-c.dx, -c.dy));
  }

  void _shape(Canvas canvas, Node shape, ColorContext colors, DeckTheme theme, Map<String, Object?>? groupFill) {
    if (shape.attributes['hidden'] == true) return;
    final style = deck.styleOf(shape);
    switch (shape.type) {
      case 'grp':
        canvas.save();
        canvas.transform(_placement(shape).storage);
        for (final c in deck.shapesOf(shape)) {
          _shape(canvas, c, colors, theme, style.fill ?? groupFill);
        }
        canvas.restore();
        return;
      case 'alt':
        for (final c in deck.shapesOf(shape)) {
          _shape(canvas, c, colors, theme, groupFill);
        }
        return;
    }
    final box = style.bounds;
    if (box == null) return;
    canvas.save();
    canvas.transform(_placement(shape).storage);
    final size = box.size;
    final table = tableOf(shape);
    if (table != null) {
      table.paint(canvas, images: images);
      canvas.restore();
      return;
    }
    if (shape.type == 'frame' || shape.type == 'other') {
      final chart = shape.type == 'frame' ? _charts[shape] ??= ChartSpec.fromJson(shape.attributes['chart']) : null;
      if (chart != null) {
        ChartPainter(chart, colors: colors, fonts: Fonts(theme: theme.typeface, package: fonts)).paint(canvas, size);
      } else {
        _stand(canvas, size);
      }
      canvas.restore();
      return;
    }
    final geometry = _geometry(style);
    final adjust = _adjust(style);
    final paths = geometry.paths(size, adjust);
    final (fill, fillColors) = _fill(style, colors, theme);
    final painter = DrawingPainter(fillColors, images: images);
    final resolvedFill = fill != null && fill['grp'] == true ? groupFill : fill;
    for (final p in paths) {
      if (p.fill != PathFill.none) painter.fill(canvas, p.path, Offset.zero & size, resolvedFill, mode: p.fill);
    }
    if (shape.type == 'pic') {
      final blip = shape.attributes['blip'];
      if (blip is Map<String, Object?>) {
        canvas.save();
        final outline = Path();
        for (final p in paths) {
          outline.addPath(p.path, Offset.zero);
        }
        canvas.clipPath(outline);
        DrawingPainter(colors, images: images).picture(canvas, Offset.zero & size, blip);
        canvas.restore();
      }
    }
    final (line, lineColors) = _line(style, colors, theme);
    for (final p in paths) {
      if (p.stroke) DrawingPainter(lineColors).line(canvas, p.path, line);
    }
    final frame = textOf(shape);
    frame?.paint(canvas);
    canvas.restore();
  }

  /// The table of a frame laid out, null for a frame that shows none.
  TableLayout? tableOf(Node frame) {
    if (frame.type != 'frame' || frame.attributes['frame'] != 'table') return null;
    final nodes = TableLayout.nodesOf(deck.tree, frame);
    final kept = _tables[frame];
    if (kept != null && kept.isOf(nodes)) return kept;
    final page = deck.pageOf(frame);
    return _tables[frame] = TableLayout.of(
      deck,
      frame,
      colors: page == null ? const ColorContext() : deck.colorsOf(page),
      theme: page == null ? DeckTheme(null) : deck.themeOf(page),
      fonts: fonts,
    );
  }

  /// The table frame a cell is in.
  Node? frameOf(Node cell) {
    final row = deck.tree[cell.parent];
    return row == null ? null : deck.tree[row.parent];
  }

  /// A cell of a table as it is drawn.
  TableCell? cellOf(Node cell) {
    final frame = frameOf(cell);
    return frame == null ? null : tableOf(frame)?.cells[cell.id];
  }

  /// The size a shape is drawn at, in its own coordinates: a table's as
  /// its rows and columns lay it out.
  Size? sizeOf(Node shape) => switch (shape.type) {
    'tc' => cellOf(shape)?.rect.size,
    _ => tableOf(shape)?.size ?? deck.styleOf(shape).bounds?.size,
  };

  /// What stands for an object the editor does not draw yet.
  void _stand(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    canvas.drawRect(rect, Paint()..color = const Color(0x14000000));
    canvas.drawRect(
      rect,
      Paint()
        ..style = PaintingStyle.stroke
        ..color = const Color(0x40000000),
    );
  }

  Geometry _geometry(ShapeStyle style) {
    final g = style.geometry;
    final preset = g?['prst'];
    if (preset is String) return Geometry.preset(preset) ?? Geometry.preset('rect')!;
    return Geometry.fromJson(g?['cust']) ?? Geometry.preset('rect')!;
  }

  Map<String, String> _adjust(ShapeStyle style) {
    final av = style.geometry?['av'];
    return {
      if (av is Map<String, Object?>)
        for (final e in av.entries)
          if (e.value is String) e.key: e.value! as String,
    };
  }

  /// The fill of a shape: its own, or the theme's its style points to, with
  /// the colors "phClr" resolves in.
  (Map<String, Object?>?, ColorContext) _fill(ShapeStyle style, ColorContext colors, DeckTheme theme) {
    final ref = style.style?['fill'];
    final refColors = ref is Map<String, Object?> ? colors.withPlaceholder(colors.resolve(ref['color'])) : colors;
    if (style.fill != null) return (style.fill, refColors);
    if (ref is! Map<String, Object?>) return (null, colors);
    final idx = int.tryParse('${ref['idx']}') ?? 0;
    if (idx <= 0) return (null, colors);
    final list = idx >= 1001 ? theme.backgroundFills : theme.fills;
    final i = idx >= 1001 ? idx - 1001 : idx - 1;
    return (i < list.length ? list[i] as Map<String, Object?>? : null, refColors);
  }

  /// The line of a shape: the theme's its style points to, with its own
  /// properties over it.
  (Map<String, Object?>?, ColorContext) _line(ShapeStyle style, ColorContext colors, DeckTheme theme) {
    final ref = style.style?['ln'];
    var ctx = colors;
    Map<String, Object?>? line;
    if (ref is Map<String, Object?>) {
      final idx = int.tryParse('${ref['idx']}') ?? 0;
      if (idx > 0 && idx <= theme.lines.length) line = theme.lines[idx - 1] as Map<String, Object?>?;
      ctx = colors.withPlaceholder(colors.resolve(ref['color']));
    }
    if (style.line != null) line = {...?line, ...style.line!};
    return (line, ctx);
  }

  /// The text of a shape laid out in its box, null for one without text.
  TextFrame? textOf(Node shape) {
    if (shape.type == 'tc') return cellOf(shape)?.text;
    final text = shape.text;
    if (shape.type != 'sp' || text == null) return null;
    return _frames[shape] ??= layout(shape, text);
  }

  /// Whether a shape is a placeholder with no text, which the editor shows
  /// with its prompt.
  bool isEmptyPlaceholder(Node shape) =>
      shape.type == 'sp' && shape.attributes['ph'] != null && (shape.text?.length ?? 1) <= 1;

  /// A text laid out as the shape would lay out its own, in [color] when
  /// given.
  TextFrame? layout(Node shape, Delta text, {Color? color}) {
    final style = deck.styleOf(shape);
    final box = style.bounds;
    if (box == null) return null;
    final page = deck.pageOf(shape);
    final colors = page == null ? const ColorContext() : deck.colorsOf(page);
    final theme = page == null ? DeckTheme(null) : deck.themeOf(page);
    final fontRef = style.style?['font'];
    Props levels(int lvl) => color == null
        ? style.level(lvl)
        : {...style.level(lvl), 'fill': '{"solid":{"rgb":"${(color.toARGB32() & 0xFFFFFF).toRadixString(16).padLeft(6, '0')}"}}'};
    return TextFrame.layout(
      text,
      box: _geometry(style).textRect(box.size, _adjust(style)),
      body: style.body,
      levels: levels,
      colors: colors,
      fonts: Fonts(theme: theme.typeface, package: fonts),
      defaultColor: color ?? (fontRef is Map<String, Object?> ? colors.resolve(fontRef['color']) : null),
    );
  }
}

/// A 2D affine transform as a 4×4 matrix, which Canvas.transform takes.
@immutable
class Matrix4Like {
  const Matrix4Like._(this.storage);

  factory Matrix4Like.identity() => Matrix4Like._(Float64List.fromList([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));

  factory Matrix4Like.translation(double x, double y) =>
      Matrix4Like._(Float64List.fromList([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, x, y, 0, 1]));

  factory Matrix4Like.scale(double x, double y) =>
      Matrix4Like._(Float64List.fromList([x, 0, 0, 0, 0, y, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));

  factory Matrix4Like.rotation(double radians) {
    final c = math.cos(radians), s = math.sin(radians);
    return Matrix4Like._(Float64List.fromList([c, s, 0, 0, -s, c, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));
  }

  /// Column-major, as Canvas.transform wants it.
  final Float64List storage;

  /// This transform after [other]: other applies first.
  Matrix4Like multiply(Matrix4Like other) {
    final a = storage, b = other.storage;
    final out = Float64List(16);
    for (var col = 0; col < 4; col++) {
      for (var row = 0; row < 4; row++) {
        var sum = 0.0;
        for (var k = 0; k < 4; k++) {
          sum += a[k * 4 + row] * b[col * 4 + k];
        }
        out[col * 4 + row] = sum;
      }
    }
    return Matrix4Like._(out);
  }

  Offset apply(Offset p) {
    final m = storage;
    return Offset(m[0] * p.dx + m[4] * p.dy + m[12], m[1] * p.dx + m[5] * p.dy + m[13]);
  }

  /// The inverse, for a transform that has one.
  Matrix4Like invert() {
    final m = storage;
    final det = m[0] * m[5] - m[1] * m[4];
    if (det == 0) return Matrix4Like.identity();
    final a = m[5] / det, b = -m[1] / det, c = -m[4] / det, d = m[0] / det;
    final tx = -(a * m[12] + c * m[13]), ty = -(b * m[12] + d * m[13]);
    return Matrix4Like._(Float64List.fromList([a, b, 0, 0, c, d, 0, 0, 0, 0, 1, 0, tx, ty, 0, 1]));
  }
}
