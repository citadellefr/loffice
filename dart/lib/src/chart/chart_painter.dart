import 'dart:convert';
import 'dart:math' as math;

import 'package:flutter/painting.dart';

import '../drawing/color.dart';
import '../drawing/paint.dart';
import '../excel/number_format.dart';
import '../pdf/canvas.dart';
import '../text/text_frame.dart';
import 'chart.dart';
import 'scale.dart';

/// Values the host reads afresh where a series points, instead of those
/// Office cached: the cells of the workbook a chart sits in.
typedef LiveData = DataSpec? Function(DataSpec cached);

/// Paints a chart in a box, in points, as Office lays it out: title on
/// top, legend aside, the plot in what is left.
class ChartPainter {
  ChartPainter(this.chart, {required this.colors, this.fonts = const Fonts(), this.live, this.background});

  final ChartSpec chart;
  final ColorContext colors;
  final Fonts fonts;
  final LiveData? live;

  /// The fill of a chart whose file gives none: white in a workbook,
  /// none on a slide or a page.
  final Color? background;

  static const _pad = 7.0;
  static const _tick = 4.0;

  late final _painter = DrawingPainter(colors);
  final _data = Expando<DataSpec>();

  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;
    final box = Offset.zero & size;
    canvas.save();
    canvas.clipRect(box.inflate(1));
    _box(canvas, box, chart.space, fallback: background, rounded: chart.rounded);
    var inner = box.deflate(_pad);
    inner = _title(canvas, box, inner);
    inner = _legend(canvas, box, inner);
    final plots = chart.plots.where((p) => p.series.isNotEmpty).toList();
    if (plots.isEmpty) {
      canvas.restore();
      return;
    }
    if (plots.every((p) => p.round)) {
      final area = _manual(chart.layout, box) ?? inner;
      _box(canvas, area, chart.area);
      for (final p in plots) {
        _round(canvas, p, area);
      }
    } else if (plots.first.kind == 'radar') {
      final area = _manual(chart.layout, box) ?? inner;
      _box(canvas, area, chart.area);
      _radar(canvas, plots.first, area);
    } else {
      _cartesian(canvas, box, inner, plots.where((p) => !p.round && p.kind != 'radar').toList());
    }
    canvas.restore();
  }

  // values

  DataSpec? _resolve(DataSpec? d) {
    if (d == null) return null;
    return _data[d] ??= (d.ref.isNotEmpty ? live?.call(d) : null) ?? d;
  }

  DataSpec? _val(SeriesSpec s) => _resolve(s.val);
  DataSpec? _cat(SeriesSpec s) => _resolve(s.cat);

  double? _value(SeriesSpec s, int i) {
    final v = _val(s)?.number(i);
    if (v == null && chart.blanks == 'zero' && i < (_val(s)?.length ?? 0)) return 0;
    return v;
  }

  String _name(SeriesSpec s) {
    final n = _resolve(s.name);
    if (n != null) {
      final t = n.text(0);
      if (t.isNotEmpty) return t;
      final v = n.number(0);
      if (v != null) return _format(v, n.format);
    }
    return 'Série${s.index + 1}';
  }

  int _count(List<SeriesSpec> series) {
    var n = 0;
    for (final s in series) {
      n = math.max(n, math.max(_val(s)?.length ?? 0, _cat(s)?.length ?? 0));
    }
    return n;
  }

  /// The categories, from the first series that has them.
  String _category(List<SeriesSpec> series, int i) {
    for (final s in series) {
      final c = _cat(s);
      if (c == null) continue;
      if (c.texts.isNotEmpty) return c.text(i);
      final v = c.number(i);
      return v == null ? '' : _format(v, c.format);
    }
    return '${i + 1}';
  }

  String _format(double v, String code) {
    if (code.isEmpty || code == 'General') return generalText(v, NumberLocale.fr);
    return NumberFormat(code).format(v, NumberLocale.fr).text;
  }

  // colors

  /// The color Office gives a series, or a point of a series that varies,
  /// which the file leaves without one: the accents of the theme in turn,
  /// darker and lighter on the next rounds, or shades of one accent or of
  /// gray for the styles that use them.
  Map<String, Object?> _auto(int i, int count) {
    final column = (chart.style - 1) % 8;
    if (column == 0) {
      final t = count <= 1 ? 0.5 : i / (count - 1);
      final v = (0x40 + (0xC0 - 0x40) * t).round();
      final hex = v.toRadixString(16).padLeft(2, '0').toUpperCase();
      return {'rgb': '$hex$hex$hex'};
    }
    if (column >= 2) {
      final t = count <= 1 ? 0.0 : i / (count - 1) * 2 - 1;
      return {
        'scheme': 'accent${column - 1}',
        'mods': [
          if (t < 0) ['shade', ((1 + t * 0.5) * 100000).round()],
          if (t > 0) ['tint', ((1 - t * 0.6) * 100000).round()],
        ],
      };
    }
    const rounds = [
      <List<Object>>[],
      [['lumMod', 60000]],
      [['lumMod', 80000], ['lumOff', 20000]],
      [['lumMod', 80000]],
      [['lumMod', 60000], ['lumOff', 40000]],
      [['lumMod', 50000]],
      [['lumMod', 70000], ['lumOff', 30000]],
      [['lumMod', 70000]],
      [['lumMod', 50000], ['lumOff', 50000]],
    ];
    return {'scheme': 'accent${i % 6 + 1}', 'mods': rounds[(i ~/ 6) % rounds.length]};
  }

  /// The fill of a series or of a point.
  Map<String, Object?> _fill(SeriesSpec s, int point, {bool vary = false, int count = 1}) {
    final own = _pointShape(s, point)?['fill'] ?? s.shape?['fill'];
    if (own is Map<String, Object?>) return own;
    final index = vary ? point : _seriesIndex(s);
    return {'solid': _auto(index, vary ? count : _seriesCount)};
  }

  /// The line of a series or of a point; [lines] tells whether it has one
  /// when the file says nothing, as those of line charts do.
  Map<String, Object?>? _line(SeriesSpec s, int point, {bool lines = false, bool vary = false, int count = 1}) {
    final own = {...?_map(s.shape?['line']), ...?_map(_pointShape(s, point)?['line'])};
    final fill = own['fill'];
    if (fill is Map && fill['none'] == true) return null;
    if (fill == null && !lines) return null;
    final color = fill ?? {'solid': _auto(vary ? point : _seriesIndex(s), vary ? count : _seriesCount)};
    return {'w': 28575, 'cap': 'rnd', ...own, 'fill': color};
  }

  Map<String, Object?>? _pointShape(SeriesSpec s, int i) => _map(s.points[i]?['shape']);

  late final List<SeriesSpec> _allSeries = chart.series;
  int get _seriesCount => _allSeries.length;
  int _seriesIndex(SeriesSpec s) => s.index;

  // boxes and text

  void _box(Canvas canvas, Rect rect, Map<String, Object?>? shape, {Color? fallback, bool rounded = false}) {
    final path = Path();
    if (rounded) {
      path.addRRect(RRect.fromRectAndRadius(rect, const Radius.circular(6)));
    } else {
      path.addRect(rect);
    }
    final fill = shape?['fill'];
    if (fill is Map<String, Object?>) {
      _painter.fill(canvas, path, rect, fill);
    } else if (fallback != null) {
      canvas.drawPath(path, Paint()..color = fallback);
    }
    final line = shape?['line'];
    if (line is Map<String, Object?>) _painter.line(canvas, path, line);
  }

  TextPainter _text(String text, Props props, {double size = 10, bool bold = false, double? width, TextAlign align = TextAlign.center}) {
    final p = {...chart.text, ...props};
    final own = double.tryParse(props['sz'] ?? ''), base = double.tryParse(chart.text['sz'] ?? '');
    final b = p['b'];
    final fill = p['fill'];
    Color? color;
    if (fill != null) {
      try {
        color = _painter.color(jsonDecode(fill) as Map<String, Object?>?);
      } on FormatException {
        color = null;
      }
    }
    final (family, fallback) = fonts.families(p['font'] ?? '+mn-lt');
    final painter = TextPainter(
      text: TextSpan(
        text: text,
        style: TextStyle(
          fontFamily: family,
          fontFamilyFallback: fallback,
          fontSize: own != null ? own / 100 : (base != null ? base / 100 * size / 10 : size),
          fontWeight: (b == null ? bold : b == '1') ? FontWeight.bold : FontWeight.normal,
          fontStyle: p['i'] == '1' ? FontStyle.italic : FontStyle.normal,
          color: color ?? colors.resolve(const {'scheme': 'tx1', 'mods': [['lumMod', 75000], ['lumOff', 25000]]}) ?? const Color(0xFF404040),
          height: 1.15,
        ),
      ),
      textAlign: align,
      textDirection: TextDirection.ltr,
    );
    painter.layout(maxWidth: width ?? double.infinity);
    return painter;
  }

  void _paintRotated(Canvas canvas, TextPainter tp, Offset center, double degrees) {
    canvas.save();
    canvas.translate(center.dx, center.dy);
    canvas.rotate(degrees * math.pi / 180);
    paintText(canvas, tp, Offset(-tp.width / 2, -tp.height / 2));
    canvas.restore();
  }

  Rect? _manual(LayoutSpec? l, Rect box) {
    if (l == null || !l.edge || !l.edgeY || l.w <= 0 || l.h <= 0) return null;
    return Rect.fromLTWH(box.left + l.x * box.width, box.top + l.y * box.height, l.w * box.width, l.h * box.height);
  }

  // title

  /// The title of the chart: its own text, or the name of the only series.
  String? get _titleText {
    final t = chart.title;
    if (t == null) return null;
    if (t.text.isNotEmpty) return t.text;
    final all = chart.series;
    return all.length == 1 ? _name(all.first) : 'Titre du graphique';
  }

  Rect _title(Canvas canvas, Rect box, Rect inner) {
    final text = _titleText;
    final t = chart.title;
    if (text == null || t == null) return inner;
    final tp = _text(text, t.props, size: 14, bold: true, width: inner.width * 0.8);
    var at = Offset(inner.center.dx - tp.width / 2, inner.top);
    final l = t.layout;
    if (l != null && l.edge && l.edgeY) at = Offset(box.left + l.x * box.width, box.top + l.y * box.height);
    _box(canvas, (at & tp.size).inflate(2), t.shape);
    paintText(canvas, tp, at);
    if (t.overlay || l != null) return inner;
    return Rect.fromLTRB(inner.left, inner.top + tp.height + _pad, inner.right, inner.bottom);
  }

  // legend

  /// What the legend lists: the series, or the points of a plot whose
  /// points each have their color.
  List<_Entry> get _entries {
    final plots = chart.plots.where((p) => p.series.isNotEmpty).toList();
    if (plots.length == 1 && (plots.first.round || plots.first.vary && plots.first.series.length == 1)) {
      final p = plots.first;
      final s = p.series.first;
      final n = _count([s]);
      return [
        for (var i = 0; i < n; i++)
          _Entry(_category(p.series, i), _fill(s, i, vary: true, count: n), _line(s, i, vary: true, count: n), null, false),
      ];
    }
    return [
      for (final p in plots)
        for (final s in p.series)
          if (p.kind == 'line' || p.xy || p.kind == 'radar' && p.style != 'filled' || p.kind == 'stock')
            _Entry(_name(s), null, _line(s, -1, lines: _hasLines(p, s)), _markerOf(p, s, -1), true)
          else
            _Entry(_name(s), _fill(s, -1), _line(s, -1), null, false),
    ];
  }

  Rect _legend(Canvas canvas, Rect box, Rect inner) {
    final legend = chart.legend;
    if (legend == null) return inner;
    final entries = [
      for (final (i, e) in _entries.indexed)
        if (!legend.hidden.contains(i)) e,
    ];
    if (entries.isEmpty) return inner;
    final pos = legend.position;
    final vertical = pos == 'r' || pos == 'l' || pos == 'tr';
    final maxText = vertical ? inner.width * 0.35 : inner.width * 0.8;
    final texts = [for (final e in entries) _text(e.label, legend.text, size: 9, width: maxText, align: TextAlign.left)];
    final key = (texts.isEmpty ? 9.0 : texts.first.preferredLineHeight) * 0.7;
    final keyWidth = entries.any((e) => e.lineKey) ? key * 2.6 : key;
    const gap = 4.0;
    final widths = [for (final t in texts) keyWidth + gap + t.width];
    final height = texts.fold(0.0, (h, t) => math.max(h, t.height));
    late Size size;
    final rows = <List<int>>[];
    if (vertical) {
      for (var i = 0; i < texts.length; i++) {
        rows.add([i]);
      }
      final total = texts.fold(0.0, (h, t) => h + t.height + 2);
      size = Size(widths.fold(0.0, math.max), math.min(total, inner.height));
    } else {
      var row = <int>[];
      var w = 0.0;
      for (var i = 0; i < widths.length; i++) {
        if (row.isNotEmpty && w + widths[i] + 10 > inner.width) {
          rows.add(row);
          row = [];
          w = 0;
        }
        row.add(i);
        w += widths[i] + 10;
      }
      if (row.isNotEmpty) rows.add(row);
      size = Size(
        rows.map((r) => r.fold(0.0, (a, i) => a + widths[i] + 10) - 10).fold(0.0, math.max),
        rows.length * (height + 2),
      );
    }
    var at = switch (pos) {
      'l' => Offset(inner.left, inner.center.dy - size.height / 2),
      't' => Offset(inner.center.dx - size.width / 2, inner.top),
      'b' => Offset(inner.center.dx - size.width / 2, inner.bottom - size.height),
      'tr' => Offset(inner.right - size.width, inner.top),
      _ => Offset(inner.right - size.width, inner.center.dy - size.height / 2),
    };
    final l = legend.layout;
    if (l != null && l.edge && l.edgeY) at = Offset(box.left + l.x * box.width, box.top + l.y * box.height);
    final rect = at & size;
    _box(canvas, rect.inflate(3), legend.shape);
    var y = rect.top;
    for (final row in rows) {
      final rowWidth = row.fold(0.0, (a, i) => a + widths[i] + 10) - 10;
      var x = vertical ? rect.left : rect.left + (rect.width - rowWidth) / 2;
      var rowHeight = 0.0;
      for (final i in row) {
        final t = texts[i];
        if (y + t.height > rect.bottom + 1 && vertical) break;
        final mid = y + t.preferredLineHeight / 2;
        _key(canvas, entries[i], Rect.fromCenter(center: Offset(x + keyWidth / 2, mid), width: keyWidth, height: key));
        paintText(canvas, t, Offset(x + keyWidth + gap, y));
        x += widths[i] + 10;
        rowHeight = math.max(rowHeight, t.height);
      }
      y += rowHeight + 2;
    }
    if (legend.overlay || l != null) return inner;
    return switch (pos) {
      'l' => Rect.fromLTRB(rect.right + _pad, inner.top, inner.right, inner.bottom),
      't' => Rect.fromLTRB(inner.left, rect.bottom + _pad, inner.right, inner.bottom),
      'b' => Rect.fromLTRB(inner.left, inner.top, inner.right, rect.top - _pad),
      _ => Rect.fromLTRB(inner.left, inner.top, rect.left - _pad, inner.bottom),
    };
  }

  void _key(Canvas canvas, _Entry e, Rect r) {
    if (e.lineKey) {
      if (e.line != null) _painter.line(canvas, Path()..moveTo(r.left, r.center.dy)..lineTo(r.right, r.center.dy), e.line);
      if (e.marker != null) _mark(canvas, r.center, e.marker!);
      return;
    }
    final path = Path()..addRect(r);
    _painter.fill(canvas, path, r, e.fill);
    if (e.line != null) _painter.line(canvas, path, {...e.line!, 'w': math.min(((e.line!['w'] as num?) ?? 9525).toDouble(), 19050)});
  }

  // plots on axes

  void _cartesian(Canvas canvas, Rect box, Rect inner, List<PlotSpec> plots) {
    if (plots.isEmpty) return;
    final groups = <String, _Group>{};
    for (final p in plots) {
      final ids = p.axes;
      final a = ids.isNotEmpty ? chart.axis(ids[0]) : null;
      final b = ids.length > 1 ? chart.axis(ids[1]) : null;
      final key = '${a?.id}/${b?.id}';
      (groups[key] ??= _Group(a ?? _fallbackAxis(p, true), b ?? _fallbackAxis(p, false))).plots.add(p);
    }
    final list = groups.values.toList();
    final manual = chart.layout;
    var plot = _manual(manual, box);
    for (final g in list) {
      g.prepare(this, (plot ?? inner).size);
    }
    if (plot == null || manual?.inner != true) {
      var r = plot ?? inner;
      double left = 0, right = 0, top = 0, bottom = 0;
      for (final g in list) {
        for (final axis in [g.cat, g.val]) {
          final need = g.room(this, axis, r.size);
          switch (axis.spec.position) {
            case 'l':
              left = math.max(left, need);
            case 'r':
              right = math.max(right, need);
            case 't':
              top = math.max(top, need);
            default:
              bottom = math.max(bottom, need);
          }
        }
      }
      if (plot == null) {
        r = Rect.fromLTRB(r.left + left, r.top + top, r.right - right, r.bottom - bottom);
        if (r.width < 4 || r.height < 4) return;
        plot = r;
      } else {
        plot = Rect.fromLTRB(plot.left + left, plot.top + top, plot.right - right, plot.bottom - bottom);
      }
      for (final g in list) {
        g.prepare(this, plot.size);
      }
    }
    final area = plot;
    _box(canvas, area, chart.area);
    for (final g in list) {
      _grid(canvas, area, g);
    }
    for (final g in list) {
      for (final p in g.plots.where((p) => p.kind == 'area')) {
        _areas(canvas, area, g, p);
      }
      for (final p in g.plots.where((p) => p.kind == 'bar')) {
        _bars(canvas, area, g, p);
      }
      for (final p in g.plots.where((p) => p.kind != 'area' && p.kind != 'bar')) {
        _lines(canvas, area, g, p);
      }
    }
    for (final g in list) {
      _axes(canvas, area, g);
    }
  }

  /// An axis the file does not have: categories along the bottom, values
  /// up the left.
  AxisSpec _fallbackAxis(PlotSpec p, bool cat) {
    final pos = (cat != p.horizontal) ? 'b' : 'l';
    return AxisSpec({'id': cat ? -1 : -2, 'kind': cat && !p.xy ? 'cat' : 'val', 'pos': pos, 'delete': true});
  }

  void _grid(Canvas canvas, Rect area, _Group g) {
    for (final a in [g.val, g.cat]) {
      final spec = a.spec;
      for (final (has, shape, minor) in [(spec.hasMinorGrid, spec.minorGrid, true), (spec.hasGrid, spec.grid, false)]) {
        if (!has) continue;
        final line = {'w': 9525, 'fill': {'solid': {'rgb': minor ? 'F2F2F2' : 'D9D9D9'}}, ...?_map(shape?['line'])};
        for (final at in a.gridAt(minor)) {
          final p = Path();
          if (spec.vertical) {
            final y = area.bottom - at * area.height;
            p..moveTo(area.left, y)..lineTo(area.right, y);
          } else {
            final x = area.left + at * area.width;
            p..moveTo(x, area.top)..lineTo(x, area.bottom);
          }
          _painter.line(canvas, p, line);
        }
      }
    }
  }

  void _axes(Canvas canvas, Rect area, _Group g) {
    for (final a in [g.cat, g.val]) {
      final spec = a.spec;
      if (spec.deleted) continue;
      final other = identical(a, g.cat) ? g.val : g.cat;
      final cross = other.crossing(spec, this);
      final horizontal = !spec.vertical;
      final linePos = horizontal ? area.bottom - cross * area.height : area.left + cross * area.width;
      final line = spec.shape?['line'];
      final stroke = line is Map<String, Object?> ? line : {'w': 9525, 'fill': {'solid': {'rgb': 'BFBFBF'}}};
      final path = Path();
      if (horizontal) {
        path..moveTo(area.left, linePos)..lineTo(area.right, linePos);
      } else {
        path..moveTo(linePos, area.top)..lineTo(linePos, area.bottom);
      }
      _painter.line(canvas, path, stroke);
      if (spec.tick != 'none') {
        final (outward, inward) = switch (spec.tick) {
          'in' => (0.0, _tick),
          'cross' => (_tick, _tick),
          _ => (_tick, 0.0),
        };
        final sign = spec.position == 'l' || spec.position == 'b' ? 1.0 : -1.0;
        final ticks = Path();
        for (final at in a.tickAt()) {
          if (horizontal) {
            final x = area.left + at * area.width;
            ticks..moveTo(x, linePos - inward * sign)..lineTo(x, linePos + outward * sign);
          } else {
            final y = area.bottom - at * area.height;
            ticks..moveTo(linePos + inward * sign, y)..lineTo(linePos - outward * sign, y);
          }
        }
        _painter.line(canvas, ticks, stroke);
      }
      if (spec.labels != 'none') {
        final edge = switch (spec.labels) {
          'low' => horizontal ? area.bottom : area.left,
          'high' => horizontal ? area.top : area.right,
          _ => linePos,
        };
        final after = spec.position == 'b' || spec.position == 'r';
        final outside = spec.tick == 'out' || spec.tick == 'cross' ? _tick : 0.0;
        for (final (at, tp, rot) in a.labelsFor(this, horizontal ? area.width : area.height)) {
          if (horizontal) {
            final x = area.left + at * area.width;
            if (rot != 0) {
              final rad = rot * math.pi / 180;
              final w = (tp.width * math.cos(rad)).abs() + (tp.height * math.sin(rad)).abs();
              final h = (tp.width * math.sin(rad)).abs() + (tp.height * math.cos(rad)).abs();
              final cy = after ? edge + outside + 2 + h / 2 : edge - outside - 2 - h / 2;
              final cx = rot < 0 ? x - w / 2 + tp.height / 2 : x + w / 2 - tp.height / 2;
              _paintRotated(canvas, tp, Offset(cx, cy), rot);
            } else {
              paintText(canvas, tp, Offset(x - tp.width / 2, after ? edge + outside + 2 : edge - outside - 2 - tp.height));
            }
          } else {
            final y = area.bottom - at * area.height;
            paintText(canvas, tp, Offset(after ? edge + outside + 3 : edge - outside - 3 - tp.width, y - tp.height / 2));
          }
        }
      }
      final title = spec.title;
      if (title != null) {
        final tp = _text(title.text.isEmpty ? 'Titre de l’axe' : title.text, title.props, size: 10, bold: true, width: horizontal ? area.width : area.height);
        final room = a.labelRoom;
        if (horizontal) {
          final y = spec.position == 't' ? area.top - room - _pad - tp.height : area.bottom + room + _pad;
          paintText(canvas, tp, Offset(area.center.dx - tp.width / 2, y));
        } else {
          final degrees = title.rotation ?? -90;
          final x = spec.position == 'r' ? area.right + room + _pad + tp.height / 2 : area.left - room - _pad - tp.height / 2;
          _paintRotated(canvas, tp, Offset(x, area.center.dy), degrees);
        }
      }
    }
  }

  void _bars(Canvas canvas, Rect area, _Group g, PlotSpec p) {
    final n = g.count;
    if (n == 0) return;
    final series = p.series;
    final along = g.cat.spec.vertical ? area.height : area.width;
    final slot = along / n;
    final clusters = p.stacked ? 1 : series.length;
    final overlap = p.overlap.clamp(-100, 100) / 100;
    final width = slot / (clusters - (clusters - 1) * overlap + p.gap.clamp(0, 500) / 100);
    final base = g.val.base(this);
    final pos = List<double>.filled(n, 0), neg = List<double>.filled(n, 0);
    final totals = p.percent ? g.totals(this, p) : null;
    final labels = <(Rect, SeriesSpec, int, double)>[];
    for (final (k, s) in series.indexed) {
      final vary = p.vary && series.length == 1;
      for (var i = 0; i < n; i++) {
        var v = _value(s, i);
        if (v == null) continue;
        final raw = v;
        if (totals != null) v = totals[i] == 0 ? 0 : v / totals[i];
        var from = base;
        var to = v;
        if (p.stacked) {
          if (v >= 0) {
            from = pos[i];
            pos[i] += v;
            to = pos[i];
          } else {
            from = neg[i];
            neg[i] += v;
            to = neg[i];
          }
        }
        final offset = (slot - (clusters - (clusters - 1) * overlap) * width) / 2 + (p.stacked ? 0 : k * width * (1 - overlap));
        final start = g.cat.slot(i, n) * slot + offset;
        final a = g.val.at(from), b = g.val.at(to);
        final Rect rect;
        if (g.cat.spec.vertical) {
          rect = Rect.fromLTRB(area.left + math.min(a, b) * area.width, area.bottom - start - width, area.left + math.max(a, b) * area.width, area.bottom - start);
        } else {
          rect = Rect.fromLTRB(area.left + start, area.bottom - math.max(a, b) * area.height, area.left + start + width, area.bottom - math.min(a, b) * area.height);
        }
        final shape = Path()..addRect(rect);
        var fill = _fill(s, i, vary: vary, count: n);
        final invert = _pointInvert(s, i);
        if (raw < 0 && invert) fill = {'solid': {'rgb': 'FFFFFF'}};
        _painter.fill(canvas, shape, rect, fill);
        final line = _line(s, i, vary: vary, count: n);
        if (line != null) _painter.line(canvas, shape, line);
        labels.add((rect, s, i, raw));
      }
    }
    for (final (rect, s, i, raw) in labels) {
      final l = (s.labels ?? p.labels)?.at(i);
      if (l == null || !l.any) continue;
      final tp = _text(_labelText(l, s, p, i, raw, null), l.text, size: 9);
      final horizontal = g.cat.spec.vertical;
      final up = raw >= 0;
      final position = l.position.isEmpty ? (p.stacked ? 'ctr' : 'outEnd') : l.position;
      final Offset c;
      if (horizontal) {
        final end = up ? rect.right : rect.left;
        final start = up ? rect.left : rect.right;
        final dir = up ? 1 : -1;
        c = switch (position) {
          'ctr' => rect.center,
          'inEnd' => Offset(end - dir * (tp.width / 2 + 3), rect.center.dy),
          'inBase' => Offset(start + dir * (tp.width / 2 + 3), rect.center.dy),
          _ => Offset(end + dir * (tp.width / 2 + 3), rect.center.dy),
        };
      } else {
        final end = up ? rect.top : rect.bottom;
        final start = up ? rect.bottom : rect.top;
        final dir = up ? -1 : 1;
        c = switch (position) {
          'ctr' => rect.center,
          'inEnd' => Offset(rect.center.dx, end - dir * (tp.height / 2 + 2)),
          'inBase' => Offset(rect.center.dx, start + dir * (tp.height / 2 + 2)),
          _ => Offset(rect.center.dx, end + dir * (tp.height / 2 + 2)),
        };
      }
      paintText(canvas, tp, c - Offset(tp.width / 2, tp.height / 2));
    }
  }

  bool _pointInvert(SeriesSpec s, int i) {
    final own = s.points[i]?['invert'];
    return own is bool ? own : s.invert;
  }

  void _areas(Canvas canvas, Rect area, _Group g, PlotSpec p) {
    final n = g.count;
    if (n == 0) return;
    final stack = List<double>.filled(n, 0);
    final totals = p.percent ? g.totals(this, p) : null;
    final base = g.val.base(this);
    final layers = <(Path, SeriesSpec)>[];
    for (final s in p.series) {
      final top = <Offset>[], bottom = <Offset>[];
      for (var i = 0; i < n; i++) {
        var v = _value(s, i) ?? 0;
        if (totals != null) v = totals[i] == 0 ? 0 : v / totals[i];
        final from = p.stacked ? stack[i] : base;
        final to = p.stacked ? stack[i] + v : v;
        if (p.stacked) stack[i] = to;
        final x = g.cat.point(i, n);
        top.add(_xy(area, g, x, g.val.at(to)));
        bottom.add(_xy(area, g, x, g.val.at(from)));
      }
      final path = Path()..addPolygon([...top, ...bottom.reversed], true);
      layers.add((path, s));
    }
    for (final (path, s) in layers) {
      _painter.fill(canvas, path, area, _fill(s, -1));
      final line = _line(s, -1);
      if (line != null) _painter.line(canvas, path, line);
    }
  }

  /// A point of the plot from where it falls along the category and value
  /// axes, from 0 to 1 each.
  Offset _xy(Rect area, _Group g, double along, double value) {
    if (g.cat.spec.vertical) return Offset(area.left + value * area.width, area.bottom - along * area.height);
    return Offset(area.left + along * area.width, area.bottom - value * area.height);
  }

  bool _hasLines(PlotSpec p, SeriesSpec s) => switch (p.kind) {
    'scatter' => p.style != 'marker',
    'stock' => false,
    'bubble' => false,
    _ => true,
  };

  MarkerSpec? _markerOf(PlotSpec p, SeriesSpec s, int point) {
    final own = MarkerSpec.fromJson(s.points[point]?['marker']) ?? s.marker;
    final shows = switch (p.kind) {
      'line' => p.markers,
      'scatter' => p.style != 'line' && p.style != 'smooth',
      'radar' => p.style == 'marker',
      'stock' => true,
      _ => false,
    };
    if (own == null && !shows) return null;
    var symbol = own?.symbol ?? '';
    if (symbol == 'none') return null;
    if (symbol.isEmpty || symbol == 'auto') {
      if (own == null && !shows) return null;
      const auto = ['diamond', 'square', 'triangle', 'x', 'star', 'circle', 'plus', 'dash', 'dot'];
      symbol = auto[s.index % auto.length];
    }
    final color = _line(s, -1, lines: true)?['fill'] ?? _fill(s, -1);
    return MarkerSpec({
      'symbol': symbol,
      'size': own?.size ?? 5,
      'shape': {
        'fill': own?.shape?['fill'] ?? color,
        'line': {'w': 9525, 'fill': color, ...?_map(own?.shape?['line'])},
      },
    });
  }

  void _lines(Canvas canvas, Rect area, _Group g, PlotSpec p) {
    final n = g.count;
    final stack = List<double>.filled(n, 0);
    final totals = p.percent ? g.totals(this, p) : null;
    final sizes = p.kind == 'bubble' ? g.bubbleMax(this, p) : 0.0;
    for (final s in p.series) {
      final points = <Offset?>[];
      final count = math.max(_val(s)?.length ?? 0, p.xy ? 0 : n);
      for (var i = 0; i < count; i++) {
        var v = _value(s, i);
        if (v == null) {
          points.add(null);
          continue;
        }
        if (totals != null) v = totals[i] == 0 ? 0 : v / totals[i];
        if (p.stacked && i < stack.length) {
          stack[i] += v;
          v = stack[i];
        }
        final double along;
        if (p.xy) {
          final xs = _cat(s);
          final x = xs == null || xs.isText ? (i + 1).toDouble() : xs.number(i);
          if (x == null) {
            points.add(null);
            continue;
          }
          along = g.cat.at(x);
        } else {
          along = g.cat.point(i, n);
        }
        points.add(_xy(area, g, along, g.val.at(v)));
      }
      if (p.kind == 'bubble') {
        for (final (i, c) in points.indexed) {
          final size = _resolve(s.size)?.number(i);
          if (c == null || size == null || size <= 0 || sizes <= 0) continue;
          final r = math.sqrt(size / sizes) * math.min(area.width, area.height) * 0.125;
          final path = Path()..addOval(Rect.fromCircle(center: c, radius: r));
          _painter.fill(canvas, path, path.getBounds(), _fill(s, i, vary: p.vary, count: count));
          final line = _line(s, i);
          if (line != null) _painter.line(canvas, path, line);
        }
        continue;
      }
      final line = _line(s, -1, lines: _hasLines(p, s));
      if (line != null) _painter.line(canvas, _polyline(points, s.smooth || p.style.startsWith('smooth')), line);
      for (final (i, c) in points.indexed) {
        if (c == null) continue;
        final m = _markerOf(p, s, i);
        if (m != null) _mark(canvas, c, m);
      }
      for (final (i, c) in points.indexed) {
        final l = (s.labels ?? p.labels)?.at(i);
        if (c == null || l == null || !l.any) continue;
        final v = _value(s, i) ?? 0;
        final tp = _text(_labelText(l, s, p, i, v, null), l.text, size: 9);
        final at = switch (l.position) {
          'ctr' => c - Offset(tp.width / 2, tp.height / 2),
          'l' => c - Offset(tp.width + 5, tp.height / 2),
          't' => c - Offset(tp.width / 2, tp.height + 4),
          'b' => c + Offset(-tp.width / 2, 4),
          _ => c + Offset(5, -tp.height / 2),
        };
        paintText(canvas, tp, at);
      }
    }
  }

  /// The path through points, broken where one is missing unless blanks
  /// are spanned; smoothed as Office smooths, by Catmull-Rom curves.
  Path _polyline(List<Offset?> points, bool smooth) {
    final path = Path();
    final runs = <List<Offset>>[];
    var run = <Offset>[];
    for (final p in points) {
      if (p == null) {
        if (chart.blanks != 'span' && run.isNotEmpty) {
          runs.add(run);
          run = [];
        }
        continue;
      }
      run.add(p);
    }
    if (run.isNotEmpty) runs.add(run);
    for (final r in runs) {
      path.moveTo(r.first.dx, r.first.dy);
      for (var i = 1; i < r.length; i++) {
        if (!smooth) {
          path.lineTo(r[i].dx, r[i].dy);
          continue;
        }
        final p0 = r[math.max(i - 2, 0)], p1 = r[i - 1], p2 = r[i], p3 = r[math.min(i + 1, r.length - 1)];
        final c1 = p1 + (p2 - p0) / 6, c2 = p2 - (p3 - p1) / 6;
        path.cubicTo(c1.dx, c1.dy, c2.dx, c2.dy, p2.dx, p2.dy);
      }
    }
    return path;
  }

  void _mark(Canvas canvas, Offset c, MarkerSpec m) {
    final size = (m.size ?? 5).clamp(2, 72).toDouble();
    final r = size / 2;
    final box = Rect.fromCircle(center: c, radius: r);
    final path = Path();
    var filled = true;
    switch (m.symbol) {
      case 'circle':
        path.addOval(box);
      case 'square':
        path.addRect(box);
      case 'diamond':
        path.addPolygon([box.topCenter, box.centerRight, box.bottomCenter, box.centerLeft], true);
      case 'triangle':
        path.addPolygon([box.topCenter, box.bottomRight, box.bottomLeft], true);
      case 'x':
        path..moveTo(box.left, box.top)..lineTo(box.right, box.bottom)..moveTo(box.right, box.top)..lineTo(box.left, box.bottom);
        filled = false;
      case 'star':
        path..moveTo(box.left, box.top)..lineTo(box.right, box.bottom)..moveTo(box.right, box.top)..lineTo(box.left, box.bottom)
          ..moveTo(c.dx, box.top)..lineTo(c.dx, box.bottom);
        filled = false;
      case 'plus':
        path..moveTo(c.dx, box.top)..lineTo(c.dx, box.bottom)..moveTo(box.left, c.dy)..lineTo(box.right, c.dy);
        filled = false;
      case 'dash':
        path.addRect(Rect.fromCenter(center: c, width: size, height: math.max(size / 4, 1)));
      case 'dot':
        path.addRect(Rect.fromCenter(center: c, width: math.max(size / 4, 1), height: math.max(size / 4, 1)));
      default:
        return;
    }
    final shape = m.shape;
    if (filled) _painter.fill(canvas, path, box, _map(shape?['fill']));
    final line = _map(shape?['line']);
    if (line != null) _painter.line(canvas, path, filled ? line : {...line, 'fill': line['fill'] ?? shape?['fill']});
  }

  String _labelText(LabelsSpec l, SeriesSpec s, PlotSpec p, int i, double value, double? share) {
    final parts = <String>[
      if (l.series) _name(s),
      if (l.category) _category([s], i),
      if (l.value) _format(value, l.format.isNotEmpty ? l.format : (_val(s)?.format ?? '')),
      if (l.percent && share != null) _format(share, l.format.isNotEmpty && l.format.contains('%') ? l.format : '0%'),
    ];
    return parts.join(l.separator ?? (l.category && (l.value || l.percent) && p.round ? '\n' : '; '));
  }

  // round plots

  void _round(Canvas canvas, PlotSpec p, Rect area) {
    final doughnut = p.kind == 'doughnut';
    final series = doughnut ? p.series : p.series.take(1).toList();
    final n = _count(series);
    if (n == 0) return;
    final labelled = series.any((s) => (s.labels ?? p.labels)?.any ?? false);
    final outside = labelled && series.any((s) {
      final pos = (s.labels ?? p.labels)?.position ?? '';
      return pos == 'outEnd' || pos == 'bestFit' || pos.isEmpty;
    });
    var radius = math.min(area.width, area.height) / 2 * (outside && !doughnut ? 0.8 : 0.95);
    final explode = series.expand((s) => [s.explosion, for (final pt in s.points.values) (pt['explosion'] as num?)?.toInt() ?? 0]).fold(0, math.max);
    radius /= 1 + explode / 100;
    final center = area.center;
    final hole = doughnut ? p.hole.clamp(10, 90) / 100 * radius : 0.0;
    final ring = (radius - hole) / series.length;
    final labels = <(TextPainter, Offset)>[];
    for (final (k, s) in series.indexed) {
      final values = [for (var i = 0; i < n; i++) math.max(_value(s, i) ?? 0, 0.0)];
      final total = values.fold(0.0, (a, b) => a + b);
      if (total <= 0) continue;
      var start = -math.pi / 2 + p.angle * math.pi / 180;
      final outer = hole + ring * (k + 1), inner = hole + ring * k;
      for (var i = 0; i < n; i++) {
        final sweep = values[i] / total * 2 * math.pi;
        if (sweep <= 0) continue;
        final mid = start + sweep / 2;
        final ex = ((s.points[i]?['explosion'] as num?)?.toDouble() ?? s.explosion.toDouble()) / 100 * radius;
        final c = center + Offset(math.cos(mid), math.sin(mid)) * ex;
        final path = Path();
        if (inner > 0) {
          path.arcTo(Rect.fromCircle(center: c, radius: outer), start, sweep, true);
          path.arcTo(Rect.fromCircle(center: c, radius: inner), start + sweep, -sweep, false);
          path.close();
        } else if (sweep >= 2 * math.pi - 1e-9) {
          path.addOval(Rect.fromCircle(center: c, radius: outer));
        } else {
          path.moveTo(c.dx, c.dy);
          path.arcTo(Rect.fromCircle(center: c, radius: outer), start, sweep, false);
          path.close();
        }
        final vary = p.vary || p.round;
        _painter.fill(canvas, path, Rect.fromCircle(center: c, radius: outer), _fill(s, i, vary: vary, count: n));
        final line = _line(s, i, vary: vary, count: n);
        if (line != null) _painter.line(canvas, path, line);
        final l = (s.labels ?? p.labels)?.at(i);
        if (l != null && l.any) {
          final tp = _text(_labelText(l, s, p, i, values[i], values[i] / total), l.text, size: 9);
          final pos = l.position;
          final double r;
          if (doughnut || pos == 'ctr') {
            r = (inner + outer) / 2;
          } else if (pos == 'inEnd') {
            r = outer * 0.7;
          } else if (pos == 'outEnd' || pos == 'bestFit' && sweep < 0.5 || pos.isEmpty && sweep < 0.5) {
            r = outer + 4 + math.max(tp.width, tp.height) / 2;
          } else {
            r = outer * 0.6;
          }
          labels.add((tp, c + Offset(math.cos(mid), math.sin(mid)) * r));
        }
        start += sweep;
      }
    }
    for (final (tp, at) in labels) {
      paintText(canvas, tp, at - Offset(tp.width / 2, tp.height / 2));
    }
  }

  void _radar(Canvas canvas, PlotSpec p, Rect area) {
    final n = _count(p.series);
    if (n < 2) return;
    final valAxis = p.axes.length > 1 ? chart.axis(p.axes[1]) : null;
    var lo = 0.0, hi = 0.0;
    for (final s in p.series) {
      for (var i = 0; i < n; i++) {
        final v = _value(s, i);
        if (v == null) continue;
        lo = math.min(lo, v);
        hi = math.max(hi, v);
      }
    }
    final labels = [for (var i = 0; i < n; i++) _text(_category(p.series, i), const {}, size: 9)];
    final room = labels.fold(0.0, (a, t) => math.max(a, math.max(t.width, t.height))) + 4;
    final radius = math.max(math.min(area.width, area.height) / 2 - room, 4.0);
    final ticks = (radius / 16).floor().clamp(2, 6);
    final scale = Scale.auto(lo, hi, min: valAxis?.min, max: valAxis?.max, major: valAxis?.major ?? 0, ticks: ticks);
    final center = area.center;
    Offset at(int i, double f) {
      final a = -math.pi / 2 + i * 2 * math.pi / n;
      return center + Offset(math.cos(a), math.sin(a)) * (radius * f.clamp(0, 1));
    }

    final grid = {'w': 9525, 'fill': {'solid': {'rgb': 'D9D9D9'}}, ...?_map(valAxis?.grid?['line'])};
    for (final t in scale.ticks) {
      final f = scale.fraction(t);
      _painter.line(canvas, Path()..addPolygon([for (var i = 0; i < n; i++) at(i, f)], true), grid);
    }
    for (var i = 0; i < n; i++) {
      _painter.line(canvas, Path()..moveTo(center.dx, center.dy)..lineTo(at(i, 1).dx, at(i, 1).dy), grid);
      final tp = labels[i];
      final a = -math.pi / 2 + i * 2 * math.pi / n;
      final c = center + Offset(math.cos(a), math.sin(a)) * (radius + 4 + math.max(tp.width, tp.height) / 2);
      paintText(canvas, tp, c - Offset(tp.width / 2, tp.height / 2));
    }
    if (valAxis == null || !valAxis.deleted) {
      for (final t in scale.ticks) {
        final tp = _text(_format(t, valAxis?.format ?? ''), valAxis?.text ?? const {}, size: 8);
        final c = at(0, scale.fraction(t));
        paintText(canvas, tp, c + Offset(-tp.width - 3, -tp.height / 2));
      }
    }
    for (final s in p.series) {
      final points = [for (var i = 0; i < n; i++) at(i, scale.fraction(_value(s, i) ?? scale.min))];
      final path = Path()..addPolygon(points, true);
      if (p.style == 'filled') {
        _painter.fill(canvas, path, area, _fill(s, -1));
      } else {
        final line = _line(s, -1, lines: true);
        if (line != null) _painter.line(canvas, path, line);
        for (final (i, c) in points.indexed) {
          final m = _markerOf(p, s, i);
          if (m != null) _mark(canvas, c, m);
        }
      }
    }
  }
}

class _Entry {
  _Entry(this.label, this.fill, this.line, this.marker, this.lineKey);

  final String label;
  final Map<String, Object?>? fill;
  final Map<String, Object?>? line;
  final MarkerSpec? marker;
  final bool lineKey;
}

/// The plots drawn on a pair of axes, and how those axes map values.
class _Group {
  _Group(AxisSpec cat, AxisSpec val) : cat = _Axis(cat), val = _Axis(val);

  final _Axis cat;
  final _Axis val;
  final plots = <PlotSpec>[];
  int count = 0;

  bool get xy => plots.any((p) => p.xy);

  void prepare(ChartPainter c, Size size) {
    final series = [for (final p in plots) ...p.series];
    count = c._count(series);
    cat._labels = null;
    val._labels = null;
    double length(Size s, _Axis a) => a.spec.vertical ? s.height : s.width;
    var lo = double.infinity, hi = -double.infinity;
    void see(double v) {
      lo = math.min(lo, v);
      hi = math.max(hi, v);
    }

    for (final p in plots) {
      if (p.percent) {
        final t = totals(c, p);
        var hasNeg = false;
        for (final s in p.series) {
          for (var i = 0; i < count; i++) {
            if ((c._value(s, i) ?? 0) < 0 && t[i] != 0) hasNeg = true;
          }
        }
        see(hasNeg ? -1 : 0);
        see(1);
        continue;
      }
      if (p.stacked) {
        final pos = List<double>.filled(count, 0), neg = List<double>.filled(count, 0);
        for (final s in p.series) {
          for (var i = 0; i < count; i++) {
            final v = c._value(s, i) ?? 0;
            if (v >= 0 || p.kind != 'bar') {
              pos[i] += v;
              see(pos[i]);
            } else {
              neg[i] += v;
              see(neg[i]);
            }
          }
        }
        see(0);
        continue;
      }
      for (final s in p.series) {
        for (var i = 0; i < (c._val(s)?.length ?? 0); i++) {
          final v = c._value(s, i);
          if (v != null) see(v);
        }
      }
    }
    if (lo > hi) (lo, hi) = (0, 1);
    final percent = plots.any((p) => p.percent);
    val.format = percent && val.spec.format.isEmpty || percent && val.spec.linked
        ? '0%'
        : (val.spec.linked || val.spec.format.isEmpty ? (_firstFormat(c, series) ?? val.spec.format) : val.spec.format);
    _fit(c, val, lo, hi, length(size, val), percent: percent);
    if (xy) {
      var xlo = double.infinity, xhi = -double.infinity;
      for (final s in series) {
        final x = c._cat(s);
        final n = c._val(s)?.length ?? 0;
        for (var i = 0; i < n; i++) {
          final v = x == null || x.isText ? (i + 1).toDouble() : x.number(i);
          if (v == null) continue;
          xlo = math.min(xlo, v);
          xhi = math.max(xhi, v);
        }
      }
      if (xlo > xhi) (xlo, xhi) = (0, 1);
      cat.format = cat.spec.linked || cat.spec.format.isEmpty ? (series.isEmpty ? '' : c._cat(series.first)?.format ?? '') : cat.spec.format;
      _fit(c, cat, xlo, xhi, length(size, cat));
    } else {
      cat.categories = count;
      cat.names = [for (var i = 0; i < count; i++) c._category(series, i)];
      cat.between = val.spec.json['between'] != null ? val.spec.between : !plots.every((p) => p.kind == 'area');
    }
    cat.labelStyle = cat.spec.text;
    val.labelStyle = val.spec.text;
  }

  static String? _firstFormat(ChartPainter c, List<SeriesSpec> series) {
    for (final s in series) {
      final f = c._val(s)?.format;
      if (f != null && f.isNotEmpty) return f;
    }
    return null;
  }

  /// Gives an axis the scale of values from [lo] to [hi], with no more
  /// ticks than its labels leave room for.
  static void _fit(ChartPainter c, _Axis a, double lo, double hi, double length, {bool percent = false}) {
    final s = a.spec;
    var ticks = (length / 22).floor().clamp(2, 10);
    while (true) {
      a.scale = Scale.auto(lo, hi,
          min: s.min ?? (percent ? lo : null), max: s.max ?? (percent ? hi : null), major: s.major, log: s.log, ticks: ticks);
      if (s.vertical || s.major > 0 || ticks <= 2) return;
      final values = a.scale!.ticks;
      final widest = values.map((t) => c._text(c._format(t, a.format), s.text, size: 9).width).fold(0.0, math.max);
      if (values.length * (widest + 6) <= length) return;
      ticks--;
    }
  }

  /// The sum of the values of each category, for plots in percent.
  List<double> totals(ChartPainter c, PlotSpec p) {
    final out = List<double>.filled(count, 0);
    for (final s in p.series) {
      for (var i = 0; i < count; i++) {
        out[i] += (c._value(s, i) ?? 0).abs();
      }
    }
    return out;
  }

  double bubbleMax(ChartPainter c, PlotSpec p) {
    var m = 0.0;
    for (final s in p.series) {
      final d = c._resolve(s.size);
      if (d == null) continue;
      for (var i = 0; i < d.length; i++) {
        m = math.max(m, d.number(i) ?? 0);
      }
    }
    return m;
  }

  /// The room the labels and title of an axis take beside the plot.
  double room(ChartPainter c, _Axis a, Size size) {
    if (a.spec.deleted) return 0;
    var r = 0.0;
    if (a.spec.labels != 'none') {
      final labels = a.labelsFor(c, a.spec.vertical ? size.height : size.width);
      for (final (_, tp, rot) in labels) {
        final rad = rot * math.pi / 180;
        final w = (tp.width * math.cos(rad)).abs() + (tp.height * math.sin(rad)).abs();
        final h = (tp.width * math.sin(rad)).abs() + (tp.height * math.cos(rad)).abs();
        r = math.max(r, a.spec.vertical ? w : h);
      }
      r += (a.spec.tick == 'out' || a.spec.tick == 'cross' ? ChartPainter._tick : 0) + 3;
    }
    a.labelRoom = r;
    final title = a.spec.title;
    if (title != null) {
      final tp = c._text(title.text.isEmpty ? 'Titre de l’axe' : title.text, title.props, size: 10, bold: true);
      r += tp.height + ChartPainter._pad;
    }
    return r;
  }
}

/// An axis being laid out: a scale for values, slots for categories.
class _Axis {
  _Axis(this.spec);

  final AxisSpec spec;
  Scale? scale;
  String format = '';
  int categories = 0;
  List<String> names = const [];
  bool between = true;
  Props labelStyle = const {};
  double labelRoom = 0;
  List<(double, TextPainter, double)>? _labels;
  double _labelsFor = -1;

  /// Where a value falls, from 0 to 1 along the axis.
  double at(double v) {
    final s = scale;
    if (s == null) return 0;
    final f = s.fraction(v);
    return spec.reverse ? 1 - f : f;
  }

  /// Where a category's point falls, from 0 to 1.
  double point(int i, int n) {
    if (n <= 0) return 0;
    final f = between ? (i + 0.5) / n : (n == 1 ? 0.5 : i / (n - 1));
    return spec.reverse ? 1 - f : f;
  }

  /// The rank of the slot of a category from the start of the axis.
  int slot(int i, int n) => spec.reverse ? n - 1 - i : i;

  /// The value bars and areas grow from.
  double base(ChartPainter c) {
    final s = scale;
    if (s == null) return 0;
    if (s.log > 1) return s.min;
    return s.clamp(0);
  }

  /// Where the axis crossing this one sits along this one, from 0 to 1.
  double crossing(AxisSpec other, ChartPainter c) {
    final s = scale;
    if (s == null) {
      final f = switch (other.crosses) {
        'max' => 1.0,
        _ => 0.0,
      };
      return spec.reverse ? 1 - f : f;
    }
    final v = switch (other.crosses) {
      'min' => s.min,
      'max' => s.max,
      _ => other.crossesAt ?? (s.log > 1 ? s.min : s.clamp(0)),
    };
    return at(s.clamp(v));
  }

  List<double> tickAt() {
    if (scale != null) return [for (final t in scale!.ticks) at(t)];
    final n = categories;
    if (n == 0) return const [];
    if (between) return [for (var i = 0; i <= n; i++) i / n];
    return [for (var i = 0; i < n; i++) n == 1 ? 0.5 : i / (n - 1)];
  }

  List<double> gridAt(bool minor) {
    if (scale != null && minor) {
      final s = scale!;
      if (s.log > 1) return const [];
      final step = spec.json['minor'] is num ? (spec.json['minor'] as num).toDouble() : s.step / 5;
      if (step <= 0) return const [];
      final n = ((s.max - s.min) / step).floor().clamp(0, 500);
      return [for (var i = 0; i <= n; i++) at(s.min + i * step)];
    }
    return tickAt();
  }

  /// The labels along the axis: where each goes, its text laid out and its
  /// rotation in degrees.
  List<(double, TextPainter, double)> labelsFor(ChartPainter c, double length) {
    if (_labels != null && _labelsFor == length) return _labels!;
    final out = <(double, TextPainter, double)>[];
    final horizontal = !spec.vertical;
    if (scale != null) {
      for (final t in scale!.ticks) {
        out.add((at(t), c._text(c._format(t, format), labelStyle, size: 9), 0));
      }
    } else if (categories > 0) {
      final n = categories;
      final slot = length / n;
      var skip = spec.skip > 0 ? spec.skip : 1;
      var rotation = spec.rotation ?? 0;
      var width = horizontal ? slot * skip : length * 0.4;
      if (horizontal && spec.skip <= 0) {
        var widest = 0.0;
        for (final name in names) {
          final tp = c._text(name, labelStyle, size: 9);
          widest = math.max(widest, tp.minIntrinsicWidth);
        }
        if (widest > slot && spec.rotation == null) {
          rotation = -45;
          final tp = c._text('M', labelStyle, size: 9);
          skip = math.max(1, (tp.height * 1.4 / slot).ceil());
        }
        width = rotation != 0 ? length * 0.3 : slot * skip;
      }
      for (var i = 0; i < n; i += skip) {
        out.add((point(i, n), c._text(names[i], labelStyle, size: 9, width: math.max(width, 8)), rotation));
      }
    }
    _labelsFor = length;
    return _labels = out;
  }
}

Map<String, Object?>? _map(Object? v) => v is Map<String, Object?> ? v : null;
