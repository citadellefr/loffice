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
/// Written with [export] when it is given, to tell afterwards what it left
/// aside.
Future<Uint8List> workbookPdf(
  Workbook book, {
  NumberLocale locale = NumberLocale.fr,
  String? fonts,
  String title = '',
  DateTime? now,
  PdfExport? export,
}) async {
  final pages = workbookPages(book, locale: locale, fonts: fonts, title: title, now: now);
  if (pages.isEmpty) throw const NothingToPrint();
  export ??= PdfExport();
  for (final page in pages) {
    await export.page(page.size, page.paint);
  }
  return export.save(title: title);
}
