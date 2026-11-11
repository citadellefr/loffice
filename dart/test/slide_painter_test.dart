import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/animation_sequence.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:trame/trame.dart';

class _Look extends CustomPainter {
  _Look(this.painter, this.slide, this.looks);

  final SlidePainter painter;
  final Node slide;
  final Map<String, ShapeLook> looks;

  @override
  void paint(Canvas canvas, Size size) => painter.paint(canvas, slide, looks: looks);

  @override
  bool shouldRepaint(_Look old) => true;
}

void main() {
  final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
  final deck = Deck(tree);
  final slide = deck.slides[0];
  final shape = deck.shapesOf(slide).first;
  final box = deck.styleOf(shape).bounds!;
  final c = box.center;

  Future<void> draw(WidgetTester tester, ShapeLook look) => tester.pumpWidget(
    Directionality(
      textDirection: TextDirection.ltr,
      child: CustomPaint(painter: _Look(SlidePainter(deck), slide, {shape.id: look}), size: const Size(100, 100)),
    ),
  );

  testWidgets('a shape coming in moves the part of the way that is left, to the edge it comes from', (tester) async {
    await draw(tester, const ShapeLook(from: Offset(1, 0), away: 0.5));
    final reach = (deck.size.width - box.left) * 0.5;
    expect(find.byType(CustomPaint).last, paints..translate(x: reach, y: 0));
  });

  testWidgets('a shape that turns or scales does it around the center of its box', (tester) async {
    await draw(tester, const ShapeLook(angle: 1, scale: 1.5));
    expect(
      find.byType(CustomPaint).last,
      paints
        ..translate(x: 0, y: 0)
        ..translate(x: c.dx, y: c.dy)
        ..rotate(angle: 1)
        ..scale(x: 1.5, y: null)
        ..translate(x: -c.dx, y: -c.dy),
    );
  });

  testWidgets('a wipe clips the shape to the part of its box it shows', (tester) async {
    await draw(tester, const ShapeLook(reveal: Rect.fromLTRB(0, 0, 0.5, 1)));
    expect(
      find.byType(CustomPaint).last,
      paints
        ..clipRect(rect: Offset.zero & deck.size)
        ..clipRect(rect: Rect.fromLTRB(box.left, box.top, box.left + box.width / 2, box.bottom)),
    );
  });

  testWidgets('a shape that is not opaque is drawn in a layer of its own', (tester) async {
    await draw(tester, const ShapeLook(opacity: 0.5));
    expect(find.byType(CustomPaint).last, paints..something((name, args) => name == #saveLayer));
  });
}
