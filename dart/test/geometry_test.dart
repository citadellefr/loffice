import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/drawing/geometry.dart';
import 'package:loffice/src/drawing/presets.dart';

Rect _bounds(List<ShapePath> paths) => paths.map((p) => p.path.getBounds()).reduce((a, b) => a.expandToInclude(b));

void main() {
  const size = Size(200, 100);

  test('every preset shape draws within reach of its box', () {
    expect(presetGeometries, hasLength(187));
    for (final name in presetGeometries.keys) {
      final g = Geometry.preset(name)!;
      final paths = g.paths(size);
      expect(paths, isNotEmpty, reason: name);
      final b = _bounds(paths);
      for (final v in [b.left, b.top, b.right, b.bottom]) {
        expect(v.isFinite, isTrue, reason: '$name: $b');
      }
      // callouts and a few others reach outside their box, not far
      expect(b.width, lessThan(size.width * 4), reason: '$name: $b');
      expect(b.height, lessThan(size.height * 4), reason: '$name: $b');
      final text = g.textRect(size);
      expect(text.isFinite && text.width >= 0 && text.height >= 0, isTrue, reason: '$name: $text');
    }
  });

  test('draws the common shapes where Office does', () {
    expect(_bounds(Geometry.preset('rect')!.paths(size)), const Rect.fromLTWH(0, 0, 200, 100));
    final ellipse = _bounds(Geometry.preset('ellipse')!.paths(size));
    expect(ellipse.left, closeTo(0, 0.01));
    expect(ellipse.right, closeTo(200, 0.01));
    expect(ellipse.bottom, closeTo(100, 0.01));
    expect(Geometry.preset('ellipse')!.paths(size).single.path.contains(const Offset(5, 5)), isFalse);
    expect(Geometry.preset('ellipse')!.paths(size).single.path.contains(const Offset(100, 50)), isTrue);

    // a rounded rectangle's corners, and its adjust value
    final round = Geometry.preset('roundRect')!;
    expect(round.paths(size).single.path.contains(const Offset(1, 1)), isFalse);
    expect(round.paths(size, {'adj': 'val 0'}).single.path.contains(const Offset(1, 1)), isTrue);

    // a triangle's apex follows its adjust value
    final tri = Geometry.preset('triangle')!;
    expect(tri.paths(size, {'adj': 'val 0'}).single.path.contains(const Offset(2, 20)), isTrue);
    expect(tri.paths(size).single.path.contains(const Offset(2, 20)), isFalse);

    // text inset: an ellipse puts its text in the inscribed rectangle
    final text = Geometry.preset('ellipse')!.textRect(size);
    expect(text.left, closeTo(200 * (1 - 0.7071) / 2, 0.5));
    expect(text.top, closeTo(100 * (1 - 0.7071) / 2, 0.5));
  });

  test('draws custom geometries in their own coordinates, arcs as Office', () {
    final g = Geometry.fromJson({
      'paths': [
        {
          'w': 100,
          'h': 100,
          'd': [
            ['M', '0', '50'],
            ['A', '50', '50', '10800000', '10800000'],
            ['L', '100', '100'],
            ['L', '0', '100'],
            ['Z'],
          ],
        },
      ],
    })!;
    final path = g.paths(size).single.path;
    final b = path.getBounds();
    expect(b.left, closeTo(0, 0.01));
    expect(b.top, closeTo(0, 0.01));
    expect(b.right, closeTo(200, 0.01));
    expect(b.bottom, closeTo(100, 0.01));
    // the top of the half ellipse, and the corners it leaves out
    expect(path.contains(const Offset(100, 2)), isTrue);
    expect(path.contains(const Offset(3, 3)), isFalse);
  });
}
