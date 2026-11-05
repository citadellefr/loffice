import 'dart:typed_data';

import '../pdf/canvas.dart';
import 'deck.dart';
import 'slide_painter.dart';

/// The slides of a deck as a PDF, one a page, drawn as [painter] draws
/// them on screen. [missing] is told the characters no font of the export
/// has, [ignored] what the export leaves aside of what the painter asked.
Future<Uint8List> deckPdf(Deck deck, SlidePainter painter, {String title = '', void Function(Set<String>)? missing, void Function(Set<Ignored>)? ignored}) async {
  final export = PdfExport();
  for (final slide in deck.slides) {
    await export.page(deck.size, (canvas) => painter.paint(canvas, slide));
  }
  missing?.call(export.missing);
  ignored?.call(export.ignored);
  return export.save(title: title);
}
