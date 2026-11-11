part of 'chart_painter.dart';

const _gridColor = Color(0xFFD9D9D9);
const _axisColor = Color(0xFFBFBFBF);
const _inkColor = Color(0xFF595959);
const _whiteText = {'fill': '{"rgb":"FFFFFF"}'};

/// The charts Excel 2016 added.
extension _Extended on ChartPainter {
  void _extended(Canvas canvas, PlotSpec p, Rect area) {
    _box(canvas, area, chart.area);
    final bars = p.series.where((s) => !(s.options?.line ?? false)).toList();
    if (bars.isEmpty) return;
    final s = bars.first;
    switch (p.kind) {
      case 'waterfall':
        _waterfall(canvas, p, s, area);
      case 'histogram' || 'pareto':
        _histogram(canvas, p, s, area);
      case 'boxWhisker':
        _boxes(canvas, p, bars, area);
      case 'funnel':
        _funnel(canvas, s, area);
      case 'treemap':
        _treemap(canvas, s, area);
      case 'sunburst':
        _sunburst(canvas, s, area);
      default:
        _substitute(canvas, s, area);
    }
  }

  /// What the legend lists for the charts Excel 2016 added.
  List<_Entry>? get _extendedEntries {
    final p = chart.plots.where((p) => p.extended && p.series.isNotEmpty).firstOrNull;
    if (p == null) return null;
    final s = p.series.firstWhere((s) => !(s.options?.line ?? false), orElse: () => p.series.first);
    Map<String, Object?> solid(int i, int n) => {'solid': _auto(i, n)};
    switch (p.kind) {
      case 'waterfall':
        return [
          for (final (i, name) in const ['Augmentation', 'Diminution', 'Total'].indexed) _Entry(name, solid(i, 3), null, null, false),
        ];
      case 'treemap' || 'sunburst':
        final tops = hierarchy(_cat(s), [for (var i = 0; i < (_val(s)?.length ?? 0); i++) _value(s, i)]).children;
        return [for (final (i, b) in tops.indexed) _Entry(b.label, solid(i, tops.length), null, null, false)];
      case 'pareto':
        return [_Entry(_name(s), _fill(s, -1), _line(s, -1), null, false), _Entry('Ligne de Pareto', null, _paretoLine(), null, true)];
    }
    return null;
  }

  Map<String, Object?> _paretoLine() => {
    'w': 28575,
    'cap': 'rnd',
    'fill': {'solid': _auto(1, 2)},
  };

  AxisSpec? _axisOf(PlotSpec p, String kind, [int skip = 0]) {
    final found = [
      for (final id in p.axes)
        if (chart.axis(id) case final a? when a.kind == kind) a,
    ];
    return skip < found.length ? found[skip] : null;
  }

  Path _rectPath(Rect r) => Path()..addRect(r);

  void _fillRect(Canvas canvas, Rect r, Map<String, Object?>? fill, {Map<String, Object?>? line}) {
    if (r.isEmpty) return;
    final path = _rectPath(r);
    if (fill != null) _painter.fill(canvas, path, r, fill);
    if (line != null) _painter.line(canvas, path, line);
  }

  Map<String, Object?> _own(SeriesSpec s, int point, Map<String, Object?> fallback) {
    final own = _pointShape(s, point)?['fill'] ?? s.shape?['fill'];
    return own is Map<String, Object?> ? own : fallback;
  }

  /// Draws a label centered on [at], above it for a [side] of -1, below
  /// for 1.
  void _label(Canvas canvas, String text, Props props, Offset at, {double side = 0, double? width}) {
    final t = _text(text, props, size: 9, width: width);
    paintText(canvas, t, Offset(at.dx - t.width / 2, at.dy - t.height * (1 - side) / 2));
  }

  String? _labelOf(SeriesSpec s, String category, double? value, String format) {
    final l = s.labels;
    if (l == null || !l.any) return null;
    final parts = [if (l.series) _name(s), if (l.category) category, if (l.value && value != null) _format(value, l.format.isNotEmpty ? l.format : format)];
    return parts.isEmpty ? null : parts.join(l.separator ?? ', ');
  }

  /// Draws the axes of a plot in [area]: values up the side, the texts of
  /// [slots] under, and gridlines; gives the area left for the plot.
  Rect _frame(Canvas canvas, PlotSpec p, Rect area, Scale scale, List<String> slots, {String format = '', Scale? second, String secondFormat = ''}) {
    final value = _axisOf(p, 'val'), category = _axisOf(p, 'cat');
    final fmt = value != null && value.format.isNotEmpty ? value.format : format;
    final ticks = [for (final t in scale.ticks) _text(_format(t, fmt), value?.text ?? const {}, size: 9)];
    final other = second == null ? <TextPainter>[] : [for (final t in second.ticks) _text(_format(t, secondFormat), _axisOf(p, 'val', 1)?.text ?? const {}, size: 9)];
    final left = value?.deleted == true ? 0.0 : ticks.fold(0.0, (w, t) => math.max(w, t.width)) + 6;
    final right = other.fold(0.0, (w, t) => math.max(w, t.width)) + (other.isEmpty ? 0 : 6);
    final slot = math.max(area.width - left - right, 1) / math.max(slots.length, 1);
    final names = [for (final s in slots) _text(s, category?.text ?? const {}, size: 9, width: slot)];
    final below = category?.deleted == true ? 0.0 : names.fold(0.0, (h, t) => math.max(h, t.height)) + 4;
    final plot = Rect.fromLTRB(area.left + left, area.top + 6, area.right - right, area.bottom - below);
    if (plot.isEmpty) return plot;
    double y(double v) => plot.bottom - scale.fraction(v) * plot.height;
    final grid = value == null || value.hasGrid;
    for (final (i, t) in scale.ticks.indexed) {
      if (grid) canvas.drawLine(Offset(plot.left, y(t)), Offset(plot.right, y(t)), Paint()..color = _gridColor);
      if (left > 0) paintText(canvas, ticks[i], Offset(plot.left - 6 - ticks[i].width, y(t) - ticks[i].height / 2));
    }
    if (second != null) {
      for (final (i, t) in second.ticks.indexed) {
        paintText(canvas, other[i], Offset(plot.right + 6, plot.bottom - second.fraction(t) * plot.height - other[i].height / 2));
      }
    }
    canvas.drawLine(Offset(plot.left, plot.bottom), Offset(plot.right, plot.bottom), Paint()..color = _axisColor);
    if (below > 0) {
      for (final (i, t) in names.indexed) {
        paintText(canvas, t, Offset(plot.left + slot * (i + 0.5) - t.width / 2, plot.bottom + 4));
      }
    }
    return plot;
  }

  Scale _scale(PlotSpec p, double lo, double hi, [int skip = 0]) {
    final a = _axisOf(p, 'val', skip);
    return Scale.auto(lo, hi, min: a?.min, max: a?.max, major: a?.major ?? 0);
  }

  /// The width of a bar in a slot, which the gap between bars takes a
  /// part of.
  double _barWidth(PlotSpec p, double slot, int fallback) => slot / (1 + (_int(p.json['gap']) ?? fallback) / 100);

  int? _int(Object? v) => v is num ? v.toInt() : null;

  void _waterfall(Canvas canvas, PlotSpec p, SeriesSpec s, Rect area) {
    final data = _val(s);
    final n = _count([s]);
    final steps = waterfallSteps([for (var i = 0; i < n; i++) _value(s, i)], s.options?.subtotals ?? const {});
    final top = steps.fold(0.0, (a, t) => math.max(a, t.high)), bottom = steps.fold(0.0, (a, t) => math.min(a, t.low));
    final scale = _scale(p, bottom, top);
    final plot = _frame(canvas, p, area, scale, [
      for (var i = 0; i < n; i++) _category([s], i),
    ], format: data?.format ?? '');
    if (plot.isEmpty) return;
    final slot = plot.width / n, width = _barWidth(p, slot, 50);
    double y(double v) => plot.bottom - scale.fraction(v) * plot.height;
    Rect? last;
    var previous = 0.0;
    for (final step in steps) {
      final x = plot.left + slot * (step.index + 0.5);
      final r = Rect.fromLTRB(x - width / 2, y(step.high), x + width / 2, y(step.low));
      if (last != null && (s.options?.connectors ?? true)) canvas.drawLine(Offset(last.right, y(previous)), Offset(r.left, y(step.from)), Paint()..color = _axisColor);
      _fillRect(canvas, r, _own(s, step.index, {'solid': _auto(step.kind.index, 3)}), line: _pointShape(s, step.index)?['line'] as Map<String, Object?>?);
      final text = _labelOf(s, '', _value(s, step.index), data?.format ?? '');
      if (text != null) _label(canvas, text, s.labels!.text, Offset(x, step.to >= step.from ? r.top - 2 : r.bottom + 2), side: step.to >= step.from ? -1 : 1);
      last = r;
      previous = step.to;
    }
  }

  void _histogram(Canvas canvas, PlotSpec p, SeriesSpec s, Rect area) {
    final data = _val(s);
    final n = _count([s]);
    final values = [for (var i = 0; i < n; i++) _value(s, i)];
    final columns = <(String, double)>[];
    if (s.options?.aggregate ?? false) {
      columns.addAll(
        categoryTotals(
          [
            for (var i = 0; i < n; i++) _category([s], i),
          ],
          values,
          sorted: p.kind == 'pareto',
        ),
      );
    } else {
      final bins = s.options?.bins;
      final right = bins?['closed'] != 'l';
      final list = histogramBins(
        [for (final v in values) ?v],
        size: (bins?['size'] as num?)?.toDouble(),
        count: _int(bins?['count']),
        closedRight: right,
        under: (bins?['under'] as num?)?.toDouble(),
        over: (bins?['over'] as num?)?.toDouble(),
      );
      String edge(double v) => _format(v, data?.format ?? '');
      for (final (i, b) in list.indexed) {
        final label = switch (b) {
          _ when b.low.isInfinite => '≤ ${edge(b.high)}',
          _ when b.high.isInfinite => '> ${edge(b.low)}',
          _ => right ? '${i == 0 ? '[' : ']'}${edge(b.low)} ; ${edge(b.high)}]' : '[${edge(b.low)} ; ${edge(b.high)}${i == list.length - 1 ? ']' : '['}',
        };
        columns.add((label, b.count.toDouble()));
      }
    }
    if (columns.isEmpty) return;
    final top = columns.fold(0.0, (a, c) => math.max(a, c.$2));
    final scale = _scale(p, 0, top);
    final line = p.series.any((o) => o.options?.line ?? false);
    final percent = line ? Scale.auto(0, 1, min: _axisOf(p, 'val', 1)?.min ?? 0, max: _axisOf(p, 'val', 1)?.max ?? 1) : null;
    final plot = _frame(canvas, p, area, scale, [for (final c in columns) c.$1], second: percent, secondFormat: '0%');
    if (plot.isEmpty) return;
    final slot = plot.width / columns.length, width = _barWidth(p, slot, 0);
    double y(double v) => plot.bottom - scale.fraction(v) * plot.height;
    for (final (i, c) in columns.indexed) {
      final x = plot.left + slot * (i + 0.5);
      _fillRect(canvas, Rect.fromLTRB(x - width / 2, y(c.$2), x + width / 2, y(0)), _own(s, i, _fill(s, -1)), line: _line(s, i));
      final text = _labelOf(s, c.$1, c.$2, '0');
      if (text != null) _label(canvas, text, s.labels!.text, Offset(x, y(c.$2) - 2), side: -1);
    }
    if (percent != null) {
      final path = Path();
      final shares = cumulated([for (final c in columns) c.$2]);
      final points = [for (final (i, v) in shares.indexed) Offset(plot.left + slot * (i + 0.5), plot.bottom - percent.fraction(v) * plot.height)];
      for (final (i, pt) in points.indexed) {
        i == 0 ? path.moveTo(pt.dx, pt.dy) : path.lineTo(pt.dx, pt.dy);
      }
      _painter.line(canvas, path, _paretoLine());
      for (final pt in points) {
        canvas.drawCircle(pt, 3, Paint()..color = _painter.color((_paretoLine()['fill'] as Map)['solid'] as Map<String, Object?>?) ?? _inkColor);
      }
    }
  }

  void _boxes(Canvas canvas, PlotSpec p, List<SeriesSpec> series, Rect area) {
    final first = series.first;
    final n = _count(series);
    final categories = [
      for (var i = 0; i < n; i++) _cat(first) == null ? '' : _category([first], i),
    ];
    final groups = categories.toSet().toList();
    final all = [
      for (final s in series)
        for (var i = 0; i < n; i++) ?_value(s, i),
    ];
    if (all.isEmpty) return;
    final scale = _scale(p, all.reduce(math.min), all.reduce(math.max));
    final plot = _frame(canvas, p, area, scale, [for (final g in groups) g], format: _val(first)?.format ?? '');
    if (plot.isEmpty) return;
    final slot = plot.width / groups.length, width = slot * 0.8 / series.length;
    double y(double v) => plot.bottom - scale.fraction(v) * plot.height;
    final ink = Paint()
      ..color = _inkColor
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;
    for (final (k, s) in series.indexed) {
      final look = s.options?.box;
      final means = <Offset>[];
      for (final (g, name) in groups.indexed) {
        final stats = boxStats([
          for (var i = 0; i < n; i++)
            if (categories[i] == name) ?_value(s, i),
        ], exclusive: look?['quartile'] != 'inclusive');
        if (stats == null) continue;
        final x = plot.left + slot * g + slot * 0.1 + width * (k + 0.5);
        final r = Rect.fromLTRB(x - width * 0.4, y(stats.q3), x + width * 0.4, y(stats.q1));
        canvas.drawLine(Offset(x, y(stats.high)), Offset(x, r.top), ink);
        canvas.drawLine(Offset(x, y(stats.low)), Offset(x, r.bottom), ink);
        canvas.drawLine(Offset(x - width * 0.2, y(stats.high)), Offset(x + width * 0.2, y(stats.high)), ink);
        canvas.drawLine(Offset(x - width * 0.2, y(stats.low)), Offset(x + width * 0.2, y(stats.low)), ink);
        _fillRect(canvas, r, _own(s, -1, _fill(s, -1)));
        canvas.drawRect(r, ink);
        canvas.drawLine(Offset(r.left, y(stats.median)), Offset(r.right, y(stats.median)), ink);
        final dot = Paint()..color = _inkColor;
        if (look?['inner'] == true) {
          for (final v in stats.inner) {
            canvas.drawCircle(Offset(x, y(v)), 2, dot);
          }
        }
        if (look?['outliers'] != false) {
          for (final v in stats.outliers) {
            canvas.drawCircle(Offset(x, y(v)), 2.5, ink);
          }
        }
        final mean = Offset(x, y(stats.mean));
        means.add(mean);
        if (look?['meanMarker'] != false) {
          canvas.drawLine(mean.translate(-3, -3), mean.translate(3, 3), ink);
          canvas.drawLine(mean.translate(-3, 3), mean.translate(3, -3), ink);
        }
      }
      if (look?['meanLine'] == true && means.length > 1) {
        final path = Path()..moveTo(means.first.dx, means.first.dy);
        for (final m in means.skip(1)) {
          path.lineTo(m.dx, m.dy);
        }
        canvas.drawPath(path, ink);
      }
    }
  }

  void _funnel(Canvas canvas, SeriesSpec s, Rect area) {
    final data = _val(s);
    final n = _count([s]);
    final names = [
      for (var i = 0; i < n; i++) _text(_category([s], i), const {}, size: 9),
    ];
    final left = names.fold(0.0, (w, t) => math.max(w, t.width)) + 8;
    final bars = funnelBars([for (var i = 0; i < n; i++) _value(s, i) ?? 0], Rect.fromLTRB(area.left + left, area.top + 4, area.right, area.bottom - 4));
    for (final (i, r) in bars.indexed) {
      paintText(canvas, names[i], Offset(area.left + left - 8 - names[i].width, r.center.dy - names[i].height / 2));
      _fillRect(canvas, r, _own(s, i, _fill(s, -1)), line: _line(s, i));
      final text = _labelOf(s, _category([s], i), _value(s, i), data?.format ?? '');
      if (text != null) _label(canvas, text, s.labels!.text.isEmpty ? _whiteText : s.labels!.text, r.center, width: r.width);
    }
  }

  Branch _tree(SeriesSpec s) => hierarchy(_cat(s), [
    for (var i = 0; i < _count([s]); i++) _value(s, i),
  ]);

  Map<String, Object?> _tint(int top, int tops, {bool darker = false}) {
    final color = _auto(top, tops);
    return {
      'solid': darker
          ? {
              ...color,
              'mods': [
                ...(color['mods'] as List? ?? const []),
                const ['lumMod', 75000],
              ],
            }
          : color,
    };
  }

  void _treemap(Canvas canvas, SeriesSpec s, Rect area) {
    final root = _tree(s);
    final banner = s.options?.parents == 'banner' && root.depth > 2;
    final tiles = treemapTiles(root, area.deflate(2), header: banner ? 14 : 0);
    final tops = root.children.length;
    final white = {
      'w': 12700,
      'fill': {
        'solid': {'rgb': 'FFFFFF'},
      },
    };
    for (final t in tiles) {
      final leaf = t.branch.children.isEmpty;
      if (t.rect.width <= 0 || t.rect.height <= 0) continue;
      _fillRect(canvas, t.rect, _tint(t.top, tops, darker: !leaf), line: white);
      final labelled = leaf ? (s.labels?.any ?? true) : s.options?.parents != 'none';
      if (!labelled || t.rect.width < 12) continue;
      canvas.save();
      canvas.clipRect(t.rect);
      paintText(canvas, _text(t.branch.label, _whiteText, size: 9, width: t.rect.width - 4, align: TextAlign.left), t.rect.topLeft.translate(3, 2));
      canvas.restore();
    }
  }

  void _sunburst(Canvas canvas, SeriesSpec s, Rect area) {
    final root = _tree(s);
    final rings = root.depth - 1;
    if (rings < 1) return;
    final center = area.center;
    final radius = math.min(area.width, area.height) / 2 - 2;
    if (radius <= 0) return;
    final width = radius / rings;
    final tops = root.children.length;
    final white = {
      'w': 12700,
      'fill': {
        'solid': {'rgb': 'FFFFFF'},
      },
    };
    for (final a in sunburstArcs(root)) {
      if (a.sweep <= 0) continue;
      final inner = width * a.ring, outer = inner + width;
      final path = Path()..arcTo(Rect.fromCircle(center: center, radius: outer), a.start, a.sweep, true);
      if (inner > 0) {
        path.arcTo(Rect.fromCircle(center: center, radius: inner), a.start + a.sweep, -a.sweep, false);
      } else {
        path.lineTo(center.dx, center.dy);
      }
      path.close();
      _painter.fill(canvas, path, path.getBounds(), _tint(a.top, tops, darker: false));
      _painter.line(canvas, path, white);
      final mid = a.start + a.sweep / 2, at = center + Offset.fromDirection(mid, (inner + outer) / 2);
      final text = _text(a.branch.label, _whiteText, size: 9);
      if (a.sweep * (inner + outer) / 2 > text.width && width > text.height) paintText(canvas, text, at - Offset(text.width / 2, text.height / 2));
    }
  }

  /// What stands for a chart the client cannot draw: the maps, which Excel
  /// draws from data of its own.
  void _substitute(Canvas canvas, SeriesSpec s, Rect area) {
    final name = _name(s);
    final text = _text('Carte non affichée${name.isEmpty ? '' : ' : $name'}', const {}, size: 11, width: area.width - 16);
    canvas.drawRect(area.deflate(4), Paint()..color = const Color(0xFFF2F2F2));
    paintText(canvas, text, area.center - Offset(text.width / 2, text.height / 2));
  }
}
