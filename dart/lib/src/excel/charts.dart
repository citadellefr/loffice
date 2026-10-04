import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../chart/chart.dart';
import '../chart/chart_painter.dart';
import '../text/text_frame.dart';
import 'number_format.dart';
import 'workbook.dart';

const _emuPerPixel = 9525.0;

/// A chart a sheet holds, as the Go package reads those of its drawing
/// into its "charts".
class SheetChart {
  SheetChart._(this.spec, this._json);

  /// The charts of a sheet, read once for each version of it.
  static List<SheetChart> of(Node sheet) => _cache[sheet] ??= () {
    final all = sheet.attributes['charts'];
    if (all is! List<Object?>) return const <SheetChart>[];
    return [
      for (final c in all)
        if (c is Map<String, Object?>)
          if (ChartSpec.fromJson(c['chart']) case final spec?) SheetChart._(spec, c),
    ];
  }();

  static final _cache = Expando<List<SheetChart>>();

  final ChartSpec spec;
  final Map<String, Object?> _json;

  /// Where the chart lies, in the pixels of the sheet at 100 %.
  Rect rect(SheetLayout l) {
    Offset? corner(Object? json) {
      if (json is! Map<String, Object?>) return null;
      final col = ((json['col'] as num?) ?? 0).toInt() + 1, row = ((json['row'] as num?) ?? 0).toInt() + 1;
      final dx = ((json['dx'] as num?) ?? 0) / _emuPerPixel, dy = ((json['dy'] as num?) ?? 0) / _emuPerPixel;
      return Offset(l.x(col) + math.min(dx, l.width(col)), l.y(row) + math.min(dy, l.height(row)));
    }

    double emu(String key) => ((_json[key] as num?) ?? 0) / _emuPerPixel;
    final from = corner(_json['from']), to = corner(_json['to']);
    final size = Size(emu('w'), emu('h'));
    if (from != null && to != null) return Rect.fromPoints(from, to);
    return (from ?? Offset(emu('x'), emu('y'))) & size;
  }
}

/// Paints a chart of a workbook with the values its cells hold now, in a
/// box on screen, the sheet shown at [zoom].
void paintSheetChart(Canvas canvas, Rect box, ChartSpec spec, Workbook book, Node sheet, {double zoom = 1, String? fonts}) {
  if (box.isEmpty) return;
  final points = 72 / 96 / zoom;
  canvas.save();
  canvas.clipRect(box);
  canvas.translate(box.left, box.top);
  canvas.scale(1 / points);
  ChartPainter(
    spec,
    colors: book.colors,
    fonts: Fonts(theme: (name) => name.startsWith('+mj') ? book.majorFont : book.minorFont, package: fonts),
    live: spec.pivot ? null : (cached) => liveData(book, sheet, cached, NumberLocale.fr),
    background: const Color(0xFFFFFFFF),
  ).paint(canvas, box.size * points);
  canvas.restore();
}

/// The values a series points to, read from the cells of the workbook
/// rather than as Office cached them, those of hidden rows and columns
/// left out as Excel leaves them; null when the reference is not one to
/// cells, or spans several rows and columns.
DataSpec? liveData(Workbook book, Node sheet, DataSpec cached, NumberLocale locale) {
  var ref = cached.ref.trim();
  if (ref.startsWith('(') && ref.endsWith(')')) ref = ref.substring(1, ref.length - 1);
  final numbers = <double?>[];
  final texts = <String>[];
  var anyText = false;
  String? format;
  for (final part in ref.split(',')) {
    final found = book.resolve(sheet, part.trim());
    if (found == null) return null;
    final (on, area) = found;
    if (area.rows > 1 && area.cols > 1 || area.rows * area.cols > 100000) return null;
    final layout = book.layout(on);
    final grid = on.grid ?? const Grid.empty();
    for (var r = area.top; r <= area.bottom; r++) {
      if (layout.height(r) == 0) continue;
      for (var c = area.left; c <= area.right; c++) {
        if (layout.width(c) == 0) continue;
        final fields = grid.cell(r, c) ?? const <String, Object?>{};
        final style = book.style(fields['s'] as String?);
        final v = fields['v'];
        format ??= style.formatCode;
        anyText |= v is String;
        numbers.add(v is num ? v.toDouble() : null);
        texts.add(v == null && fields['e'] == null ? '' : cellText(fields, style, locale, date1904: book.date1904).text);
      }
    }
  }
  if (cached.isText || anyText) return DataSpec(ref: cached.ref, texts: texts, count: texts.length);
  return DataSpec(ref: cached.ref, format: format ?? cached.format, numbers: numbers, count: numbers.length);
}
