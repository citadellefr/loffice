import 'dart:convert';
import 'dart:io';
import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/edits.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:trame/trame.dart';

void main() {
  test('a turned group keeps the box of its shapes, which stay where they are', () {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
    // s257-5 holds s257-2, s257-3 on its right edge and s257-4
    final group = tree['s257-5']!.attributes['xfrm']! as Map<String, Object?>;
    expect(tree.apply(Edit([Change.set('s257-5', attributes: {'xfrm': {...group, 'rot': 5400000}})])), isNotNull);
    Offset onSlide(String id) => SlidePainter(Deck(tree)).transformOf(tree[id]!).apply(Offset.zero);
    final before = {for (final id in ['s257-2', 's257-4']) id: onSlide(id)};

    final x = tree['s257-3']!.attributes['xfrm']! as Map<String, Object?>;
    double pt(String k) => (x[k]! as num) / emuPerPoint;
    final moved = Rect.fromLTWH(pt('x') + 20, pt('y'), pt('w'), pt('h'));
    expect(tree.apply(DeckEdits(Deck(tree)).place({tree['s257-3']!: moved})), isNotNull);

    final fitted = tree['s257-5']!.attributes['xfrm']! as Map<String, Object?>;
    expect(fitted['cw'], 1105272 + 254000);
    expect(fitted['w'], 1105272 + 254000);
    expect(fitted['rot'], 5400000);
    for (final MapEntry(key: id, value: at) in before.entries) {
      expect(onSlide(id), offsetMoreOrLessEquals(at, epsilon: 0.01));
    }
  });
}
