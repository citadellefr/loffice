import 'dart:convert';
import 'dart:ui';

import 'package:trame/trame.dart';

import '../chart/chart.dart';
import 'charts.dart';
import 'workbook.dart';

/// The kinds of chart the Insert tab offers.
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

  /// The type a plot is of, null for one the tab does not offer.
  static ChartType? of(PlotSpec plot) {
    for (final t in values) {
      if (t.kind == plot.kind && (t.dir == null || t.dir == (plot.horizontal ? 'bar' : 'col')) && t.grouping == plot.grouping) return t;
    }
    return null;
  }
}

/// A series as the dialog of the data lists it: the one of the plot it
/// stands for, null for a new one, and the references it points to.
class SeriesInput {
  const SeriesInput(this.source, this.name, this.values);

  final int? source;
  final String name;
  final String values;
}

/// The edits of the charts of a sheet, as the Go package takes them: each
/// starts from a copy of what the sheet holds and changes only the keys it
/// means to, as what is missing reads as removed. A value is made for one
/// edit: every method returns the list of charts to set.
class ChartEdits {
  ChartEdits(Node sheet, this.layout) : _all = _copy(sheet.attributes['charts']);

  final SheetLayout layout;
  final List<Map<String, Object?>> _all;

  static List<Map<String, Object?>> _copy(Object? charts) {
    if (charts is! List<Object?>) return [];
    return [...(json.decode(json.encode(charts)) as List<Object?>).whereType<Map<String, Object?>>()];
  }

  Map<String, Object?> _chart(int i) => _all[i]['chart']! as Map<String, Object?>;

  Map<String, Object?> _plot(int i) => (_chart(i)['plots']! as List<Object?>).first! as Map<String, Object?>;

  /// Adds a chart of the cells of an area of the sheet [name], right of
  /// them; the Go package gives it its series.
  List<Map<String, Object?>> add(CellArea area, String name, ChartType type) {
    final row = area.top - 1;
    Map<String, Object?> corner(int col, int row) => {'col': col, 'row': row, 'dx': 0, 'dy': 0};
    final quoted = "'${name.replaceAll("'", "''")}'";
    String cell(int row, int col) => '\$${columnName(col)}\$$row';
    _all.add({
      'from': corner(area.right, row),
      'to': corner(area.right + 8, row + 15),
      'source': '$quoted!${cell(area.top, area.left)}:${cell(area.bottom, area.right)}',
      'chart': {
        'plots': [
          {'kind': type.kind, 'dir': ?type.dir, 'grouping': type.grouping, 'series': <Object?>[]},
        ],
        'legend': {'pos': 'r'},
        'title': <String, Object?>{},
      },
    });
    return _all;
  }

  List<Map<String, Object?>> retype(int i, ChartType type) {
    final plot = _plot(i);
    plot['kind'] = type.kind;
    if (type.dir == null) {
      plot.remove('dir');
    } else {
      plot['dir'] = type.dir;
    }
    plot['grouping'] = type.grouping;
    plot.remove('3d');
    return _all;
  }

  /// Sets the title of a chart, the one made of its series for an empty
  /// text, none for null.
  List<Map<String, Object?>> setTitle(int i, String? text) {
    _titled(_chart(i), text);
    return _all;
  }

  /// Sets where the legend goes, none for null.
  List<Map<String, Object?>> setLegend(int i, String? position) {
    final chart = _chart(i);
    final legend = chart['legend'];
    if (position == null) {
      chart.remove('legend');
    } else {
      chart['legend'] = {if (legend is Map<String, Object?>) ...legend, 'pos': position};
    }
    return _all;
  }

  /// Sets the titles of the axes of the first plot, the one of its
  /// categories and the one of its values, none for empty ones.
  List<Map<String, Object?>> setAxisTitles(int i, String categories, String values) {
    final ids = (_plot(i)['axes'] as List<Object?>?) ?? const [];
    final axes = (_chart(i)['axes'] as List<Object?>?) ?? const [];
    for (final (k, text) in [categories, values].indexed) {
      if (k >= ids.length) continue;
      for (final axis in axes.whereType<Map<String, Object?>>()) {
        if (axis['id'] == ids[k]) _titled(axis, text.isEmpty ? null : text);
      }
    }
    return _all;
  }

  /// Sets the series of the first plot and the categories they share.
  List<Map<String, Object?>> setSeries(int i, List<SeriesInput> inputs, String categories) {
    final plot = _plot(i);
    final old = (plot['series'] as List<Object?>?) ?? const [];
    plot['series'] = [
      for (final s in inputs)
        () {
          final series = s.source == null ? <String, Object?>{} : old[s.source!]! as Map<String, Object?>;
          _reference(series, 'name', s.name);
          _reference(series, 'val', s.values);
          _reference(series, 'cat', categories);
          return series;
        }(),
    ];
    return _all;
  }

  /// Puts a chart in a rectangle, in pixels of the sheet at 100 %.
  List<Map<String, Object?>> place(int i, Rect rect) {
    final placed = _all[i];
    int emu(double px) => (px * emuPerPixel).round();
    if (placed['from'] == null) {
      placed['x'] = emu(rect.left);
      placed['y'] = emu(rect.top);
    } else {
      placed['from'] = chartAnchor(layout, rect.topLeft);
    }
    if (placed['to'] == null) {
      placed['w'] = emu(rect.width);
      placed['h'] = emu(rect.height);
    } else {
      placed['to'] = chartAnchor(layout, rect.bottomRight);
    }
    return _all;
  }

  List<Map<String, Object?>> remove(int i) => _all..removeAt(i);

  static void _titled(Map<String, Object?> owner, String? text) {
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

  /// Points the values of a series to the cells a reference names, typed
  /// with or without "=", none for an empty one.
  static void _reference(Map<String, Object?> series, String key, String typed) {
    final ref = typed.trim().startsWith('=') ? typed.trim().substring(1).trim() : typed.trim();
    final data = series[key];
    if (ref == (data is Map<String, Object?> ? data['ref'] ?? '' : '')) return;
    if (ref.isEmpty) {
      series.remove(key);
    } else {
      series[key] = {if (data is Map<String, Object?>) ...data, 'ref': ref};
    }
  }
}
