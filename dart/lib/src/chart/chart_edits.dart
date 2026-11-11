import 'dart:convert';

import 'package:trame/trame.dart';

import 'chart.dart';

/// The kinds of chart the editors offer.
enum ChartType {
  column('bar', 'col', 'clustered'),
  bar('bar', 'bar', 'clustered'),
  line('line', null, 'standard'),
  pie('pie', null, 'standard'),
  scatter('scatter', null, 'standard'),
  area('area', null, 'standard');

  const ChartType(this.kind, this.dir, this.grouping);

  final String kind;
  final String? dir;
  final String grouping;

  /// The type a plot is of, null for one the editors do not offer.
  static ChartType? of(PlotSpec plot) {
    for (final t in values) {
      if (t.kind == plot.kind && (t.dir == null || t.dir == (plot.horizontal ? 'bar' : 'col')) && t.grouping == plot.grouping) return t;
    }
    return null;
  }
}

/// The edits of a chart as the Go package takes it: each changes only the
/// keys it means to of a copy of the chart, as what is missing reads as
/// removed.
abstract final class ChartChanges {
  static Map<String, Object?> copy(Object? chart) => json.decode(json.encode(chart)) as Map<String, Object?>;

  /// The edit of the chart of a node of a document, [change] made to a copy.
  static Edit edit(Node node, void Function(Map<String, Object?> chart) change) {
    final chart = copy(node.attributes['chart']);
    change(chart);
    return Edit([
      Change.set(node.id, attributes: {'chart': chart}),
    ]);
  }

  static Map<String, Object?> plot(Map<String, Object?> chart) => (chart['plots']! as List<Object?>).first! as Map<String, Object?>;

  static void retype(Map<String, Object?> chart, ChartType type) {
    final plot = ChartChanges.plot(chart);
    plot['kind'] = type.kind;
    if (type.dir == null) {
      plot.remove('dir');
    } else {
      plot['dir'] = type.dir;
    }
    plot['grouping'] = type.grouping;
    plot.remove('3d');
  }

  /// Sets the title of a chart or an axis, the one made of the series of
  /// a chart for an empty text, none for null.
  static void setTitle(Map<String, Object?> owner, String? text) {
    if (text == null) {
      owner.remove('title');
      return;
    }
    final old = owner['title'];
    final title = <String, Object?>{if (old is Map<String, Object?>) ...old};
    if (text.isEmpty) {
      title.remove('text');
    } else {
      title['text'] = text;
    }
    owner['title'] = title;
  }

  /// Sets where the legend goes, none for null.
  static void setLegend(Map<String, Object?> chart, String? position) {
    final legend = chart['legend'];
    if (position == null) {
      chart.remove('legend');
    } else {
      chart['legend'] = {if (legend is Map<String, Object?>) ...legend, 'pos': position};
    }
  }
}
