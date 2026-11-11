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

/// A tile as it was asked of the engine, which does not tell it back:
/// [matrix] places it, as the shader was given it.
sealed class TileSpec {
  const TileSpec(this.matrix);

  final Float64List matrix;
}

/// A picture repeated.
class ImageTile extends TileSpec {
  const ImageTile(this.image, super.matrix);

  final ui.Image image;
}

/// A pattern fill of DrawingML: [bits] are the rows of eight pixels, set
/// ones in [fg] and the others in [bg].
class PatternTile extends TileSpec {
  const PatternTile(this.bits, this.fg, this.bg, super.matrix);

  final List<int> bits;
  final Color fg, bg;
}

/// The gradients of the shaders made by [gradient], the tiles of those that
/// repeat, and the files of the pictures decoded: what an export writes in
/// place of pixels.
final _gradients = Expando<GradientSpec>();
final _tiles = Expando<TileSpec>();
final pictureFiles = Expando<Uint8List>();

/// Remembers the tile an export fills with for [shader].
T tiled<T extends ui.Shader>(T shader, TileSpec tile) {
  _tiles[shader] = tile;
  return shader;
}

/// The shader of a gradient, remembered for the exports.
ui.Gradient gradient(GradientSpec spec) {
  final shader = spec.center == null ? ui.Gradient.linear(spec.from!, spec.to!, spec.colors, spec.stops) : ui.Gradient.radial(spec.center!, spec.radius, spec.colors, spec.stops);
  _gradients[shader] = spec;
  return shader;
}

/// A document exported as a PDF: each page is painted on a canvas as it is
/// on screen, and written as lines, curves, pictures and text set in the
/// fonts of [OfficeFonts].
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

  /// What painters asked of their canvas and the export leaves aside: the
  /// vertices, atlases, paragraphs and pictures of the engine, filters and
  /// blend modes. Each is drawn as nothing, or as its plainest form.
  final ignored = <Ignored>{};

  /// Adds a page of [size], in points, painted by [paint].
  Future<void> page(Size size, void Function(Canvas canvas) paint) async {
    final canvas = PdfCanvas._(this, size);
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
      if (_pngFrame(file) case final frame?) return _doc.png(frame.width, frame.height, frame.space, frame.depth, frame.colors, frame.data);
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

/// The size, color space and zlib stream of an opaque PNG file that a PDF
/// reads as it is: not interlaced, in gray or palette colors of up to eight
/// bits or in eight-bit RGB, without a transparent color. Null for another.
({int width, int height, String space, int depth, int colors, Uint8List data})? _pngFrame(Uint8List file) {
  const signature = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
  if (file.length < 33 || !signature.indexed.every((e) => file[e.$1] == e.$2)) return null;
  final bytes = ByteData.sublistView(file);
  var width = 0, height = 0, depth = 0, type = -1;
  Uint8List? palette;
  final data = BytesBuilder(copy: false);
  for (var at = 8; at + 12 <= file.length;) {
    final length = bytes.getUint32(at), end = at + 12 + length;
    if (end > file.length) return null;
    final body = Uint8List.sublistView(file, at + 8, at + 8 + length);
    switch (String.fromCharCodes(file, at + 4, at + 8)) {
      case 'IHDR':
        if (length < 13 || body[10] != 0 || body[11] != 0 || body[12] != 0) return null;
        width = bytes.getUint32(at + 8);
        height = bytes.getUint32(at + 12);
        depth = body[8];
        type = body[9];
      case 'PLTE':
        palette = body;
      case 'tRNS':
        return null;
      case 'IDAT':
        data.add(body);
      case 'IEND':
        final space = switch (type) {
          0 => '/DeviceGray',
          2 => '/DeviceRGB',
          3 when palette != null && palette.length >= 3 => '[/Indexed /DeviceRGB ${palette.length ~/ 3 - 1} <${palette.map((b) => b.toRadixString(16).padLeft(2, '0')).join()}>]',
          _ => null,
        };
        if (space == null || width == 0 || height == 0 || data.isEmpty) return null;
        if (type == 2 ? depth != 8 : !const [1, 2, 4, 8].contains(depth)) return null;
        return (width: width, height: height, space: space, depth: depth, colors: type == 2 ? 3 : 1, data: data.takeBytes());
    }
    at = end;
  }
  return null;
}

/// An affine transform of the plane: a point goes to
/// (a x + c y + e, b x + d y + f).
class _Affine {
  const _Affine(this.a, this.b, this.c, this.d, this.e, this.f);

  /// The transform of a shader or a canvas, given as the engine has them.
  _Affine.of(Float64List m) : this(m[0], m[1], m[4], m[5], m[12], m[13]);

  static const identity = _Affine(1, 0, 0, 1, 0, 0);

  final double a, b, c, d, e, f;

  /// This transform followed by [next].
  _Affine then(_Affine next) =>
      _Affine(a * next.a + b * next.c, a * next.b + b * next.d, c * next.a + d * next.c, c * next.b + d * next.d, e * next.a + f * next.c + next.e, e * next.b + f * next.d + next.f);

  _Affine? get inverse {
    final det = a * d - b * c;
    if (det == 0) return null;
    return _Affine(d / det, -b / det, -c / det, a / det, (c * f - d * e) / det, (b * e - a * f) / det);
  }

  Rect bounds(Rect r) {
    final corners = [r.topLeft, r.topRight, r.bottomLeft, r.bottomRight].map((p) => Offset(a * p.dx + c * p.dy + e, b * p.dx + d * p.dy + f));
    return Rect.fromLTRB(corners.map((p) => p.dx).reduce(math.min), corners.map((p) => p.dy).reduce(math.min), corners.map((p) => p.dx).reduce(math.max), corners.map((p) => p.dy).reduce(math.max));
  }

  List<double> get list => [a, b, c, d, e, f];
}

/// What the operators of a page last set, which the next ones need not
/// set again.
class _State {
  _State({this.fill, this.stroke, this.gs, this.width, this.cap, this.join, this.opacity = 1, this.transform = _Affine.identity});

  String? fill, stroke, gs, width, cap, join;

  /// The opacity of the layers the drawing is in.
  double opacity;

  /// Where the drawing is on the page.
  _Affine transform;

  _State copy() => _State(fill: fill, stroke: stroke, gs: gs, width: width, cap: cap, join: join, opacity: opacity, transform: transform);
}

/// What an export leaves aside of what a painter asked.
enum Ignored { colorFilter, imageFilter, blendMode, maskFilter, shader, picture, paragraph, vertices, atlas, other }

/// The canvas of a page of a [PdfExport]. It writes what the painters of the
/// package ask of a canvas; what a PDF has no word for is left aside, and
/// told in [PdfExport.ignored].
class PdfCanvas implements Canvas {
  PdfCanvas._(this._export, this._size);

  final PdfExport _export;
  final Size _size;
  final _out = StringBuffer();
  final _states = [_State()];
  final _pictures = <(String, ui.Image)>[];
  final _patterns = <(Object, String), ({String mark, TileSpec spec, _Affine matrix})>{};

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
    for (final (:mark, :spec, :matrix) in _patterns.values) {
      content = content.replaceAll(mark, '/${await _pattern(spec, matrix)}');
    }
    return content;
  }

  /// The pattern of a tile placed by [matrix] on the page.
  Future<String> _pattern(TileSpec spec, _Affine matrix) async {
    switch (spec) {
      case ImageTile(:final image):
        final name = await _export._image(image);
        final w = image.width.toDouble(), h = image.height.toDouble();
        return _doc.tiling(matrix.list, w, h, name == null ? '' : '${pdfNumber(w)} 0 0 ${pdfNumber(-h)} 0 ${pdfNumber(h)} cm /$name Do', [?name]);
      case PatternTile(:final bits, :final fg, :final bg):
        final out = StringBuffer();
        if (bg.a > 0) out.write('${_rgb(bg)} rg 0 0 8 8 re f ');
        var any = false;
        out.write('${_rgb(fg)} rg ');
        for (var y = 0; y < 8; y++) {
          for (var x = 0; x < 8; x++) {
            if (bits[y] & (0x80 >> x) == 0) continue;
            var length = 1;
            while (x + length < 8 && bits[y] & (0x80 >> (x + length)) != 0) {
              length++;
            }
            out.write('$x $y $length 1 re ');
            x += length;
            any = true;
          }
        }
        if (any) out.write('f');
        return _doc.tiling(matrix.list, 8, 8, '$out');
    }
  }

  @override
  dynamic noSuchMethod(Invocation invocation) {
    _export.ignored.add(Ignored.other);
    return null;
  }

  // state

  @override
  void save() {
    _out.writeln('q');
    _states.add(_state.copy());
  }

  @override
  void saveLayer(Rect? bounds, Paint paint) {
    _leftAside(paint);
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
  void restoreToCount(int count) {
    while (_states.length > math.max(count, 1)) {
      restore();
    }
  }

  @override
  int getSaveCount() => _states.length;

  void _matrix(double a, double b, double c, double d, double e, double f) {
    _out.writeln('${[a, b, c, d, e, f].map(pdfNumber).join(' ')} cm');
    _state.transform = _Affine(a, b, c, d, e, f).then(_state.transform);
  }

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
  Float64List getTransform() {
    final t = _state.transform;
    return Float64List.fromList([t.a, t.b, 0, 0, t.c, t.d, 0, 0, 0, 0, 1, 0, t.e, t.f, 0, 1]);
  }

  @override
  void clipRect(Rect rect, {ui.ClipOp clipOp = ui.ClipOp.intersect, bool doAntiAlias = true}) => _out.writeln('${_rect(rect)} W n');

  @override
  void clipRRect(RRect rrect, {bool doAntiAlias = true}) => _out.writeln('${_rrect(rrect)} W n');

  @override
  void clipRSuperellipse(ui.RSuperellipse rsuperellipse, {bool doAntiAlias = true}) => clipPath(Path()..addRSuperellipse(rsuperellipse));

  @override
  void clipPath(Path path, {bool doAntiAlias = true}) => _out.writeln('${_path(path)} ${path.fillType == PathFillType.evenOdd ? 'W*' : 'W'} n');

  /// The clip is not followed: the bounds are those of the page.
  @override
  Rect getDestinationClipBounds() => Offset.zero & _size;

  @override
  Rect getLocalClipBounds() => (_state.transform.inverse ?? _Affine.identity).bounds(getDestinationClipBounds());

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
        if (depth >= 18 || d1 - d0 < 0.01 || _away(pm, p0, p1) < _tolerance && _away(at((d0 + middle) / 2), p0, p1) < _tolerance && _away(at((middle + d1) / 2), p0, p1) < _tolerance) {
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

  /// Notes what [paint] asks beyond what an export writes: a color filter,
  /// an image filter or a blend mode.
  void _leftAside(Paint paint) {
    if (paint.colorFilter != null) _export.ignored.add(Ignored.colorFilter);
    if (paint.imageFilter != null) _export.ignored.add(Ignored.imageFilter);
    if (paint.blendMode != BlendMode.srcOver) _export.ignored.add(Ignored.blendMode);
  }

  /// The tile that stands in a page for a repeated picture or pattern drawn
  /// from the origin of the canvas as it is now: the pattern is written
  /// once the pictures are encoded, and is marked until then.
  String _tile(TileSpec spec) {
    final matrix = _Affine.of(spec.matrix).then(_state.transform).then(_Affine(1, 0, 0, -1, 0, _size.height));
    final key = (spec is ImageTile ? spec.image : spec, matrix.list.join(' '));
    return (_patterns[key] ??= (mark: '%pattern${_patterns.length}%', spec: spec, matrix: matrix)).mark;
  }

  /// Fills or strokes [shape], the operators of a path, as [paint] says.
  /// A blur is written as shapes that fade out; the filters and blend modes
  /// of a paint are left aside.
  void _draw(String shape, Paint paint, {bool evenOdd = false, bool? stroke}) {
    if (shape.isEmpty) return;
    _leftAside(paint);
    final outline = stroke ?? paint.style == PaintingStyle.stroke;
    if (paint.maskFilter case final blur?) {
      final sigma = RegExp(r', ([\d.]+)\)$').firstMatch('$blur')?.group(1);
      if (sigma == null) {
        _export.ignored.add(Ignored.maskFilter);
        return;
      }
      _blur(shape, paint, double.parse(sigma), evenOdd: evenOdd, stroke: outline);
      return;
    }
    if (outline) {
      _stroke(shape, paint, paint.strokeWidth);
      return;
    }
    final shader = paint.shader;
    if (shader == null) {
      _fill(shape, paint.color, paint.color.a, evenOdd);
    } else if (_gradients[shader] case final spec?) {
      final coords = spec.center == null ? [spec.from!.dx, spec.from!.dy, spec.to!.dx, spec.to!.dy] : [spec.center!.dx, spec.center!.dy, 0.0, spec.center!.dx, spec.center!.dy, spec.radius];
      final name = _doc.shading(coords, [
        for (final c in spec.colors) [c.r, c.g, c.b],
      ], spec.stops);
      _opacity(spec.colors.map((c) => c.a).reduce(math.max) * paint.color.a);
      _out.writeln('q $shape ${evenOdd ? 'W*' : 'W'} n /$name sh Q');
    } else if (_tiles[shader] case final tile?) {
      _opacity(paint.color.a * (tile is PatternTile ? tile.fg.a : 1));
      _set((s) => s.fill, (s, v) => s.fill = v, '/Pattern cs ${_tile(tile)}', 'scn');
      _out.writeln('$shape ${evenOdd ? 'f*' : 'f'}');
    } else {
      _export.ignored.add(Ignored.shader);
    }
  }

  void _fill(String shape, Color color, double opacity, bool evenOdd) {
    _opacity(opacity);
    _set((s) => s.fill, (s, v) => s.fill = v, _rgb(color), 'rg');
    _out.writeln('$shape ${evenOdd ? 'f*' : 'f'}');
  }

  void _stroke(String shape, Paint paint, double width, {String operator = 'S'}) {
    _opacity(paint.color.a);
    _set((s) => s.stroke, (s, v) => s.stroke = v, _rgb(paint.color), 'RG');
    _set((s) => s.width, (s, v) => s.width = v, pdfNumber(math.max(width, 0.1)), 'w');
    _set((s) => s.cap, (s, v) => s.cap = v, '${paint.strokeCap.index}', 'J');
    _set((s) => s.join, (s, v) => s.join = v, '${paint.strokeJoin.index}', 'j');
    _out.writeln('$shape $operator');
  }

  /// How many shapes a blur is written as: the first widest and faintest,
  /// the last the shape itself.
  static const _blurLayers = 8;

  /// Writes [shape] blurred by a Gaussian of [sigma], as shapes grown by
  /// wider and wider strokes, each fainter than the one it covers. The
  /// shape only grows: the blur stays outside of it.
  void _blur(String shape, Paint paint, double sigma, {required bool evenOdd, required bool stroke}) {
    double normal(double x) => 1 / (1 + math.exp(-1.702 * x));
    final opacity = paint.color.a;
    var covered = 0.0;
    for (var i = 0; i <= _blurLayers; i++) {
      final grow = 3 * sigma * (_blurLayers - i) / _blurLayers;
      final target = i == _blurLayers ? opacity : opacity * (1 - normal((grow - 1.5 * sigma) / sigma));
      final layer = covered >= 1 ? 0.0 : 1 - (1 - target) / (1 - covered);
      covered = target;
      final faint = Paint()
        ..color = paint.color.withValues(alpha: layer)
        ..strokeCap = paint.strokeCap
        ..strokeJoin = i == _blurLayers ? paint.strokeJoin : StrokeJoin.round;
      if (stroke) {
        _stroke(shape, faint, paint.strokeWidth + 2 * grow);
      } else if (grow == 0) {
        _fill(shape, faint.color, layer, evenOdd);
      } else {
        _set((s) => s.fill, (s, v) => s.fill = v, _rgb(paint.color), 'rg');
        _stroke(shape, faint, 2 * grow, operator: evenOdd ? 'B*' : 'B');
      }
    }
  }

  @override
  void drawRect(Rect rect, Paint paint) => _draw(_rect(rect), paint);

  @override
  void drawRRect(RRect rrect, Paint paint) => _draw(_rrect(rrect), paint);

  @override
  void drawDRRect(RRect outer, RRect inner, Paint paint) => _draw('${_rrect(outer)} ${_rrect(inner)}', paint, evenOdd: true);

  @override
  void drawRSuperellipse(ui.RSuperellipse rsuperellipse, Paint paint) => drawPath(Path()..addRSuperellipse(rsuperellipse), paint);

  @override
  void drawOval(Rect rect, Paint paint) => _draw(_rrect(RRect.fromRectAndRadius(rect, Radius.elliptical(rect.width / 2, rect.height / 2))), paint);

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
  void drawPaint(Paint paint) => drawRect(getLocalClipBounds(), paint);

  @override
  void drawColor(Color color, BlendMode blendMode) {
    if (blendMode == BlendMode.srcOver || blendMode == BlendMode.src) {
      drawPaint(Paint()..color = color);
    } else {
      _export.ignored.add(Ignored.blendMode);
    }
  }

  /// Points are drawn as the squares or the discs of the width of the
  /// stroke, lines and polygons as strokes.
  @override
  void drawPoints(ui.PointMode pointMode, List<Offset> points, Paint paint) {
    switch (pointMode) {
      case ui.PointMode.points:
        final dot = Paint()..color = paint.color;
        for (final p in points) {
          final box = Rect.fromCenter(center: p, width: paint.strokeWidth, height: paint.strokeWidth);
          if (paint.strokeCap == StrokeCap.round) {
            drawOval(box, dot);
          } else {
            drawRect(box, dot);
          }
        }
      case ui.PointMode.lines:
        for (var i = 0; i + 1 < points.length; i += 2) {
          drawLine(points[i], points[i + 1], paint);
        }
      case ui.PointMode.polygon:
        for (var i = 0; i + 1 < points.length; i++) {
          drawLine(points[i], points[i + 1], paint);
        }
    }
  }

  @override
  void drawRawPoints(ui.PointMode pointMode, Float32List points, Paint paint) => drawPoints(pointMode, [for (var i = 0; i + 1 < points.length; i += 2) Offset(points[i], points[i + 1])], paint);

  /// A shadow as Material draws them: the shape lowered, blurred and faint.
  @override
  void drawShadow(Path path, Color color, double elevation, bool transparentOccluder) {
    save();
    translate(0, elevation / 2);
    _blur(_path(path), Paint()..color = color.withValues(alpha: color.a * 0.3), elevation / 2, evenOdd: path.fillType == PathFillType.evenOdd, stroke: false);
    restore();
  }

  @override
  void drawImage(ui.Image image, Offset offset, Paint paint) {
    final size = Size(image.width.toDouble(), image.height.toDouble());
    drawImageRect(image, Offset.zero & size, offset & size, paint);
  }

  @override
  void drawImageRect(ui.Image image, Rect src, Rect dst, Paint paint) {
    if (src.isEmpty || dst.isEmpty) return;
    _leftAside(paint);
    final mark = '%picture${_pictures.length}%';
    _pictures.add((mark, image));
    save();
    _opacity(paint.color.a);
    _out.writeln('${_rect(dst)} W n');
    // the whole picture is placed so that src falls on dst
    final sx = dst.width / src.width, sy = dst.height / src.height;
    _matrix(image.width * sx, 0, 0, -image.height * sy, dst.left - src.left * sx, dst.top - src.top * sy + image.height * sy);
    _out.writeln(mark);
    restore();
  }

  /// A picture in nine parts: the corners as they are, the sides stretched
  /// along themselves and the center both ways.
  @override
  void drawImageNine(ui.Image image, Rect center, Rect dst, Paint paint) {
    final xs = [0.0, center.left, center.right, image.width.toDouble()];
    final ys = [0.0, center.top, center.bottom, image.height.toDouble()];
    // the corners keep their size, unless dst is smaller than both of them
    final left = math.min(center.left, dst.width * center.left / (center.left + image.width - center.right));
    final right = math.min(image.width - center.right, dst.width - left);
    final top = math.min(center.top, dst.height * center.top / (center.top + image.height - center.bottom));
    final bottom = math.min(image.height - center.bottom, dst.height - top);
    final dx = [dst.left, dst.left + left, dst.right - right, dst.right];
    final dy = [dst.top, dst.top + top, dst.bottom - bottom, dst.bottom];
    for (var row = 0; row < 3; row++) {
      for (var column = 0; column < 3; column++) {
        drawImageRect(image, Rect.fromLTRB(xs[column], ys[row], xs[column + 1], ys[row + 1]), Rect.fromLTRB(dx[column], dy[row], dx[column + 1], dy[row + 1]), paint);
      }
    }
  }

  // what a PDF cannot be told of

  @override
  void drawPicture(ui.Picture picture) => _export.ignored.add(Ignored.picture);

  @override
  void drawParagraph(ui.Paragraph paragraph, Offset offset) => _export.ignored.add(Ignored.paragraph);

  @override
  void drawVertices(ui.Vertices vertices, BlendMode blendMode, Paint paint) => _export.ignored.add(Ignored.vertices);

  @override
  void drawAtlas(ui.Image atlas, List<ui.RSTransform> transforms, List<Rect> rects, List<Color>? colors, BlendMode? blendMode, Rect? cullRect, Paint paint) => _export.ignored.add(Ignored.atlas);

  @override
  void drawRawAtlas(ui.Image atlas, Float32List rstTransforms, Float32List rects, Int32List? colors, BlendMode? blendMode, Rect? cullRect, Paint paint) => _export.ignored.add(Ignored.atlas);

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
