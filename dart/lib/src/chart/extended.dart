import 'dart:math' as math;
import 'dart:ui';

import 'chart.dart';

/// Where the bars of a waterfall float, and the layouts of the other
/// charts Excel 2016 added, in the units of the area they are given.

enum StepKind { increase, decrease, total }

/// A bar of a waterfall, from where the running total stood to where it
/// lands.
class Step {
  const Step(this.index, this.from, this.to, this.kind);

  final int index;
  final double from;
  final double to;
  final StepKind kind;

  double get low => math.min(from, to);
  double get high => math.max(from, to);
}

/// The bars of a waterfall: each point moves the running total, a total
/// shows its own value from zero and starts the running again from there.
List<Step> waterfallSteps(List<double?> values, Set<int> totals) {
  final steps = <Step>[];
  var sum = 0.0;
  for (final (i, v) in values.indexed) {
    if (v == null) continue;
    if (totals.contains(i)) {
      steps.add(Step(i, 0, v, StepKind.total));
      sum = v;
    } else {
      steps.add(Step(i, sum, sum + v, v < 0 ? StepKind.decrease : StepKind.increase));
      sum += v;
    }
  }
  return steps;
}

/// An interval of a histogram and the values it holds; unbounded where it
/// takes what is beyond the bounds the chart fixes.
class Bin {
  Bin(this.low, this.high);

  final double low;
  final double high;
  int count = 0;
}

/// The intervals of a histogram and their counts. The width comes from the
/// size or the count the chart fixes, or from Scott's rule when it fixes
/// none; [closedRight] tells which end of an interval holds its value.
List<Bin> histogramBins(List<double> values, {double? size, int? count, bool closedRight = true, double? under, double? over}) {
  if (values.isEmpty) return [];
  final lo = values.reduce(math.min), hi = values.reduce(math.max);
  final start = under ?? lo, end = over ?? hi;
  var width = size ?? 0;
  if (width <= 0) {
    if (count != null && count > 0) {
      width = (end - start) / count;
    } else {
      final mean = values.reduce((a, b) => a + b) / values.length;
      final sigma = math.sqrt(values.fold(0.0, (a, v) => a + (v - mean) * (v - mean)) / values.length);
      width = 3.49 * sigma / math.pow(values.length, 1 / 3);
    }
  }
  if (width <= 0) width = 1;
  final n = count != null && size == null ? math.max(count, 1) : math.max(((end - start) / width - 1e-9).ceil(), 1);
  final bins = [
    if (under != null) Bin(double.negativeInfinity, start),
    for (var i = 0; i < n; i++) Bin(start + i * width, start + (i + 1) * width),
    if (over != null) Bin(start + n * width, double.infinity),
  ];
  final first = under != null ? 1 : 0;
  for (final v in values) {
    if (under != null && v <= start) {
      bins.first.count++;
    } else if (over != null && v > end) {
      bins.last.count++;
    } else {
      final q = ((v - start) / width * 1e9).round() / 1e9;
      final i = closedRight ? q.ceil() - 1 : q.floor();
      bins[first + i.clamp(0, n - 1)].count++;
    }
  }
  return bins;
}

/// The totals of a histogram by category, the values of a category added,
/// or its occurrences counted when there are no values; most first when
/// [sorted], as a pareto has them.
List<(String, double)> categoryTotals(List<String> categories, List<double?> values, {bool sorted = false}) {
  final totals = <String, double>{};
  final hasValues = values.any((v) => v != null);
  for (final (i, c) in categories.indexed) {
    totals[c] = (totals[c] ?? 0) + (hasValues ? values.elementAtOrNull(i) ?? 0 : 1);
  }
  final out = [for (final e in totals.entries) (e.key, e.value)];
  if (sorted) out.sort((a, b) => b.$2.compareTo(a.$2));
  return out;
}

/// The share of the sum each prefix of [values] makes: the line of a pareto.
List<double> cumulated(List<double> values) {
  final total = values.fold(0.0, (a, b) => a + b);
  var sum = 0.0;
  return [for (final v in values) total == 0 ? 0 : (sum += v) / total];
}

/// What a box and whisker draws of a set of values.
class BoxStats {
  BoxStats(this.q1, this.median, this.q3, this.mean, this.low, this.high, this.inner, this.outliers);

  final double q1;
  final double median;
  final double q3;
  final double mean;

  /// The ends of the whiskers: the extreme values within 1.5 boxes of it.
  final double low;
  final double high;

  /// The values within the whiskers, and those beyond them.
  final List<double> inner;
  final List<double> outliers;
}

/// The quartiles of [values] as Excel finds them, with the median left out
/// of the halves when [exclusive] is set.
BoxStats? boxStats(List<double> values, {bool exclusive = true}) {
  if (values.isEmpty) return null;
  final sorted = [...values]..sort();
  final n = sorted.length;
  double at(double p) {
    final pos = exclusive ? p * (n + 1) : 1 + p * (n - 1);
    final x = pos.clamp(1, n).toDouble();
    final i = x.floor();
    return i >= n ? sorted[n - 1] : sorted[i - 1] + (x - i) * (sorted[i] - sorted[i - 1]);
  }

  final q1 = at(0.25), q3 = at(0.75);
  final reach = (q3 - q1) * 1.5;
  final within = [
    for (final v in sorted)
      if (v >= q1 - reach && v <= q3 + reach) v,
  ];
  return BoxStats(q1, at(0.5), q3, sorted.reduce((a, b) => a + b) / n, within.first, within.last, within, [
    for (final v in sorted)
      if (v < q1 - reach || v > q3 + reach) v,
  ]);
}

/// The bars of a funnel, one under the other and centered, as wide as
/// their value is large.
List<Rect> funnelBars(List<double> values, Rect area, {double gap = 0.1}) {
  final top = values.fold(0.0, (a, v) => math.max(a, v));
  final slot = values.isEmpty ? 0.0 : area.height / values.length;
  return [
    for (final (i, v) in values.indexed)
      Rect.fromCenter(center: Offset(area.center.dx, area.top + slot * (i + 0.5)), width: top <= 0 ? 0 : area.width * math.max(v, 0) / top, height: slot * (1 - gap)),
  ];
}

/// A category of a hierarchy, and what it holds.
class Branch {
  Branch(this.label, [this.value = 0]);

  final String label;
  double value;
  final List<Branch> children = [];

  /// The sum of what its leaves hold.
  double get total => children.isEmpty ? value : children.fold(0.0, (a, b) => a + b.total);

  int get depth => children.isEmpty ? 1 : 1 + children.fold(0, (a, b) => math.max(a, b.depth));
}

/// The hierarchy a series draws, from the texts of its categories on
/// several levels, the innermost first, and the size of each point.
Branch hierarchy(DataSpec? categories, List<double?> sizes) {
  final root = Branch('');
  final levels = categories == null
      ? <List<String>>[]
      : categories.levels.isNotEmpty
      ? categories.levels
      : [categories.texts];
  for (var i = 0; i < sizes.length; i++) {
    var at = root;
    for (final level in levels.reversed) {
      final label = level.elementAtOrNull(i) ?? '';
      if (label.isEmpty) continue;
      at = at.children.firstWhere(
        (b) => b.label == label,
        orElse: () {
          final b = Branch(label);
          at.children.add(b);
          return b;
        },
      );
    }
    if (at != root) at.value += math.max(sizes[i] ?? 0, 0);
  }
  return root;
}

/// A category of a hierarchy placed in a treemap.
class Tile {
  const Tile(this.branch, this.depth, this.top, this.rect);

  final Branch branch;
  final int depth;

  /// Which of the categories of the first level it belongs to.
  final int top;
  final Rect rect;
}

/// The rectangles of a treemap: the children of a category share what its
/// own rectangle leaves them under a [header] and within [pad], the largest
/// first, in shapes as square as can be.
List<Tile> treemapTiles(Branch root, Rect area, {double header = 0, double pad = 1}) {
  final out = <Tile>[];
  void place(Branch b, Rect rect, int depth, int top) {
    final inner = b.children.isEmpty ? rect : Rect.fromLTRB(rect.left + pad, rect.top + pad + header, rect.right - pad, rect.bottom - pad);
    final kids = [...b.children]..sort((x, y) => y.total.compareTo(x.total));
    final rects = squarify([for (final k in kids) k.total], inner.width <= 0 || inner.height <= 0 ? Rect.zero : inner);
    for (final (i, k) in kids.indexed) {
      final t = depth == 0 ? b.children.indexOf(k) : top;
      out.add(Tile(k, depth, t, rects[i]));
      place(k, rects[i], depth + 1, t);
    }
  }

  place(root, area, 0, 0);
  return out;
}

/// Lays [weights], largest first, in [area], row by row along its short
/// side, a row ending where adding another would make its shapes less
/// square.
List<Rect> squarify(List<double> weights, Rect area) {
  final total = weights.fold(0.0, (a, b) => a + b);
  final out = List.filled(weights.length, Rect.fromLTWH(area.left, area.top, 0, 0));
  if (total <= 0 || area.isEmpty) return out;
  final areas = [for (final w in weights) w * area.width * area.height / total];
  double worst(int from, int to, double sum, double side) {
    final lo = areas.sublist(from, to).reduce(math.min), hi = areas.sublist(from, to).reduce(math.max);
    return lo <= 0 ? double.infinity : math.max(side * side * hi / (sum * sum), sum * sum / (side * side * lo));
  }

  var rest = area;
  var start = 0;
  while (start < areas.length) {
    final wide = rest.width >= rest.height;
    final side = wide ? rest.height : rest.width;
    var end = start + 1;
    var sum = areas[start];
    var score = worst(start, end, sum, side);
    while (end < areas.length) {
      final next = worst(start, end + 1, sum + areas[end], side);
      if (next > score) break;
      sum += areas[end];
      score = next;
      end++;
    }
    final thickness = side <= 0 ? 0.0 : sum / side;
    var offset = 0.0;
    for (var i = start; i < end; i++) {
      final length = thickness <= 0 ? 0.0 : areas[i] / thickness;
      out[i] = wide ? Rect.fromLTWH(rest.left, rest.top + offset, thickness, length) : Rect.fromLTWH(rest.left + offset, rest.top, length, thickness);
      offset += length;
    }
    rest = wide ? Rect.fromLTRB(rest.left + thickness, rest.top, rest.right, rest.bottom) : Rect.fromLTRB(rest.left, rest.top + thickness, rest.right, rest.bottom);
    start = end;
  }
  return out;
}

/// A category of a hierarchy placed in a sunburst: the ring it sits on and
/// the angles it covers, from [start] clockwise.
class Arc {
  const Arc(this.branch, this.ring, this.start, this.sweep, this.top);

  final Branch branch;
  final int ring;
  final double start;
  final double sweep;
  final int top;
}

/// The arcs of a sunburst, a ring a level, the first around the center: the
/// children of a category share the angles it covers.
List<Arc> sunburstArcs(Branch root, {double start = -math.pi / 2}) {
  final out = <Arc>[];
  void place(Branch b, int ring, double from, double sweep, int top) {
    final total = b.total;
    var at = from;
    for (final (i, k) in b.children.indexed) {
      final share = total <= 0 ? 0.0 : sweep * k.total / total;
      final t = ring == 0 ? i : top;
      out.add(Arc(k, ring, at, share, t));
      place(k, ring + 1, at, share, t);
      at += share;
    }
  }

  place(root, 0, start, 2 * math.pi, 0);
  return out;
}
