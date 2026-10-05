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
  Future<void> show(WidgetTester tester, List<Map<String, Object?>> transitions) async {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
    final deck = Deck(tree);
    for (final (i, t) in transitions.indexed) {
      expect(tree.apply(Edit([Change.set(deck.slides[i].id, attributes: {'transition': t})])), isNotNull);
    }
    await tester.pumpWidget(MaterialApp(home: Slideshow(deck: deck, painter: SlidePainter(deck))));
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
}
