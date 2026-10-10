import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:characters/characters.dart';
import 'package:flutter/painting.dart';

import '../fonts.dart';
import 'document.dart';
import 'truetype.dart';

/// Paints laid out text: as text on the pages of an export, where a
/// paragraph of the engine would be a picture of it.
void paintText(Canvas canvas, TextPainter painter, Offset at) {
  if (canvas is PdfCanvas) {
    canvas.drawText(painter, at);
  } else {
    painter.paint(canvas, at);
  }
}

/// A gradient as it was asked of the engine, which does not tell it back.
class GradientSpec {
  const GradientSpec.linear(Offset this.from, Offset this.to, this.colors, this.stops) : center = null, radius = 0;
  const GradientSpec.radial(Offset this.center, this.radius, this.colors, this.stops) : from = null, to = null;

  final Offset? from, to, center;
  final double radius;
  final List<Color> colors;
  final List<double> stops;
}

/// The gradients of the shaders made by [gradient], and the files of the
/// pictures decoded: what an export writes in place of pixels.
final _gradients = Expando<GradientSpec>();
final pictureFiles = Expando<Uint8List>();

/// The color that stands on an export for a shader it cannot write: the
/// tile of a pattern.
final _flats = Expando<Color>();

/// Remembers the color an export fills with in place of [shader].
T flat<T extends ui.Shader>(T shader, Color color) {
  _flats[shader] = color;
  return shader;
}

/// The shader of a gradient, remembered for the exports.
ui.Gradient gradient(GradientSpec spec) {
  final shader = spec.center == null
      ? ui.Gradient.linear(spec.from!, spec.to!, spec.colors, spec.stops)
      : ui.Gradient.radial(spec.center!, spec.radius, spec.colors, spec.stops);
  _gradients[shader] = spec;
  return shader;
}

/// A document exported as a PDF: each page is painted on a canvas as it is
/// on screen, and written as lines, curves and text set in the fonts of
/// [OfficeFonts].
class PdfExport {
  PdfExport() {
    if (OfficeFonts.file('Carlito') == null) {
      throw StateError('the fonts are not loaded: ${OfficeFonts.failure ?? 'OfficeFonts.load() was not called'}');
    }
  }

  final _doc = PdfDocument();
  final _images = <ui.Image, Future<String?>>{};
  final _faces = <Uint8List, TrueTypeFont>{};

  /// The characters no font of the export draws, left blank on its pages.
  final missing = <String>{};

  /// Adds a page of [size], in points, painted by [paint].
  Future<void> page(Size size, void Function(Canvas canvas) paint) async {
    final canvas = PdfCanvas._(this);
    paint(canvas);
    _doc.addPage(size.width, size.height, await canvas._finish());
  }

  Uint8List save({String title = ''}) => _doc.save(title: title);

  TrueTypeFont? _face(String family, bool bold, bool italic) {
    final file = OfficeFonts.file(family, bold: bold, italic: italic);
    return file == null ? null : _faces[file] ??= TrueTypeFont(file);
  }

  /// The name of a picture in the document, null when it cannot be read.
  Future<String?> _image(ui.Image image) => _images[image] ??= _encode(image);

  Future<String?> _encode(ui.Image image) async {
    final file = pictureFiles[image];
    if (file != null) {
      if (_jpegFrame(file) case final frame?) return _doc.jpeg(frame.width, frame.height, frame.components, file);
    }
    final data = await image.toByteData(format: ui.ImageByteFormat.rawStraightRgba);
    if (data == null) return null;
    final pixels = image.width * image.height;
    final rgb = Uint8List(3 * pixels), alpha = Uint8List(pixels);
    var opaque = true;
    for (var i = 0; i < pixels; i++) {
      rgb[3 * i] = data.getUint8(4 * i);
      rgb[3 * i + 1] = data.getUint8(4 * i + 1);
      rgb[3 * i + 2] = data.getUint8(4 * i + 2);
      alpha[i] = data.getUint8(4 * i + 3);
      opaque = opaque && alpha[i] == 255;
    }
    return _doc.image(image.width, image.height, rgb, opaque ? null : alpha);
  }
}

/// The size and color components of a JPEG file, null for another file.
({int width, int height, int components})? _jpegFrame(Uint8List file) {
  if (file.length < 4 || file[0] != 0xFF || file[1] != 0xD8) return null;
  for (var at = 2; at + 9 < file.length;) {
    if (file[at] != 0xFF) return null;
    final marker = file[at + 1];
    if (marker == 0xFF) {
      at++;
      continue;
    }
    // the frame header: any SOF but the tables that share its range
    if (marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC) {
      return (height: file[at + 5] << 8 | file[at + 6], width: file[at + 7] << 8 | file[at + 8], components: file[at + 9]);
    }
    at += 2 + (file[at + 2] << 8 | file[at + 3]);
  }
  return null;
}

/// What the operators of a page last set, which the next ones need not
/// set again.
class _State {
  _State({this.fill, this.stroke, this.gs, this.width, this.cap, this.join, this.opacity = 1});

  String? fill, stroke, gs, width, cap, join;

  /// The opacity of the layers the drawing is in.
  double opacity;

  _State copy() => _State(fill: fill, stroke: stroke, gs: gs, width: width, cap: cap, join: join, opacity: opacity);
}

/// The canvas of a page of a [PdfExport]. It takes what the painters of
/// the package ask of a canvas, and refuses the rest.
class PdfCanvas implements Canvas {
  PdfCanvas._(this._export);

  final PdfExport _export;
  final _out = StringBuffer();
  final _states = [_State()];
  final _pictures = <(String, ui.Image)>[];

  _State get _state => _states.last;
  PdfDocument get _doc => _export._doc;

  /// How far a curve may be from the lines that stand for it, in points.
  static const _tolerance = 0.02;

  Future<String> _finish() async {
    while (_states.length > 1) {
      restore();
    }
    var content = '$_out';
    for (final (mark, image) in _pictures) {
      final name = await _export._image(image);
      content = content.replaceAll(mark, name == null ? 'n' : '/$name Do');
    }
    return content;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) =>
      throw UnsupportedError('${invocation.memberName} is not exported to PDF');

  // state

  @override
  void save() {
    _out.writeln('q');
    _states.add(_state.copy());
  }

  @override
  void saveLayer(Rect? bounds, Paint paint) {
    save();
    _state.opacity *= paint.color.a;
  }

  @override
  void restore() {
    if (_states.length == 1) return;
    _states.removeLast();
    _out.writeln('Q');
  }

  @override
  int getSaveCount() => _states.length;

  void _matrix(double a, double b, double c, double d, double e, double f) =>
      _out.writeln('${[a, b, c, d, e, f].map(pdfNumber).join(' ')} cm');

  @override
  void translate(double dx, double dy) => _matrix(1, 0, 0, 1, dx, dy);

  @override
  void scale(double sx, [double? sy]) => _matrix(sx, 0, 0, sy ?? sx, 0, 0);

  @override
  void rotate(double radians) => _matrix(math.cos(radians), math.sin(radians), -math.sin(radians), math.cos(radians), 0, 0);

  @override
  void skew(double sx, double sy) => _matrix(1, sy, sx, 1, 0, 0);

  @override
  void transform(Float64List matrix4) => _matrix(matrix4[0], matrix4[1], matrix4[4], matrix4[5], matrix4[12], matrix4[13]);

  @override
  void clipRect(Rect rect, {ui.ClipOp clipOp = ui.ClipOp.intersect, bool doAntiAlias = true}) =>
      _out.writeln('${_rect(rect)} W n');

  @override
  void clipRRect(RRect rrect, {bool doAntiAlias = true}) => _out.writeln('${_rrect(rrect)} W n');

  @override
  void clipPath(Path path, {bool doAntiAlias = true}) =>
      _out.writeln('${_path(path)} ${path.fillType == PathFillType.evenOdd ? 'W*' : 'W'} n');

  // shapes

  static String _point(Offset p) => '${pdfNumber(p.dx)} ${pdfNumber(p.dy)}';

  static String _rect(Rect r) => '${pdfNumber(r.left)} ${pdfNumber(r.top)} ${pdfNumber(r.width)} ${pdfNumber(r.height)} re';

  static String _curve(Offset a, Offset b, Offset c) => '${_point(a)} ${_point(b)} ${_point(c)} c';

  /// A rectangle with rounded corners: a quarter of an ellipse is drawn by
  /// a curve whose handles are this far along its sides.
  static const _kappa = 0.5522847498;

  static String _rrect(RRect r) {
    final out = StringBuffer('${_point(Offset(r.left + r.tlRadiusX, r.top))} m ');
    void corner(Offset from, Offset corner, Offset to) {
      out.write('${_point(from)} l ');
      out.write('${_curve(from + (corner - from) * _kappa, to + (corner - to) * _kappa, to)} ');
    }

    corner(Offset(r.right - r.trRadiusX, r.top), r.outerRect.topRight, Offset(r.right, r.top + r.trRadiusY));
    corner(Offset(r.right, r.bottom - r.brRadiusY), r.outerRect.bottomRight, Offset(r.right - r.brRadiusX, r.bottom));
    corner(Offset(r.left + r.blRadiusX, r.bottom), r.outerRect.bottomLeft, Offset(r.left, r.bottom - r.blRadiusY));
    corner(Offset(r.left, r.top + r.tlRadiusY), r.outerRect.topLeft, Offset(r.left + r.tlRadiusX, r.top));
    out.write('h');
    return '$out';
  }

  /// The contours of a path as lines: the engine gives the points along a
  /// path, not the curves it was made of. Each stretch is cut in two until
  /// the path stays within [_tolerance] of its chord, which finds corners
  /// as it rounds curves.
  static String _path(Path path) {
    final out = StringBuffer();
    for (final contour in path.computeMetrics()) {
      Offset at(double d) => contour.getTangentForOffset(d)?.position ?? Offset.zero;
      final points = [at(0)];
      void cut(double d0, Offset p0, double d1, Offset p1, int depth) {
        final middle = (d0 + d1) / 2;
        final pm = at(middle);
        if (depth >= 18 ||
            d1 - d0 < 0.01 ||
            _away(pm, p0, p1) < _tolerance &&
                _away(at((d0 + middle) / 2), p0, p1) < _tolerance &&
                _away(at((middle + d1) / 2), p0, p1) < _tolerance) {
          points.add(p1);
          return;
        }
        cut(d0, p0, middle, pm, depth + 1);
        cut(middle, pm, d1, p1, depth + 1);
      }

      cut(0, points.first, contour.length, at(contour.length), 0);
      // the points gathered around a corner lie on its two sides
      final kept = [points.first];
      for (var i = 1; i < points.length; i++) {
        if (i + 1 < points.length && _away(points[i], kept.last, points[i + 1]) < _tolerance / 4) continue;
        kept.add(points[i]);
      }
      out.write('${_point(kept.first)} m ');
      final last = contour.isClosed ? kept.length - 1 : kept.length;
      for (var i = 1; i < last; i++) {
        out.write('${_point(kept[i])} l ');
      }
      if (contour.isClosed) out.write('h ');
    }
    return '$out'.trimRight();
  }

  /// How far [p] is from the stretch from [a] to [b].
  static double _away(Offset p, Offset a, Offset b) {
    final ab = b - a;
    final length = ab.distanceSquared;
    if (length == 0) return (p - a).distance;
    final t = (((p - a).dx * ab.dx + (p - a).dy * ab.dy) / length).clamp(0.0, 1.0);
    return (p - (a + ab * t)).distance;
  }

  static String _rgb(Color c) => '${pdfNumber(c.r)} ${pdfNumber(c.g)} ${pdfNumber(c.b)}';

  void _set(String? Function(_State s) get, void Function(_State s, String v) set, String value, String operator) {
    if (get(_state) == value) return;
    set(_state, value);
    _out.writeln('$value $operator'.trimLeft());
  }

  void _opacity(double alpha) {
    final a = (alpha * _state.opacity).clamp(0.0, 1.0);
    _set((s) => s.gs, (s, v) => s.gs = v, '/${_doc.alpha(a, a)}', 'gs');
  }

  /// Fills or strokes [shape], the operators of a path, as [paint] says.
  /// What a PDF has no simple word for is left out: blurs, tiled pictures;
  /// a pattern is filled with the color between its two.
  void _draw(String shape, Paint paint, {bool evenOdd = false, bool? stroke}) {
    if (paint.maskFilter != null || shape.isEmpty) return;
    if (stroke ?? paint.style == PaintingStyle.stroke) {
      _opacity(paint.color.a);
      _set((s) => s.stroke, (s, v) => s.stroke = v, _rgb(paint.color), 'RG');
      _set((s) => s.width, (s, v) => s.width = v, pdfNumber(math.max(paint.strokeWidth, 0.1)), 'w');
      _set((s) => s.cap, (s, v) => s.cap = v, '${paint.strokeCap.index}', 'J');
      _set((s) => s.join, (s, v) => s.join = v, '${paint.strokeJoin.index}', 'j');
      _out.writeln('$shape S');
      return;
    }
    final shader = paint.shader;
    if (shader == null) {
      _opacity(paint.color.a);
      _set((s) => s.fill, (s, v) => s.fill = v, _rgb(paint.color), 'rg');
      _out.writeln('$shape ${evenOdd ? 'f*' : 'f'}');
      return;
    }
    final spec = _gradients[shader];
    if (spec == null) {
      final color = _flats[shader];
      if (color == null) return;
      _opacity(color.a * paint.color.a);
      _set((s) => s.fill, (s, v) => s.fill = v, _rgb(color), 'rg');
      _out.writeln('$shape ${evenOdd ? 'f*' : 'f'}');
      return;
    }
    final coords = spec.center == null
        ? [spec.from!.dx, spec.from!.dy, spec.to!.dx, spec.to!.dy]
        : [spec.center!.dx, spec.center!.dy, 0.0, spec.center!.dx, spec.center!.dy, spec.radius];
    final name = _doc.shading(coords, [for (final c in spec.colors) [c.r, c.g, c.b]], spec.stops);
    _opacity(spec.colors.map((c) => c.a).reduce(math.max) * paint.color.a);
    _out.writeln('q $shape ${evenOdd ? 'W*' : 'W'} n /$name sh Q');
  }

  @override
  void drawRect(Rect rect, Paint paint) => _draw(_rect(rect), paint);

  @override
  void drawRRect(RRect rrect, Paint paint) => _draw(_rrect(rrect), paint);

  @override
  void drawOval(Rect rect, Paint paint) =>
      _draw(_rrect(RRect.fromRectAndRadius(rect, Radius.elliptical(rect.width / 2, rect.height / 2))), paint);

  @override
  void drawCircle(Offset c, double radius, Paint paint) => drawOval(Rect.fromCircle(center: c, radius: radius), paint);

  @override
  void drawLine(Offset p1, Offset p2, Paint paint) => _draw('${_point(p1)} m ${_point(p2)} l', paint, stroke: true);

  @override
  void drawArc(Rect rect, double startAngle, double sweepAngle, bool useCenter, Paint paint) {
    final path = Path();
    if (useCenter) path.moveTo(rect.center.dx, rect.center.dy);
    path.arcTo(rect, startAngle, sweepAngle, !useCenter);
    if (useCenter) path.close();
    drawPath(path, paint);
  }

  @override
  void drawPath(Path path, Paint paint) => _draw(_path(path), paint, evenOdd: path.fillType == PathFillType.evenOdd);

  @override
  void drawImageRect(ui.Image image, Rect src, Rect dst, Paint paint) {
    if (src.isEmpty || dst.isEmpty) return;
    final mark = '%picture${_pictures.length}%';
    _pictures.add((mark, image));
    _out.writeln('q');
    final previous = _state.gs;
    _opacity(paint.color.a);
    _out.writeln('${_rect(dst)} W n');
    // the whole picture is placed so that src falls on dst
    final sx = dst.width / src.width, sy = dst.height / src.height;
    _matrix(image.width * sx, 0, 0, -image.height * sy, dst.left - src.left * sx, dst.top - src.top * sy + image.height * sy);
    _out.writeln('$mark\nQ');
    _state.gs = previous;
  }

  // text

  /// Writes the text a painter laid out, each cluster where the painter
  /// has it: the lines break and the words sit as on screen.
  void drawText(TextPainter painter, Offset at) {
    final root = painter.text;
    if (root == null) return;
    final plain = root.toPlainText(includeSemanticsLabels: false);
    final runs = <({int start, int end, TextStyle style})>[];
    var length = 0;
    void visit(InlineSpan span, TextStyle inherited) {
      final style = span.style == null ? inherited : inherited.merge(span.style);
      if (span is! TextSpan) {
        length++;
        return;
      }
      if (span.text case final text? when text.isNotEmpty) {
        runs.add((start: length, end: length + text.length, style: style));
        length += text.length;
      }
      for (final child in span.children ?? const <InlineSpan>[]) {
        visit(child, style);
      }
    }

    visit(root, const TextStyle());
    if (runs.isEmpty) return;
    final lines = painter.computeLineMetrics();
    if (lines.isEmpty) return;
    // the baseline of the line whose top is at y
    double baseline(double y) => lines.firstWhere((l) => l.baseline > y + 0.01, orElse: () => lines.last).baseline;

    save();
    translate(at.dx, at.dy);
    for (final run in runs) {
      final background = run.style.background?.color ?? run.style.backgroundColor;
      if (background == null || background.a == 0) continue;
      for (final box in painter.getBoxesForSelection(TextSelection(baseOffset: run.start, extentOffset: run.end))) {
        drawRect(box.toRect(), Paint()..color = background);
      }
    }

    final text = _TextWriter(this);
    var run = 0, index = 0;
    for (final cluster in plain.characters) {
      final start = index;
      index += cluster.length;
      while (run < runs.length - 1 && start >= runs[run].end) {
        run++;
      }
      final first = cluster.runes.first;
      if (start < runs[run].start || first < 0x20 || first == 0xFFFC || first == 0x200B || first == 0xFEFF) continue;
      final style = runs[run].style;
      final face = _faceFor(style, cluster);
      if (face == null) {
        if (cluster.trim().isNotEmpty) _export.missing.add(cluster);
        continue;
      }
      final caret = painter.getOffsetForCaret(TextPosition(offset: start), Rect.zero);
      text.cluster(face, style, cluster, caret.dx, baseline(caret.dy));
    }
    text.end();

    for (final run in runs) {
      final decoration = run.style.decoration;
      if (decoration == null || decoration == TextDecoration.none) continue;
      final face = _faceFor(run.style, 'x');
      if (face == null) continue;
      final size = run.style.fontSize ?? 14;
      final thickness = math.max(face.underlineThickness / face.unitsPerEm * size, 0.4);
      final paint = Paint()..color = run.style.decorationColor ?? run.style.foreground?.color ?? run.style.color ?? const Color(0xFF000000);
      for (final box in painter.getBoxesForSelection(TextSelection(baseOffset: run.start, extentOffset: run.end))) {
        final base = baseline(box.top);
        void rule(double y) => drawRect(Rect.fromLTWH(box.left, y - thickness / 2, box.right - box.left, thickness), paint);
        if (decoration.contains(TextDecoration.underline)) {
          final y = base - face.underlinePosition / face.unitsPerEm * size;
          rule(y);
          if (run.style.decorationStyle == TextDecorationStyle.double) rule(y + 2 * thickness);
        }
        if (decoration.contains(TextDecoration.lineThrough)) rule(base - 0.26 * size);
        if (decoration.contains(TextDecoration.overline)) rule(base - face.ascent / face.unitsPerEm * size);
      }
    }
    restore();
  }

  /// The font that draws a cluster in a style: the family asked for when
  /// it is one of [OfficeFonts] and has the character, another of them
  /// otherwise.
  TrueTypeFont? _faceFor(TextStyle style, String cluster) {
    final bold = (style.fontWeight ?? FontWeight.normal).value >= 600;
    final italic = style.fontStyle == FontStyle.italic;
    final character = cluster.runes.first;
    for (final family in [?style.fontFamily, ...?style.fontFamilyFallback, ...OfficeFonts.families.keys]) {
      final face = _export._face(family.substring(family.lastIndexOf('/') + 1), bold, italic);
      if (face != null && face.glyph(character) != 0) return face;
    }
    return null;
  }
}

/// Writes clusters of text, gathering in one operator those that follow
/// each other on a line in the same font, size and color.
class _TextWriter {
  _TextWriter(this.canvas);

  final PdfCanvas canvas;
  final _glyphs = StringBuffer();
  PdfFont? _font;
  String _key = '';
  double _size = 0, _pen = 0;

  void cluster(TrueTypeFont face, TextStyle style, String cluster, double x, double baseline) {
    var size = style.fontSize ?? 14;
    // the small raised and lowered figures of a font, when it has none
    final features = style.fontFeatures ?? const <FontFeature>[];
    if (features.any((f) => f.feature == 'sups')) {
      baseline -= 0.35 * size;
      size *= 0.6;
    } else if (features.any((f) => f.feature == 'subs')) {
      baseline += 0.15 * size;
      size *= 0.6;
    }
    final color = style.foreground?.color ?? style.color ?? const Color(0xFF000000);
    final font = canvas._doc.font(face);
    final key = '${font.name} ${pdfNumber(size)} ${pdfNumber(baseline)} ${color.toARGB32()}';
    if (key != _key) {
      end();
      _key = key;
      _font = font;
      _size = size;
      _pen = x;
      canvas._opacity(color.a);
      canvas._set((s) => s.fill, (s, v) => s.fill = v, PdfCanvas._rgb(color), 'rg');
      canvas._out.write('BT /${font.name} ${pdfNumber(size)} Tf 1 0 0 -1 ${pdfNumber(x)} ${pdfNumber(baseline)} Tm [');
    }
    // the pen is moved to where the painter has the cluster, by
    // thousandths of the size
    final shift = (_pen - x) * 1000 / _size;
    if (shift.abs() >= 0.5) _glyphs.write('${pdfNumber(shift)} ');
    _pen = x;
    _glyphs.write('<');
    for (final rune in cluster.runes) {
      final glyph = font.glyph(rune);
      if (glyph == null) continue;
      _glyphs.write(glyph);
      _pen += font.advance(rune) * _size;
    }
    _glyphs.write('> ');
  }

  void end() {
    if (_font == null) return;
    canvas._out.writeln('$_glyphs] TJ ET');
    _glyphs.clear();
    _font = null;
    _key = '';
  }
}
