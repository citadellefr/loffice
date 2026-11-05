import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/drawing/color.dart';
import 'package:loffice/src/drawing/paint.dart';
import 'package:loffice/src/pdf/canvas.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';

/// What the canvas of an export writes, read back from the file. With
/// $LOFFICE_PDF set, the files are written there.
void main() {
  setUpAll(() async {
    await OfficeFonts.register({
      for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf'))) f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
    });
  });

  const size = Size(200, 100);
  var written = 0;

  Future<(_Pdf, PdfExport)> exported(void Function(Canvas canvas) paint) async {
    final export = PdfExport();
    await export.page(size, paint);
    final bytes = export.save();
    final out = Platform.environment['LOFFICE_PDF'];
    if (out != null) File('$out/canvas-${++written}.pdf').writeAsBytesSync(bytes);
    final pdf = _Pdf(bytes);
    pdf.expectValid();
    return (pdf, export);
  }

  const painter = DrawingPainter(ColorContext());
  const red = {'rgb': 'FF0000'}, white = {'rgb': 'FFFFFF'};

  test('a pattern fill is a tiling pattern, placed where the shape is', () async {
    final (pdf, export) = await exported((canvas) {
      canvas.translate(10, 20);
      painter.fill(canvas, Path()..addRect(const Rect.fromLTWH(0, 0, 50, 30)), const Rect.fromLTWH(0, 0, 50, 30), {
        'patt': {'prst': 'horz', 'fg': red, 'bg': white},
      });
    });
    expect(export.ignored, isEmpty);
    final pattern = pdf.patterns.single;
    expect(pattern.dictionary, contains('/PatternType 1'));
    expect(pattern.dictionary, contains('/XStep 8 /YStep 8'));
    // eight pixels, a pixel of 0.75 point, below the translation and the flip of the page
    expect(pattern.dictionary, contains('/Matrix [0.75 0 0 -0.75 10 80]'));
    expect(pattern.data, '1 1 1 rg 0 0 8 8 re f 1 0 0 rg 0 0 8 1 re 0 4 8 1 re f');
    final content = pdf.content;
    expect(content, contains('/Pattern cs /P1 scn'));
    expect(content, contains('0 0 m 50 0 l 50 30 l 0 30 l h f'));
    expect(pdf.resources, contains('/Pattern << /P1 ${pattern.id} 0 R >>'));
  });

  test('a pattern fill keeps the pixels of its preset, and a transparent background', () async {
    final (pdf, _) = await exported((canvas) {
      painter.fill(canvas, Path()..addRect(Offset.zero & size), Offset.zero & size, {
        'patt': {
          'prst': 'dnDiag',
          'fg': red,
          'bg': {
            'rgb': 'FFFFFF',
            'mods': [
              ['alpha', 0],
            ],
          },
        },
      });
    });
    expect(pdf.patterns.single.data, startsWith('1 0 0 rg 0 0 1 1 re 4 0 1 1 re 1 1 1 1 re 5 1 1 1 re'));
  });

  test('the same pattern is written once, however many shapes use it', () async {
    final (pdf, _) = await exported((canvas) {
      for (var i = 0; i < 3; i++) {
        painter.fill(canvas, Path()..addRect(Rect.fromLTWH(i * 20.0, 0, 10, 10)), Rect.fromLTWH(i * 20.0, 0, 10, 10), {
          'patt': {'prst': 'pct50', 'fg': red, 'bg': white},
        });
      }
    });
    expect(pdf.patterns, hasLength(1));
    expect('/Pattern cs /P1 scn'.allMatches(pdf.content), hasLength(1));
  });

  Future<ui.Image> picture(int width, int height, Color Function(int x, int y) color) async {
    final bytes = ByteData(4 * width * height);
    for (var y = 0; y < height; y++) {
      for (var x = 0; x < width; x++) {
        final c = color(x, y);
        bytes.setUint32(4 * (y * width + x), (c.toARGB32() << 8 | c.toARGB32() >> 24) & 0xFFFFFFFF);
      }
    }
    final codec = await ui.ImageDescriptor.raw(
      await ui.ImmutableBuffer.fromUint8List(bytes.buffer.asUint8List()),
      width: width,
      height: height,
      pixelFormat: ui.PixelFormat.rgba8888,
    ).instantiateCodec();
    return (await codec.getNextFrame()).image;
  }

  test('a tiled picture is a tiling pattern that shows the picture', () async {
    final image = await picture(4, 2, (x, y) => const Color(0xFF3366CC));
    final (pdf, export) = await exported((canvas) {
      DrawingPainter(const ColorContext(), images: (_) => image).fill(canvas, Path()..addRect(Offset.zero & size), Offset.zero & size, {
        'blip': {'media': 'tile.png', 'tile': true},
      });
    });
    expect(export.ignored, isEmpty);
    final pattern = pdf.patterns.single;
    expect(pattern.dictionary, contains('/XStep 4 /YStep 2'));
    expect(pattern.dictionary, contains('/Matrix [1 0 0 -1 0 100]'));
    expect(pattern.dictionary, contains('/XObject << /Im1 ${pdf.images.single.id} 0 R >>'));
    expect(pattern.data, '4 0 0 -2 0 2 cm /Im1 Do');
    expect(pdf.content, contains('/Pattern cs /P1 scn'));
    expect(pdf.images.single.dictionary, contains('/Width 4 /Height 2'));
  });

  test('a transparent PNG is written as its colors and, as a soft mask, its opacity', () async {
    final image = await _decode(
      _png(3, 2, 8, 6, [
        for (var y = 0; y < 2; y++)
          [
            for (var x = 0; x < 3; x++) ...[10 * x, 20 * y, 30, x == 1 ? 0 : 255 - 50 * y],
          ],
      ]),
    );
    final (pdf, _) = await exported((canvas) => canvas.drawImageRect(image, Offset.zero & const Size(3, 2), const Rect.fromLTWH(10, 10, 60, 40), Paint()));
    final rgb = pdf.images.firstWhere((i) => i.dictionary.contains('/DeviceRGB'));
    expect(rgb.dictionary, contains('/Filter /FlateDecode'));
    // the engine has the colors of what is clear as black
    expect(rgb.data.codeUnits, [
      for (var y = 0; y < 2; y++)
        for (var x = 0; x < 3; x++) ...(x == 1 ? [0, 0, 0] : [10 * x, 20 * y, 30]),
    ]);
    final mask = pdf.object(int.parse(RegExp(r'/SMask (\d+) 0 R').firstMatch(rgb.dictionary)!.group(1)!));
    expect(mask.dictionary, contains('/DeviceGray'));
    expect(mask.data.codeUnits, [255, 0, 255, 205, 0, 205]);
  });

  test('an opaque PNG is embedded as it is, its rows still filtered', () async {
    final rows = [
      for (var y = 0; y < 2; y++)
        [
          for (var x = 0; x < 3; x++) ...[10 * x, 20 * y, 30],
        ],
    ];
    final image = await _decode(_png(3, 2, 8, 2, rows));
    final (pdf, _) = await exported((canvas) => canvas.drawImageRect(image, Offset.zero & const Size(3, 2), Offset.zero & size, Paint()));
    final embedded = pdf.images.single;
    expect(embedded.dictionary, contains('/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /DecodeParms << /Predictor 15 /Colors 3 /BitsPerComponent 8 /Columns 3 >>'));
    expect(embedded.data.codeUnits, [
      for (final row in rows) ...[0, ...row],
    ]);
  });

  test('a PNG of palette colors is embedded with its palette, unless a color is transparent', () async {
    final palette = [255, 0, 0, 0, 255, 0, 0, 0, 255];
    final rows = [
      [0, 1, 2, 1],
      [2, 1, 0, 1],
    ];
    final image = await _decode(_png(4, 2, 8, 3, rows, palette: palette));
    final (pdf, _) = await exported((canvas) => canvas.drawImageRect(image, Offset.zero & const Size(4, 2), Offset.zero & size, Paint()));
    expect(pdf.images.single.dictionary, contains('/ColorSpace [/Indexed /DeviceRGB 2 <ff000000ff000000ff>] /BitsPerComponent 8'));
    expect(pdf.images.single.dictionary, contains('/Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns 4'));

    final clear = await _decode(_png(4, 2, 8, 3, rows, palette: palette, transparent: [0, 255, 255]));
    final (other, _) = await exported((canvas) => canvas.drawImageRect(clear, Offset.zero & const Size(4, 2), Offset.zero & size, Paint()));
    expect(other.images.map((i) => i.dictionary).join(), contains('/SMask'));
    expect(other.images.map((i) => i.dictionary).join(), isNot(contains('/Predictor')));
  });

  /// The opacities a page sets, in order, as the file has them.
  List<double> opacities(_Pdf pdf) {
    final states = {for (final m in RegExp(r'/(G\d+) << /ca ([\d.]+)').allMatches(pdf.resources)) m.group(1)!: double.parse(m.group(2)!)};
    return [for (final m in RegExp(r'/(G\d+) gs').allMatches(pdf.content)) states[m.group(1)!]!];
  }

  /// The opacity the layers of a shadow come to, laid over each other.
  double total(List<double> layers) => 1 - layers.fold(1.0, (a, b) => a * (1 - b));

  test('a shadow is the shape lowered, in layers that fade out', () async {
    final (pdf, export) = await exported((canvas) => canvas.drawShadow(Path()..addRect(const Rect.fromLTWH(20, 20, 60, 30)), const Color(0xFF000000), 8, false));
    expect(export.ignored, isEmpty);
    expect(pdf.content, contains('1 0 0 1 0 4 cm'));
    final layers = opacities(pdf);
    expect(layers, hasLength(9));
    expect(layers.first, lessThan(layers.last));
    expect(total(layers), closeTo(0.3, 0.01));
    // the widest layer reaches three deviations out
    expect(pdf.content, contains('24 w'));
    expect(' h B\n'.allMatches(pdf.content), hasLength(8));
    expect(' h f\n'.allMatches(pdf.content), hasLength(1));
  });

  test('the outer shadow of a slide shape is written under it', () async {
    final edit = Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!;
    final deck = Deck(Tree.fromEdit(edit)!);
    final shape = deck.tree['s257-9']!;
    Future<_Pdf> slide() async => (await exported((canvas) => SlidePainter(deck).paint(canvas, deck.slides[1]))).$1;
    final plain = opacities(await slide());
    expect(plain, hasLength(1));
    deck.tree.apply(
      Edit([
        Change.set(
          shape.id,
          attributes: {
            'shadow': {
              'color': {
                'rgb': '000000',
                'mods': [
                  ['alpha', 40000],
                ],
              },
              'blur': 50800,
              'dist': 38100,
              'dir': 2700000,
            },
          },
        ),
      ]),
    );
    final (pdf, export) = await exported((canvas) => SlidePainter(deck).paint(canvas, deck.slides[1]));
    expect(export.ignored, isEmpty);
    final all = opacities(pdf);
    expect(all, hasLength(11));
    expect(total(all.sublist(1, 10)), closeTo(0.4, 0.01));
  });

  test('a blurred shape is written as the shape and its blur', () async {
    final (pdf, export) = await exported((canvas) {
      canvas.drawRRect(
        RRect.fromRectAndRadius(const Rect.fromLTWH(20, 20, 60, 30), const Radius.circular(5)),
        Paint()
          ..color = const Color(0x80FF0000)
          ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 2),
      );
    });
    expect(export.ignored, isEmpty);
    expect(total(opacities(pdf)), closeTo(0x80 / 255, 0.01));
    expect(pdf.content, contains('1 0 0 rg'));
    expect(pdf.content, contains('6 w'));
  });

  test('a blurred line is widened by its blur', () async {
    final (pdf, _) = await exported((canvas) {
      canvas.drawLine(
        const Offset(10, 10),
        const Offset(90, 10),
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2
          ..color = const Color(0xFF000000)
          ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 1),
      );
    });
    expect(pdf.content, contains('8 w'));
    expect(pdf.content, contains('2 w'));
    expect(total(opacities(pdf)), closeTo(1, 0.01));
  });

  test('no method of a canvas fails the export: what is left aside is told', () async {
    final image = await picture(4, 4, (x, y) => const Color(0xFF008000));
    final recorder = ui.PictureRecorder();
    ui.Canvas(recorder).drawRect(Offset.zero & size, Paint());
    final recorded = recorder.endRecording();
    final builder = ui.ParagraphBuilder(ui.ParagraphStyle())..addText('text');
    final paragraph = builder.build()..layout(const ui.ParagraphConstraints(width: 100));
    final paint = Paint()..color = const Color(0xFF0000FF);
    const box = Rect.fromLTWH(10, 10, 40, 20);
    final (pdf, export) = await exported((canvas) {
      canvas.save();
      canvas.translate(10, 20);
      canvas.scale(2);
      expect(canvas.getTransform()[0], 2);
      expect(canvas.getTransform().sublist(12, 14), [10, 20]);
      expect(canvas.getDestinationClipBounds(), Offset.zero & size);
      expect(canvas.getLocalClipBounds(), const Rect.fromLTRB(-5, -10, 95, 40));
      canvas.rotate(0.1);
      canvas.skew(0.1, 0);
      canvas.transform(Float64List.fromList([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]));
      canvas.clipRect(box);
      canvas.clipRRect(RRect.fromRectAndRadius(box, const Radius.circular(3)));
      canvas.clipRSuperellipse(ui.RSuperellipse.fromRectAndRadius(box, const Radius.circular(3)));
      canvas.clipPath(Path()..addOval(box));
      canvas.drawColor(const Color(0x20000000), BlendMode.srcOver);
      canvas.drawPaint(paint);
      canvas.drawLine(box.topLeft, box.bottomRight, paint);
      canvas.drawRect(box, paint);
      canvas.drawRRect(RRect.fromRectAndRadius(box, const Radius.circular(3)), paint);
      canvas.drawDRRect(RRect.fromRectAndRadius(box, const Radius.circular(8)), RRect.fromRectAndRadius(box.deflate(4), const Radius.circular(4)), paint);
      canvas.drawRSuperellipse(ui.RSuperellipse.fromRectAndRadius(box, const Radius.circular(3)), paint);
      canvas.drawOval(box, paint);
      canvas.drawCircle(box.center, 5, paint);
      canvas.drawArc(box, 0, 2, true, paint);
      canvas.drawPath(Path()..addOval(box), paint);
      canvas.drawImage(image, const Offset(60, 10), paint);
      canvas.drawImageRect(image, const Rect.fromLTWH(0, 0, 2, 2), box, paint);
      canvas.drawImageNine(image, const Rect.fromLTRB(1, 1, 3, 3), box, paint);
      canvas.drawPoints(ui.PointMode.points, [box.topLeft, box.center], paint..strokeWidth = 3);
      canvas.drawPoints(ui.PointMode.lines, [box.topLeft, box.center, box.bottomRight, box.bottomLeft], paint);
      canvas.drawRawPoints(ui.PointMode.polygon, Float32List.fromList([0, 0, 10, 10, 20, 0]), paint);
      canvas.drawShadow(Path()..addOval(box), const Color(0xFF000000), 4, true);
      canvas.saveLayer(box, Paint()..color = const Color(0x80000000));
      canvas.drawRect(box, paint);
      canvas.restore();
      expect(canvas.getSaveCount(), 2);
      canvas.save();
      canvas.restoreToCount(1);
      expect(canvas.getSaveCount(), 1);
      canvas.drawPicture(recorded);
      canvas.drawParagraph(paragraph, Offset.zero);
      canvas.drawVertices(ui.Vertices(ui.VertexMode.triangles, [box.topLeft, box.topRight, box.bottomRight]), BlendMode.srcOver, paint);
      canvas.drawAtlas(image, [ui.RSTransform.fromComponents(rotation: 0, scale: 1, anchorX: 0, anchorY: 0, translateX: 0, translateY: 0)], [const Rect.fromLTWH(0, 0, 2, 2)], null, null, null, paint);
      canvas.drawRawAtlas(image, Float32List.fromList([1, 0, 0, 0]), Float32List.fromList([0, 0, 2, 2]), null, null, null, paint);
      canvas.drawColor(const Color(0x20000000), BlendMode.multiply);
      canvas.drawRect(box, Paint()..colorFilter = const ColorFilter.mode(Color(0xFF000000), BlendMode.srcIn));
      canvas.drawRect(box, Paint()..blendMode = BlendMode.multiply);
    });
    expect(export.ignored, {
      'a picture of the engine',
      'a paragraph of the engine',
      'vertices',
      'an atlas',
      'a color in blend mode multiply',
      'a shape with a color filter',
      'a shape in blend mode multiply',
    });
    expect(pdf.content, contains('f*'));
    expect(RegExp(r'/Im\d+ Do').allMatches(pdf.content), hasLength(11));
  });
}

/// An object of a file.
class _Object {
  _Object(this.id, this.dictionary, this.stream);

  final int id;
  final String dictionary;
  final String? stream;

  /// What the stream holds, as characters, once decoded.
  String get data => dictionary.contains('/Filter /FlateDecode') ? latin1.decode(zlib.decode(latin1.encode(stream!))) : stream!;
}

/// A PDF read back, and checked for what a reader needs: the table of its
/// objects, their lengths, the streams that decode, the references that
/// lead somewhere and the states a page saves and restores.
class _Pdf {
  _Pdf(Uint8List bytes) : text = latin1.decode(bytes) {
    final start = int.parse(RegExp(r'startxref\n(\d+)\n%%EOF\n$').firstMatch(text)!.group(1)!);
    expect(text.substring(start, start + 4), 'xref');
    final offsets = [for (final m in RegExp(r'(\d{10}) 00000 n \n').allMatches(text, start)) int.parse(m.group(1)!)];
    expect(text, contains('/Size ${offsets.length + 1}'));
    for (final (i, offset) in offsets.indexed) {
      final end = i + 1 < offsets.length ? offsets[i + 1] : start;
      final header = '${i + 1} 0 obj\n';
      expect(text.substring(offset, offset + header.length), header);
      final body = text.substring(offset + header.length, end);
      final stream = body.indexOf('\nstream\n');
      if (stream < 0) {
        expect(body.endsWith('\nendobj\n'), isTrue);
        objects.add(_Object(i + 1, body.substring(0, body.length - 8), null));
      } else {
        expect(body.endsWith('\nendstream\nendobj\n'), isTrue);
        final data = body.substring(stream + 8, body.length - 18);
        final dictionary = body.substring(0, stream);
        expect(int.parse(RegExp(r'/Length (\d+)').firstMatch(dictionary)!.group(1)!), data.length);
        objects.add(_Object(i + 1, dictionary.substring(3, dictionary.length - 2), data));
      }
    }
  }

  final String text;
  final objects = <_Object>[];

  _Object object(int id) => objects[id - 1];

  void expectValid() {
    expect(text, startsWith('%PDF-1.7\n'));
    for (final o in objects) {
      for (final m in RegExp(r'(\d+) 0 R').allMatches(o.dictionary)) {
        expect(int.parse(m.group(1)!), inInclusiveRange(1, objects.length), reason: o.dictionary);
      }
      if (o.stream != null && o.dictionary.contains('/FlateDecode')) expect(() => o.data, returnsNormally);
    }
    final saved = RegExp(r'(?<!\S)q(?!\S)').allMatches(content).length, restored = RegExp(r'(?<!\S)Q(?!\S)').allMatches(content).length;
    expect(saved, restored);
  }

  Iterable<_Object> get patterns => objects.where((o) => o.dictionary.contains('/PatternType 1'));
  Iterable<_Object> get images => objects.where((o) => o.dictionary.contains('/Subtype /Image') && !o.dictionary.contains('/DeviceGray'));
  _Object get _page => objects.firstWhere((o) => o.dictionary.contains('/Type /Page '));
  String get resources => object(int.parse(RegExp(r'/Resources (\d+) 0 R').firstMatch(_page.dictionary)!.group(1)!)).dictionary;
  String get content => object(int.parse(RegExp(r'/Contents (\d+) 0 R').firstMatch(_page.dictionary)!.group(1)!)).data;
}

/// A PNG file of [rows] of bytes, as the engine decodes it too.
Uint8List _png(int width, int height, int depth, int type, List<List<int>> rows, {List<int>? palette, List<int>? transparent}) {
  final out = BytesBuilder();
  void chunk(String name, List<int> data) {
    final body = [...name.codeUnits, ...data];
    final length = ByteData(4)..setUint32(0, data.length);
    final crc = ByteData(4)..setUint32(0, _crc(body));
    out
      ..add(length.buffer.asUint8List())
      ..add(body)
      ..add(crc.buffer.asUint8List());
  }

  out.add([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
  final header = ByteData(13)
    ..setUint32(0, width)
    ..setUint32(4, height)
    ..setUint8(8, depth)
    ..setUint8(9, type);
  chunk('IHDR', header.buffer.asUint8List());
  if (palette != null) chunk('PLTE', palette);
  if (transparent != null) chunk('tRNS', transparent);
  // each row starts with the filter that means none
  chunk(
    'IDAT',
    zlib.encode([
      for (final row in rows) ...[0, ...row],
    ]),
  );
  chunk('IEND', const []);
  return out.takeBytes();
}

int _crc(List<int> bytes) {
  var crc = 0xFFFFFFFF;
  for (final b in bytes) {
    crc ^= b;
    for (var i = 0; i < 8; i++) {
      crc = crc & 1 != 0 ? (crc >> 1) ^ 0xEDB88320 : crc >> 1;
    }
  }
  return crc ^ 0xFFFFFFFF;
}

/// The picture of a file, which an export finds the file of.
Future<ui.Image> _decode(Uint8List file) async {
  final frame = await (await ui.instantiateImageCodec(file)).getNextFrame();
  pictureFiles[frame.image] = file;
  return frame.image;
}
