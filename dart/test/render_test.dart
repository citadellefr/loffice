import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:trame/trame.dart';

import 'fonts.dart';

/// Draws the slides of the trees in $LOFFICE_RENDER, as the Go package dumps
/// them, into PNG files beside them, with the fonts found in $LOFFICE_FONTS:
/// a look at the rendering, not a test.
void main() {
  final dir = Platform.environment['LOFFICE_RENDER'];
  test('renders slides', () async {
    await loadFonts();
    for (final file in Directory(dir!).listSync().whereType<File>().where((f) => f.path.endsWith('.json'))) {
      final base = file.path.substring(0, file.path.length - 5);
      final deck = Deck(Tree.fromEdit(Edit.fromJson(jsonDecode(file.readAsStringSync()))!)!);
      final images = <String, ui.Image>{};
      final media = Directory(base);
      if (media.existsSync()) {
        for (final m in media.listSync().whereType<File>()) {
          try {
            final codec = await ui.instantiateImageCodec(m.readAsBytesSync());
            images[m.uri.pathSegments.last] = (await codec.getNextFrame()).image;
          } on Object {
            // a format the engine does not read
          }
        }
      }
      final painter = SlidePainter(deck, images: (m) => images[m]);
      Future<void> save(ui.Picture picture, Size size, String path) async {
        final image = await picture.toImage(size.width.ceil(), size.height.ceil());
        final png = await image.toByteData(format: ui.ImageByteFormat.png);
        File(path).writeAsBytesSync(png!.buffer.asUint8List());
      }

      final slides = deck.slides;
      for (var i = 0; i < slides.length; i++) {
        const scale = 1.5;
        final recorder = ui.PictureRecorder();
        painter.paint(ui.Canvas(recorder)..scale(scale), slides[i]);
        await save(recorder.endRecording(), deck.size * scale, '$base-${i + 1}.png');
      }
      // contact sheets of twelve slides, four by three
      const scale = 0.4, gap = 8.0;
      final cell = deck.size * scale;
      for (var first = 0; first < slides.length; first += 12) {
        final recorder = ui.PictureRecorder();
        final canvas = ui.Canvas(recorder);
        final sheet = Size(4 * (cell.width + gap) + gap, 3 * (cell.height + gap) + gap);
        canvas.drawRect(Offset.zero & sheet, Paint()..color = const Color(0xFF808080));
        for (var i = first; i < slides.length && i < first + 12; i++) {
          final k = i - first;
          canvas.save();
          canvas.translate(gap + (k % 4) * (cell.width + gap), gap + (k ~/ 4) * (cell.height + gap));
          canvas.scale(scale);
          painter.paint(canvas, slides[i]);
          canvas.restore();
        }
        await save(recorder.endRecording(), sheet, '$base-sheet-${first ~/ 12 + 1}.png');
      }
    }
  }, skip: dir == null ? 'LOFFICE_RENDER not set' : false);
}
