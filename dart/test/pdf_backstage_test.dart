import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:trame/testing.dart';

void main() {
  testWidgets('File > Export as PDF hands the host the document, named after it', (tester) async {
    await tester.runAsync(() => OfficeFonts.register({
      for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf')))
        f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
    }));
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    Uint8List? exported;
    String? name;
    await tester.pumpWidget(MaterialApp(
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
    ));
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
    await tester.runAsync(() => OfficeFonts.register({
      for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf')))
        f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
    }));
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/xlsx/49156.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    Uint8List? exported;
    String? name;
    await tester.pumpWidget(MaterialApp(
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
    ));
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
