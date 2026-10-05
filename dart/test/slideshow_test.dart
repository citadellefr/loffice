import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/powerpoint/slideshow.dart';
import 'package:trame/trame.dart';

void main() {
  // shp-shapes has two slides
  Future<void> show(WidgetTester tester, List<Map<String, Object?>> transitions, {bool presenter = false}) async {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
    final deck = Deck(tree);
    for (final (i, t) in transitions.indexed) {
      expect(tree.apply(Edit([Change.set(deck.slides[i].id, attributes: {'transition': t})])), isNotNull);
    }
    if (presenter) {
      final notes = tree.children(deck.slides[0].id).firstWhere((n) => n.type == 'notes', orElse: () => const Node(id: '', type: '', key: ''));
      if (notes.id.isEmpty) {
        expect(tree.apply(Edit([Change.create(Node(id: 'notes', type: 'notes', parent: deck.slides[0].id, key: 'V', text: Delta([const Op.insert('Saluer la salle\n')])))])), isNotNull);
      }
    }
    await tester.pumpWidget(MaterialApp(home: Slideshow(deck: deck, painter: SlidePainter(deck), presenter: presenter)));
  }

  const end = 'Fin du diaporama, cliquez pour quitter.';

  testWidgets('goes on by itself, the next slide pushed in', (tester) async {
    await show(tester, [
      {'dur': 500, 'after': 1000},
      {'effect': 'push', 'dir': 'u', 'dur': 300, 'after': 1000},
    ]);
    await tester.pump(const Duration(milliseconds: 1100));
    await tester.pump(const Duration(milliseconds: 150));
    expect(find.text(end), findsNothing);
    await tester.pump(const Duration(milliseconds: 200));
    await tester.pump(const Duration(milliseconds: 1100));
    expect(find.text(end), findsOneWidget);
  });

  testWidgets('a slide a click does not go on from goes on with the keyboard', (tester) async {
    await show(tester, [
      {'dur': 500, 'noClick': true},
      {'effect': 'fade', 'thruBlk': true, 'dur': 0},
    ]);
    await tester.tap(find.byType(CustomPaint).last);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.space);
    await tester.pump();
    await tester.tap(find.byType(CustomPaint).last);
    await tester.pump();
    expect(find.text(end), findsOneWidget);
  });

  testWidgets('the presenter view shows the notes, the next slide and the time', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await show(tester, [], presenter: true);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    expect(find.textContaining('Saluer la salle'), findsOneWidget);
    expect(find.text('00:00'), findsOneWidget);
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('00:02'), findsOneWidget);
    await tester.tap(find.byTooltip('Diapositive suivante'));
    await tester.pump();
    expect(find.text('Diapositive 2 sur 2'), findsOneWidget);
    expect(find.text(end), findsOneWidget);
    await tester.tap(find.byTooltip('Suspendre le minuteur'));
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('00:02'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
}
