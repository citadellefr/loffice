import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/chart/chart.dart';
import 'package:loffice/src/chart/chart_painter.dart';
import 'package:loffice/src/chart/extended.dart';
import 'package:loffice/src/drawing/color.dart';

const _colors = ColorContext(scheme: {'accent1': Color(0xFF4F81BD), 'accent2': Color(0xFFC0504D), 'accent3': Color(0xFF9BBB59)});

class _Spy implements Canvas {
  final calls = <Symbol, int>{};

  @override
  dynamic noSuchMethod(Invocation invocation) {
    calls[invocation.memberName] = (calls[invocation.memberName] ?? 0) + 1;
    return null;
  }

  int count(Symbol name) => calls[name] ?? 0;
}

Map<String, Object?> _data(List<Object?> values, {String? ref}) => {'ref': ?ref, 'count': values.length, 'num': values};

Map<String, Object?> _texts(List<String> values) => {'count': values.length, 'str': values};

Map<String, Object?> _chart(String kind, Map<String, Object?> series, {List<Map<String, Object?>> axes = const [], Object? legend}) => {
  'plots': [
    {
      'kind': kind,
      'axes': [for (final a in axes) a['id']],
      'series': [series],
    },
  ],
  'axes': axes,
  'legend': ?legend,
};

const _axes = [
  {'id': 0, 'kind': 'cat', 'pos': 'b'},
  {'id': 1, 'kind': 'val', 'pos': 'l', 'grid': <String, Object?>{}},
];

const _cats = ['A', 'B', 'C', 'D', 'E', 'F'];
const _values = [100, 30, -20, 110, 15, 125];

ChartSpec _spec(String kind) {
  final base = {'index': 0, 'order': 0, 'cat': _texts(_cats), 'val': _data(_values)};
  final json = switch (kind) {
    'waterfall' => _chart(kind, {
      ...base,
      'options': {
        'subtotals': [0, 3, 5],
      },
      'labels': {'val': true},
    }, axes: _axes),
    'histogram' => _chart(kind, {
      ...base,
      'val': _data([1, 2, 2, 3, 4, 4, 4, 5, 7, 8, 9, 10]),
      'options': {
        'bins': {'size': 3.0},
      },
    }, axes: _axes),
    'pareto' => {
      'plots': [
        {
          'kind': 'pareto',
          'axes': [0, 1, 2],
          'series': [
            {
              ...base,
              'val': _data([40, 25, 15, 12, 8]),
              'cat': _texts(['a', 'b', 'c', 'd', 'e']),
              'options': {'aggregate': true},
            },
            {
              'index': 1,
              'order': 1,
              'options': {'line': true},
            },
          ],
        },
      ],
      'axes': [
        ..._axes,
        {'id': 2, 'kind': 'val', 'pos': 'r', 'max': 1, 'min': 0},
      ],
    },
    'boxWhisker' => _chart(kind, {
      ...base,
      'val': _data([1, 2, 3, 4, 5, 6, 7, 8, 9, 40]),
      'cat': _texts(['x', 'x', 'x', 'x', 'x', 'y', 'y', 'y', 'y', 'y']),
      'options': {
        'box': {'meanMarker': true, 'outliers': true, 'inner': true},
      },
    }, axes: _axes),
    'funnel' => _chart(kind, {
      ...base,
      'val': _data([100, 80, 50, 20]),
      'cat': _texts(['a', 'b', 'c', 'd']),
      'labels': {'val': true},
    }),
    'treemap' || 'sunburst' => _chart(
      kind,
      {
        'index': 0,
        'order': 0,
        'cat': {
          'count': 6,
          'str': ['Pommes', 'Poires', 'Carottes', 'Navets', 'Poireaux', 'Lait'],
          'levels': [
            ['Pommes', 'Poires', 'Carottes', 'Navets', 'Poireaux', 'Lait'],
            ['Fruits', 'Fruits', 'Légumes', 'Légumes', 'Légumes', 'Laitages'],
          ],
        },
        'val': _data([30, 20, 25, 10, 5, 10]),
        'labels': {'cat': true},
      },
      legend: {'pos': 't'},
    ),
    _ => _chart(kind, {
      ...base,
      'cat': _texts(['France', 'Italie']),
      'val': null,
    }),
  };
  return ChartSpec.fromJson(json)!;
}

_Spy _paint(ChartSpec chart) {
  final spy = _Spy();
  ChartPainter(chart, colors: _colors).paint(spy, const Size(400, 300));
  return spy;
}

void main() {
  test('the bars of a waterfall float from the running total, a total from zero', () {
    final steps = waterfallSteps([100, 30, -20, 110, 15, 125], {0, 3, 5});
    expect(
      [for (final s in steps) (s.from, s.to, s.kind)],
      [(0, 100, StepKind.total), (100, 130, StepKind.increase), (130, 110, StepKind.decrease), (0, 110, StepKind.total), (110, 125, StepKind.increase), (0, 125, StepKind.total)],
    );
    expect((steps[2].low, steps[2].high), (110, 130));
  });

  test('a histogram counts the values of each interval by the size, the count or Scott\'s rule', () {
    final values = <double>[1, 2, 2, 3, 4, 4, 4, 5, 7, 8, 9, 10];
    expect([for (final b in histogramBins(values, size: 3)) (b.low, b.high, b.count)], [(1.0, 4.0, 7), (4.0, 7.0, 2), (7.0, 10.0, 3)]);
    expect([for (final b in histogramBins(values, size: 3, closedRight: false)) b.count], [4, 4, 4]);
    expect([for (final b in histogramBins(values, count: 3)) b.count], [7, 2, 3]);
    expect(histogramBins(values).fold(0, (n, b) => n + b.count), values.length);
    final beyond = histogramBins(values, size: 3, under: 2, over: 8);
    expect((beyond.first.low, beyond.first.count, beyond.last.high, beyond.last.count), (double.negativeInfinity, 3, double.infinity, 2));
    expect(beyond.fold(0, (n, b) => n + b.count), values.length);
  });

  test('a pareto sorts the totals of its categories and cumulates them', () {
    final totals = categoryTotals(['a', 'b', 'a', 'c', 'a', 'b'], [], sorted: true);
    expect(totals, [('a', 3.0), ('b', 2.0), ('c', 1.0)]);
    expect(cumulated([for (final t in totals) t.$2]), [0.5, closeTo(5 / 6, 1e-9), 1.0]);
    expect(categoryTotals(['a', 'b', 'a'], [1, 5, 2]), [('a', 3.0), ('b', 5.0)]);
  });

  test('a box and whisker has the quartiles, whiskers and outliers Excel finds', () {
    final values = <double>[1, 2, 3, 4, 5, 6, 7, 8, 9, 40];
    final exclusive = boxStats(values)!;
    expect((exclusive.q1, exclusive.median, exclusive.q3), (2.75, 5.5, 8.25));
    expect((exclusive.low, exclusive.high), (1, 9));
    expect(exclusive.outliers, [40]);
    expect(exclusive.mean, 8.5);
    final inclusive = boxStats(values, exclusive: false)!;
    expect((inclusive.q1, inclusive.q3), (3.25, 7.75));
    expect(boxStats([]), isNull);
  });

  test('a funnel is as wide as its values', () {
    final bars = funnelBars([100, 50, 0], const Rect.fromLTWH(0, 0, 200, 90));
    expect([for (final b in bars) b.width], [200, 100, 0]);
    expect(bars[0].center.dx, bars[1].center.dx);
    expect(bars[1].top, greaterThan(bars[0].bottom));
  });

  test('a treemap covers its area with rectangles of the size of their values', () {
    const area = Rect.fromLTWH(0, 0, 200, 100);
    final rects = squarify([6, 6, 4, 3, 2, 2, 1], area);
    expect(rects.fold(0.0, (a, r) => a + r.width * r.height), closeTo(200 * 100, 1e-6));
    for (final (i, w) in [6, 6, 4, 3, 2, 2, 1].indexed) {
      expect(rects[i].width * rects[i].height, closeTo(w / 24 * 200 * 100, 1e-6));
      expect(area.inflate(1e-6).expandToInclude(rects[i]), area.inflate(1e-6));
    }
    final root = hierarchy(
      DataSpec(
        texts: ['Pommes', 'Poires', 'Carottes'],
        levels: [
          ['Pommes', 'Poires', 'Carottes'],
          ['Fruits', 'Fruits', 'Légumes'],
        ],
      ),
      [30, 20, 25],
    );
    expect([for (final b in root.children) (b.label, b.total)], [('Fruits', 50.0), ('Légumes', 25.0)]);
    final tiles = treemapTiles(root, area);
    expect(tiles.length, 5);
    final fruits = tiles.firstWhere((t) => t.branch.label == 'Fruits').rect;
    final apples = tiles.firstWhere((t) => t.branch.label == 'Pommes').rect;
    expect(fruits.width * fruits.height, closeTo(50 / 75 * 198 * 98, 1e-6));
    expect(fruits.inflate(1e-6).expandToInclude(apples), fruits.inflate(1e-6));
  });

  test('a sunburst gives each category the angle of its value, a ring a level', () {
    final root = hierarchy(
      DataSpec(
        texts: ['Pommes', 'Poires', 'Carottes'],
        levels: [
          ['Pommes', 'Poires', 'Carottes'],
          ['Fruits', 'Fruits', 'Légumes'],
        ],
      ),
      [30, 20, 25],
    );
    final arcs = sunburstArcs(root);
    final fruits = arcs.firstWhere((a) => a.branch.label == 'Fruits');
    expect(fruits.ring, 0);
    expect(fruits.sweep, closeTo(2 * 3.141592653589793 * 50 / 75, 1e-9));
    final apples = arcs.firstWhere((a) => a.branch.label == 'Pommes');
    expect((apples.ring, apples.start), (1, fruits.start));
    expect(apples.sweep, closeTo(2 * 3.141592653589793 * 30 / 75, 1e-9));
  });

  test('the seven charts of Excel 2016 are drawn, a shape to each bar, box, tile or arc', () {
    const shapes = {'waterfall': 6, 'histogram': 3, 'pareto': 10, 'boxWhisker': 2, 'funnel': 4, 'treemap': 9, 'sunburst': 9};
    for (final MapEntry(:key, :value) in shapes.entries) {
      final chart = _spec(key);
      expect(chart.extended, isTrue, reason: key);
      final spy = _paint(chart);
      expect(spy.count(#drawPath) + spy.count(#drawRect) + spy.count(#drawCircle), greaterThanOrEqualTo(value), reason: key);
      expect(spy.count(#drawParagraph), greaterThan(0), reason: key);
    }
  });

  test('a map the client cannot draw gets a substitute', () {
    final chart = _spec('regionMap');
    final spy = _paint(chart);
    expect(chart.extended, isTrue);
    expect(spy.count(#drawRect), greaterThan(0));
    expect(spy.count(#drawParagraph), greaterThan(0));
  });

  test('a chart of Excel 2016 is drawn in a recording', () {
    for (final kind in ['waterfall', 'histogram', 'pareto', 'boxWhisker', 'funnel', 'treemap', 'sunburst', 'regionMap']) {
      final recorder = ui.PictureRecorder();
      ChartPainter(_spec(kind), colors: _colors).paint(Canvas(recorder), const Size(360, 240));
      recorder.endRecording().dispose();
    }
  });
}
