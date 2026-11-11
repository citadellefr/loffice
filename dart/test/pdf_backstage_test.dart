import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/export_notice.dart';
import 'package:loffice/src/pdf/canvas.dart' show Ignored, PdfExport;
import 'package:trame/testing.dart';

Future<void> registerFonts() => OfficeFonts.register({
  for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf'))) f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
});

void main() {
  test('the notice of an export names what was left aside', () {
    const strings = LofficeStrings();
    expect(strings.exportNotice({}, {}), isNull);
    final notice = strings.exportNotice({'ß'}, {Ignored.blendMode, Ignored.colorFilter, Ignored.vertices})!;
    expect(notice, contains('ß'));
    expect(notice, contains('L’export en PDF a laissé de côté : des filtres de couleur, des modes de fusion, des formes en relief'));
    expect(notice, isNot(contains('Ignored')));
  });

  test('every kind of element left aside has its own words', () {
    const strings = LofficeStrings();
    final labels = [for (final i in Ignored.values) strings.ignoredLabel(i)];
    expect(labels.toSet().length, labels.length);
    for (final label in labels) {
      expect(labels.where((l) => l == label).length, lessThanOrEqualTo(2));
    }
    expect(strings.ignoredLabel(Ignored.maskFilter), 'des effets de flou');
  });

  test('the notice never says the same thing twice', () {
    const strings = LofficeStrings();
    final notice = strings.exportNotice({'ß'}, {...Ignored.values})!;
    for (final i in Ignored.values) {
      expect(RegExp(RegExp.escape(strings.ignoredLabel(i))).allMatches(notice).length, 1);
    }
    expect(strings.exportNotice({}, {}), isNull);
  });

  testWidgets('what an export left aside is told in a snack bar', (tester) async {
    await tester.runAsync(registerFonts);
    final export = PdfExport();
    await tester.runAsync(
      () => export.page(const Size(100, 100), (canvas) {
        canvas.drawVertices(ui.Vertices(VertexMode.triangles, const [Offset.zero, Offset(10, 0), Offset(0, 10)]), BlendMode.srcOver, Paint());
        canvas.drawRect(const Rect.fromLTWH(0, 0, 10, 10), Paint()..colorFilter = const ColorFilter.mode(Color(0xFF000000), BlendMode.srcIn));
      }),
    );
    expect(export.ignored, {Ignored.vertices, Ignored.colorFilter});
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Builder(builder: (context) => TextButton(onPressed: () => showExportNotice(ScaffoldMessenger.maybeOf(context), const LofficeStrings(), export), child: const Text('go'))),
        ),
      ),
    );
    await tester.tap(find.text('go'));
    await tester.pump();
    expect(find.textContaining('L’export en PDF a laissé de côté : des filtres de couleur, des formes en relief'), findsOneWidget);
  });

  testWidgets('File > Export as PDF hands the host the document, named after it', (tester) async {
    await tester.runAsync(registerFonts);
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    Uint8List? exported;
    String? name;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: WordEditor(
            session: session,
            media: (_) async => Uint8List(0),
            title: 'Rapport annuel.docx',
            onExport: (pdf, file) async {
              exported = pdf;
              name = file;
            },
          ),
        ),
      ),
    );
    await tester.runAsync(hub.settle);
    await tester.pump();

    await tester.tap(find.text('Fichier'));
    await tester.pump();
    await tester.tap(find.text('Exporter en PDF'));
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 500)));
    await tester.pump();

    expect(name, 'Rapport annuel.pdf');
    expect(latin1.decode(exported!.sublist(0, 8)), '%PDF-1.7');
    expect(find.text('Exporter en PDF'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  });

  testWidgets('a workbook is exported on the pages it prints on', (tester) async {
    await tester.runAsync(registerFonts);
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/xlsx/49156.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    Uint8List? exported;
    String? name;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SpreadsheetEditor(
            session: session,
            title: 'Budget.xlsx',
            onExport: (pdf, file) async {
              exported = pdf;
              name = file;
            },
          ),
        ),
      ),
    );
    await tester.runAsync(hub.settle);
    await tester.pump();

    await tester.tap(find.text('Fichier'));
    await tester.pump();
    await tester.tap(find.text('Exporter en PDF'));
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 500)));
    await tester.pump();

    expect(name, 'Budget.pdf');
    expect(RegExp(r'/Type /Page ').allMatches(latin1.decode(exported!)).length, 2);
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  });
}
