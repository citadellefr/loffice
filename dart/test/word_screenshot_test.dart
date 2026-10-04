import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:trame/testing.dart';

import 'fonts.dart';

/// The Word editor on a fixture of testdata/docx, drawn into
/// $LOFFICE_SCREENSHOT: a look at the interface, not a test.
void main() {
  final out = Platform.environment['LOFFICE_SCREENSHOT'];
  testWidgets('draws the editor', skip: out == null, (tester) async {
    await tester.runAsync(loadFonts);
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/${Platform.environment['LOFFICE_FIXTURE'] ?? 'tbl-having-applied-style'}.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(
      theme: ThemeData(fontFamily: 'Carlito'),
      home: Scaffold(body: RepaintBoundary(child: WordEditor(session: session, media: (_) async => Uint8List(0), title: 'Document'))),
    ));
    await tester.runAsync(hub.settle);
    await tester.pump();
    final boundary = tester.renderObject<RenderRepaintBoundary>(find.byType(RepaintBoundary).first);
    final image = await tester.runAsync(() => boundary.toImage());
    final png = await tester.runAsync(() => image!.toByteData(format: ui.ImageByteFormat.png));
    File(out!).writeAsBytesSync(png!.buffer.asUint8List());
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  });
}
