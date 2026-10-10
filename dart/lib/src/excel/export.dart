import 'dart:typed_data';

import '../pdf/canvas.dart';
import 'number_format.dart';
import 'page_painter.dart';
import 'workbook.dart';

/// A workbook none of whose sheets has anything to print.
class NothingToPrint implements Exception {
  const NothingToPrint();
}

/// The sheets of a workbook as a PDF, on the pages they print on.
/// [missing] is told the characters no font of the export has.
Future<Uint8List> workbookPdf(
  Workbook book, {
  NumberLocale locale = NumberLocale.fr,
  String? fonts,
  String title = '',
  DateTime? now,
  void Function(Set<String>)? missing,
}) async {
  final pages = workbookPages(book, locale: locale, fonts: fonts, title: title, now: now);
  if (pages.isEmpty) throw const NothingToPrint();
  final export = PdfExport();
  for (final page in pages) {
    await export.page(page.size, page.paint);
  }
  missing?.call(export.missing);
  return export.save(title: title);
}
