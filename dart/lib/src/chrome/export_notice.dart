import 'package:flutter/material.dart';

import '../pdf/canvas.dart' show PdfExport;
import 'strings.dart';

/// Tells in a snack bar what a finished [export] left aside, if anything.
void showExportNotice(ScaffoldMessengerState? messenger, LofficeStrings strings, PdfExport export) {
  if (strings.exportNotice(export.missing, export.ignored) case final notice?) messenger?.showSnackBar(SnackBar(content: Text(notice)));
}
