import '../text/text_frame.dart';

/// A chart as the Go package chart describes it.
class ChartSpec {
  ChartSpec._(this.json);

  static ChartSpec? fromJson(Object? json) => json is Map<String, Object?> ? ChartSpec._(json) : null;

  final Map<String, Object?> json;

  late final title = TitleSpec.fromJson(json['title']);
  late final legend = LegendSpec.fromJson(json['legend']);
  late final plots = [for (final p in _list<Map<String, Object?>>(json['plots'])) PlotSpec(p)];
  late final axes = [for (final a in _list<Map<String, Object?>>(json['axes'])) AxisSpec(a)];
  late final layout = LayoutSpec.fromJson(json['layout']);
  late final Map<String, Object?>? space = _map(json['space']);
  late final Map<String, Object?>? area = _map(json['area']);
  late final Props text = _props(json['text']);
  int get style => _int(json['style']) ?? 2;
  bool get rounded => json['rounded'] == true;
  String get blanks => json['blanks'] as String? ?? 'gap';

  /// Whether the chart shows a pivot table, whose values are not all
  /// those of the cells its series point to.
  bool get pivot => json['pivot'] == true;

  AxisSpec? axis(num id) {
    for (final a in axes) {
      if (a.id == id) return a;
    }
    return null;
  }

  /// Whether the chart is one of those Excel 2016 added, which are drawn
  /// but not edited.
  bool get extended => plots.any((p) => p.extended);

  /// The series of all plots in the order they are drawn and listed.
  List<SeriesSpec> get series => [for (final p in plots) ...p.series];
}

class PlotSpec {
  PlotSpec(this.json);

  final Map<String, Object?> json;

  String get kind => json['kind'] as String? ?? 'bar';
  bool get horizontal => json['dir'] == 'bar';
  String get grouping => json['grouping'] as String? ?? (kind == 'bar' ? 'clustered' : 'standard');
  bool get stacked => grouping == 'stacked' || grouping == 'percentStacked';
  bool get percent => grouping == 'percentStacked';
  bool get vary => json['vary'] == true;
  int get gap => _int(json['gap']) ?? 150;
  int get overlap => _int(json['overlap']) ?? (stacked ? 100 : 0);
  int get hole => _int(json['hole']) ?? 50;
  int get angle => _int(json['angle']) ?? 0;
  String get style => json['style'] as String? ?? '';
  bool get markers => json['markers'] == true;
  late final axes = [for (final a in _list<Object?>(json['axes'])) (a as num?) ?? 0];
  late final labels = LabelsSpec.fromJson(json['labels']);
  late final series = [for (final s in _list<Map<String, Object?>>(json['series'])) SeriesSpec(s, this)]..sort((a, b) => a.order.compareTo(b.order));

  /// Whether the plot is of a kind Excel 2016 added.
  bool get extended => extendedKinds.contains(kind);

  /// Whether the plot is drawn around a center rather than on axes.
  bool get round => kind == 'pie' || kind == 'doughnut';

  /// Whether points sit on x values rather than in category slots.
  bool get xy => kind == 'scatter' || kind == 'bubble';
}

/// The kinds of plot of the charts Excel 2016 added.
const extendedKinds = {'waterfall', 'histogram', 'pareto', 'boxWhisker', 'funnel', 'treemap', 'sunburst', 'regionMap'};

class SeriesSpec {
  SeriesSpec(this.json, this.plot);

  final Map<String, Object?> json;
  final PlotSpec plot;

  int get index => _int(json['index']) ?? 0;
  int get order => _int(json['order']) ?? 0;
  late final name = DataSpec.fromJson(json['name']);
  late final cat = DataSpec.fromJson(json['cat']);
  late final val = DataSpec.fromJson(json['val']);
  late final size = DataSpec.fromJson(json['size']);
  late final Map<String, Object?>? shape = _map(json['shape']);
  late final marker = MarkerSpec.fromJson(json['marker']);
  late final labels = LabelsSpec.fromJson(json['labels']);
  late final options = OptionsSpec.fromJson(json['options']);
  bool get smooth => json['smooth'] == true;
  int get explosion => _int(json['explosion']) ?? 0;
  bool get invert => json['invert'] == true;

  late final Map<int, Map<String, Object?>> points = {for (final p in _list<Map<String, Object?>>(json['points'])) _int(p['idx']) ?? -1: p};
}

/// What a series of a chart of Excel 2016 draws by.
class OptionsSpec {
  OptionsSpec(this.json);

  static OptionsSpec? fromJson(Object? json) => json is Map<String, Object?> ? OptionsSpec(json) : null;

  final Map<String, Object?> json;

  late final Set<int> subtotals = {
    for (final i in _list<Object?>(json['subtotals']))
      if (i is num) i.toInt(),
  };
  bool get connectors => json['noConnectors'] != true;
  bool get aggregate => json['aggregate'] == true;
  bool get line => json['line'] == true;
  late final Map<String, Object?>? bins = _map(json['bins']);
  late final Map<String, Object?>? box = _map(json['box']);
  String get parents => json['parents'] as String? ?? 'banner';
}

/// Values of a series: numbers, texts, or both, and where they come from.
class DataSpec {
  DataSpec({this.ref = '', this.format = '', this.numbers = const [], this.texts = const [], this.count = 0, this.levels = const []});

  static DataSpec? fromJson(Object? json) {
    if (json is! Map<String, Object?>) return null;
    final numbers = _list<Object?>(json['num']);
    final texts = _list<Object?>(json['str']);
    return DataSpec(
      ref: json['ref'] as String? ?? '',
      format: json['format'] as String? ?? '',
      numbers: [for (final v in numbers) v is num ? v.toDouble() : null],
      texts: [for (final v in texts) v is String ? v : ''],
      count: _int(json['count']) ?? 0,
      levels: [
        for (final l in _list<Object?>(json['levels']))
          if (l is List) [for (final t in l) t is String ? t : ''],
      ],
    );
  }

  final String ref;
  final String format;
  final List<double?> numbers;
  final List<String> texts;
  final int count;

  /// The texts of categories on several levels, the innermost first.
  final List<List<String>> levels;

  int get length => [count, numbers.length, texts.length].reduce((a, b) => a > b ? a : b);

  double? number(int i) => i < numbers.length ? numbers[i] : null;

  String text(int i) => i < texts.length ? texts[i] : '';

  bool get isText => texts.isNotEmpty && numbers.isEmpty;
}

class MarkerSpec {
  MarkerSpec(this.json);

  static MarkerSpec? fromJson(Object? json) => json is Map<String, Object?> ? MarkerSpec(json) : null;

  final Map<String, Object?> json;

  String get symbol => json['symbol'] as String? ?? '';
  int? get size => _int(json['size']);
  late final Map<String, Object?>? shape = _map(json['shape']);
}

class LabelsSpec {
  LabelsSpec(this.json);

  static LabelsSpec? fromJson(Object? json) => json is Map<String, Object?> ? LabelsSpec(json) : null;

  final Map<String, Object?> json;

  bool get delete => json['delete'] == true;
  bool get value => json['val'] == true;
  bool get percent => json['percent'] == true;
  bool get category => json['cat'] == true;
  bool get series => json['ser'] == true;
  bool get any => !delete && (value || percent || category || series);
  String get position => json['pos'] as String? ?? '';
  String get format => json['format'] as String? ?? '';
  String? get separator => json['sep'] as String?;
  late final Props text = _props(json['text']);
  late final Map<String, Object?>? shape = _map(json['shape']);

  /// The labels of a point: its own, or these.
  LabelsSpec at(int idx) {
    for (final p in _list<Map<String, Object?>>(json['points'])) {
      if (_int(p['idx']) == idx) return LabelsSpec({...json, ...p, 'points': null});
    }
    return this;
  }
}

class AxisSpec {
  AxisSpec(this.json);

  final Map<String, Object?> json;

  num get id => (json['id'] as num?) ?? 0;
  String get kind => json['kind'] as String? ?? 'val';
  String get position => json['pos'] as String? ?? 'b';
  bool get vertical => position == 'l' || position == 'r';
  bool get deleted => json['delete'] == true;
  num get cross => (json['cross'] as num?) ?? 0;
  String get crosses => json['crosses'] as String? ?? 'autoZero';
  double? get crossesAt => (json['crossesAt'] as num?)?.toDouble();
  bool get between => json['between'] != 'midCat';
  double? get min => (json['min'] as num?)?.toDouble();
  double? get max => (json['max'] as num?)?.toDouble();
  bool get reverse => json['reverse'] == true;
  double get log => (json['log'] as num?)?.toDouble() ?? 0;
  double get major => (json['major'] as num?)?.toDouble() ?? 0;
  late final Map<String, Object?>? grid = _map(json['grid']);
  late final Map<String, Object?>? minorGrid = _map(json['minorGrid']);
  bool get hasGrid => json['grid'] is Map;
  bool get hasMinorGrid => json['minorGrid'] is Map;
  String get format => json['format'] as String? ?? '';
  bool get linked => json['linked'] == true;
  String get labels => json['labels'] as String? ?? 'nextTo';
  String get tick => json['tick'] as String? ?? 'out';
  int get skip => _int(json['skip']) ?? 0;
  late final title = TitleSpec.fromJson(json['title']);
  late final Map<String, Object?>? shape = _map(json['shape']);
  late final Props text = _props(json['text']);

  /// The rotation of the labels in degrees, null when Office chooses.
  double? get rotation {
    final r = _int(json['rot']);
    return r == null || r == -60000000 ? null : r / 60000;
  }
}

class TitleSpec {
  TitleSpec(this.json);

  static TitleSpec? fromJson(Object? json) => json is Map<String, Object?> ? TitleSpec(json) : null;

  final Map<String, Object?> json;

  String get text => json['text'] as String? ?? '';
  late final Props props = _props(json['props']);
  bool get overlay => json['overlay'] == true;
  late final layout = LayoutSpec.fromJson(json['layout']);
  late final Map<String, Object?>? shape = _map(json['shape']);

  double? get rotation {
    final r = _int(json['rot']);
    return r == null ? null : r / 60000;
  }
}

class LegendSpec {
  LegendSpec(this.json);

  static LegendSpec? fromJson(Object? json) => json is Map<String, Object?> ? LegendSpec(json) : null;

  final Map<String, Object?> json;

  String get position => json['pos'] as String? ?? 'r';
  bool get overlay => json['overlay'] == true;
  late final hidden = {
    for (final i in _list<Object?>(json['hidden']))
      if (i is num) i.toInt(),
  };
  late final Props text = _props(json['text']);
  late final Map<String, Object?>? shape = _map(json['shape']);
  late final layout = LayoutSpec.fromJson(json['layout']);
}

/// A box placed by hand, in fractions of the chart.
class LayoutSpec {
  LayoutSpec(this.json);

  static LayoutSpec? fromJson(Object? json) => json is Map<String, Object?> ? LayoutSpec(json) : null;

  final Map<String, Object?> json;

  bool get inner => json['inner'] == true;
  bool get edge => json['xMode'] == 'edge';
  bool get edgeY => json['yMode'] == 'edge';
  double get x => (json['x'] as num?)?.toDouble() ?? 0;
  double get y => (json['y'] as num?)?.toDouble() ?? 0;
  double get w => (json['w'] as num?)?.toDouble() ?? 0;
  double get h => (json['h'] as num?)?.toDouble() ?? 0;
}

List<T> _list<T>(Object? v) => v is List
    ? [
        for (final x in v)
          if (x is T) x,
      ]
    : const [];

Map<String, Object?>? _map(Object? v) => v is Map<String, Object?> ? v : null;

int? _int(Object? v) => v is num ? v.toInt() : null;

Props _props(Object? v) => v is Map
    ? {
        for (final e in v.entries)
          if (e.value is String) '${e.key}': e.value as String,
      }
    : const {};
