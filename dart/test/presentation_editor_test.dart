import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:trame/testing.dart';

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> open(WidgetTester tester, String fixture, {Size size = const Size(1400, 900)}) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/pptx/$fixture.json').readAsStringSync()))!);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: PresentationEditor(session: session, media: (_) async => Uint8List(0), title: '$fixture.pptx')),
    ));
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  List<Node> slides() => [for (final n in hub.doc.children('deck')) if (n.type == 'slide') n];

  testWidgets('shows the ribbon, the slides and the status', (tester) async {
    await open(tester, 'shp-shapes');
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Insertion'), findsOneWidget);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('fits a phone', (tester) async {
    await open(tester, 'shp-shapes', size: const Size(390, 844));
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('adds, duplicates and deletes slides', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.byTooltip('Dupliquer la diapositive'));
    await settle(tester);
    expect(slides(), hasLength(3));
    expect(find.text('Diapositive 2 sur 3'), findsOneWidget);
    expect(hub.doc.children(slides()[1].id).length, hub.doc.children(slides()[0].id).where((n) => n.type != 'notes').length);

    await tester.tap(find.byTooltip('Supprimer la diapositive'));
    await settle(tester);
    expect(slides(), hasLength(2));

    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyM);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(slides(), hasLength(3));
    expect(slides()[1].attributes['layout'], 'L1');
    await finish(tester);
  });

  testWidgets('a new slide has the placeholders of its layout', (tester) async {
    await open(tester, 'ph-populated-placeholders');
    await tester.tap(find.byTooltip('Nouvelle diapositive').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('3_Custom Layout').last);
    await settle(tester);
    final fresh = slides()[1];
    expect(fresh.attributes['layout'], 'L5');
    final placeholders = [for (final n in hub.doc.children(fresh.id)) n.attributes['ph']];
    expect(placeholders, [{'idx': '10', 'sz': 'quarter', 'type': 'tbl'}]);
    await finish(tester);
  });

  testWidgets('inserts a shape and types into it', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.text('Insertion'));
    await tester.pump();
    await tester.tap(find.byTooltip('Zone de texte'));
    await settle(tester);
    final box = hub.doc.children(slides()[0].id).last;
    expect(box.attributes['name'], startsWith('ZoneTexte'));

    // typing goes into the new text box through the text input
    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'Bonjour', selection: TextSelection.collapsed(offset: 7)));
    await settle(tester);
    expect(hub.doc[box.id]!.text!.text, 'Bonjour\n');

    // bold on the whole word, then Enter and more text
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyA);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyG);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(hub.doc[box.id]!.text!.ops.first.attributes?['b'], '1');
    await tester.sendKeyEvent(LogicalKeyboardKey.end);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(hub.doc[box.id]!.text!.text, 'Bonjour\n\n');

    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(hub.doc[box.id], isNull);
    await finish(tester);
  });
}
