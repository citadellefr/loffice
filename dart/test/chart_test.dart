import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/chart/chart.dart';
import 'package:loffice/src/chart/chart_painter.dart';
import 'package:loffice/src/chart/scale.dart';
import 'package:loffice/src/drawing/color.dart';
import 'package:loffice/src/ot/tree.dart';

void main() {
  test('value axes take the bounds and steps Excel takes', () {
    void expectScale(Scale s, double min, double max, double step) {
      expect((s.min, s.max, s.step), (min, max, step));
    }

    expectScale(Scale.auto(1, 8.2), 0, 9, 1);
    expectScale(Scale.auto(0, 100), 0, 120, 20);
    expectScale(Scale.auto(1, 6), 0, 7, 1);
    expectScale(Scale.auto(-4, 2.5), -5, 3, 1);
    // far from zero, the axis starts near the values
    expectScale(Scale.auto(95, 100), 94, 101, 1);
    expectScale(Scale.auto(-8, -2), -9, 0, 1);
    // what the file fixes is kept
    expectScale(Scale.auto(1, 8.2, max: 70, major: 10), 0, 70, 10);
    expectScale(Scale.auto(0, 0), 0, 1.2, 0.2);
    expect(Scale.auto(0, 1, min: 0, max: 1).ticks.last, 1);
    final log = Scale.auto(3, 4200, log: 10);
    expect((log.min, log.max), (1, 10000));
    expect(log.ticks, [1, 10, 100, 1000, 10000]);
    expect(log.fraction(100), closeTo(0.5, 1e-9));
  });

  test('every chart of the test presentations is drawn', () async {
    final edit = Edit.fromJson(jsonDecode(File('../testdata/pptx/cht-chart-type.json').readAsStringSync()))!;
    final tree = Tree.fromEdit(edit)!;
    final kinds = <String>{};
    var count = 0;
    const colors = ColorContext(scheme: {'accent1': Color(0xFF4F81BD), 'accent2': Color(0xFFC0504D), 'accent3': Color(0xFF9BBB59)});
    for (final node in tree.nodes) {
      final chart = ChartSpec.fromJson(node.attributes['chart']);
      if (chart == null) continue;
      count++;
      kinds.addAll(chart.plots.map((p) => p.kind));
      final recorder = ui.PictureRecorder();
      ChartPainter(chart, colors: colors).paint(Canvas(recorder), const Size(360, 240));
      recorder.endRecording().dispose();
    }
    expect(count, 31);
    expect(kinds, containsAll(['area', 'bar', 'line', 'pie', 'scatter', 'bubble', 'radar']));
  });

  test('live values replace those Office cached', () {
    final chart = ChartSpec.fromJson({
      'plots': [
        {
          'kind': 'bar',
          'series': [
            {
              'index': 0,
              'order': 0,
              'val': {'ref': 'Feuil1!\$B\$2:\$B\$3', 'count': 2, 'num': [1, 2]},
            },
          ],
        },
      ],
    })!;
    final asked = <String>[];
    final painter = ChartPainter(chart, colors: const ColorContext(), live: (d) {
      asked.add(d.ref);
      return DataSpec(ref: d.ref, numbers: [5, 6, 7], count: 3);
    });
    final recorder = ui.PictureRecorder();
    painter.paint(Canvas(recorder), const Size(200, 100));
    recorder.endRecording().dispose();
    expect(asked, ['Feuil1!\$B\$2:\$B\$3']);
  });
}
