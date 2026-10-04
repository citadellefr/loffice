import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/filter.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/sheet_view.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/trame.dart';

import 'fonts.dart';

/// Draws the sheets of the workbooks in \$LOFFICE_SHEETS, as the Go package
/// dumps them, into PNG files beside them: a look at the rendering, not a
/// test.
void main() {
  final dir = Platform.environment['LOFFICE_SHEETS'];
  testWidgets('renders sheets', (tester) async {
    await tester.runAsync(loadFonts);
    tester.view.physicalSize = const Size(1200, 700);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    for (final file in Directory(dir!).listSync().whereType<File>().where((f) => f.path.endsWith('.json'))) {
      final base = file.path.substring(0, file.path.length - 5);
      final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(file.readAsStringSync()))!)!;
      final book = Workbook(tree);
      final sheets = [for (final s in book.sheets) if (s.grid != null) s];
      for (var i = 0; i < sheets.length && i < 4; i++) {
        final key = GlobalKey();
        await tester.pumpWidget(MaterialApp(
          home: RepaintBoundary(
            key: key,
            child: SheetView(book: book, sheet: sheets[i], selection: SheetSelection(), locale: NumberLocale.fr, filter: SheetFilter.of(sheets[i])),
          ),
        ));
        await tester.pump();
        await tester.runAsync(() async {
          final boundary = key.currentContext!.findRenderObject()! as RenderRepaintBoundary;
          final image = await boundary.toImage();
          final png = await image.toByteData(format: ui.ImageByteFormat.png);
          File('$base-${i + 1}.png').writeAsBytesSync(png!.buffer.asUint8List());
        });
      }
    }
  }, skip: dir == null);
}
