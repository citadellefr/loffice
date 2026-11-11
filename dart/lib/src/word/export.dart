import 'dart:typed_data';

import '../pdf/canvas.dart';
import 'layout.dart';
import 'page_painter.dart';

/// The pages of a document as a PDF, drawn as [painter] draws them on
/// screen, written with [export] when it is given, to tell afterwards what
/// it left aside.
Future<Uint8List> wordPdf(WordLayout layout, PagePainter painter, {String title = '', PdfExport? export}) async {
  export ??= PdfExport();
  for (final page in layout.pages) {
    await export.page(page.size, (canvas) => painter.paint(canvas, page));
  }
  return export.save(title: title);
}
