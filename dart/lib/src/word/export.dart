import 'dart:typed_data';

import '../pdf/canvas.dart';
import 'layout.dart';
import 'page_painter.dart';

/// The pages of a document as a PDF, drawn as [painter] draws them on
/// screen. [missing] is told the characters no font of the export has,
/// [ignored] what the export leaves aside of what the painter asked.
Future<Uint8List> wordPdf(WordLayout layout, PagePainter painter, {String title = '', void Function(Set<String>)? missing, void Function(Set<Ignored>)? ignored}) async {
  final export = PdfExport();
  for (final page in layout.pages) {
    await export.page(page.size, (canvas) => painter.paint(canvas, page));
  }
  missing?.call(export.missing);
  ignored?.call(export.ignored);
  return export.save(title: title);
}
