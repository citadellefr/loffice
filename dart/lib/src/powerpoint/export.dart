import 'dart:typed_data';

import '../pdf/canvas.dart';
import 'deck.dart';
import 'slide_painter.dart';

/// The slides of a deck as a PDF, one a page, drawn as [painter] draws
/// them on screen, written with [export] when it is given, to tell afterwards
/// what it left aside.
Future<Uint8List> deckPdf(Deck deck, SlidePainter painter, {String title = '', PdfExport? export}) async {
  export ??= PdfExport();
  for (final slide in deck.slides) {
    await export.page(deck.size, (canvas) => painter.paint(canvas, slide));
  }
  return export.save(title: title);
}
