/// The conditional formats the ribbon offers, as the Go package takes
/// them in a sheet's "cf" (the "ref" and "pri" are set when they are
/// added).
library;

Map<String, Object?> _rgb(String hex) => {'rgb': hex};

Map<String, Object?> _fill(String hex) => {'pattern': 'solid', 'fg': _rgb(hex)};

Map<String, Object?> _edge(String hex) => {'style': 'thin', 'color': _rgb(hex)};

/// The formats a highlighted cell takes, by the id the menus use.
final highlightStyles = <String, Map<String, Object?>>{
  'red': {
    'font': {'color': _rgb('9C0006')},
    'fill': _fill('FFC7CE'),
  },
  'yellow': {
    'font': {'color': _rgb('9C5700')},
    'fill': _fill('FFEB9C'),
  },
  'green': {
    'font': {'color': _rgb('006100')},
    'fill': _fill('C6EFCE'),
  },
  'redText': {
    'font': {'color': _rgb('9C0006')},
  },
  'redBorder': {
    'border': {'l': _edge('FF0000'), 'r': _edge('FF0000'), 't': _edge('FF0000'), 'b': _edge('FF0000')},
  },
};

/// The colors of data bars.
const dataBarColors = {'blue': '638EC6', 'green': '63C384', 'red': 'FF555A', 'orange': 'FFB628', 'purple': 'D6007B'};

/// The colors of a scale, the lowest first.
const colorScales = {
  'gyr': ['63BE7B', 'FFEB84', 'F8696B'],
  'ryg': ['F8696B', 'FFEB84', '63BE7B'],
  'gwr': ['63BE7B', 'FCFCFF', 'F8696B'],
  'rwg': ['F8696B', 'FCFCFF', '63BE7B'],
  'bwr': ['5A8AC6', 'FCFCFF', 'F8696B'],
  'rwb': ['F8696B', 'FCFCFF', '5A8AC6'],
  'wg': ['FCFCFF', '63BE7B'],
  'wr': ['FCFCFF', 'F8696B'],
};

/// A rule that holds where a formula does, written for the first cell of
/// the cells it formats.
Map<String, Object?> formulaRule(String formula, String style) => {
  'type': 'formula',
  'f': [formula],
  'style': highlightStyles[style],
};

/// A rule on the value of cells: [kind] is one of the menu's
/// 'greaterThan', 'lessThan', 'between', 'equal', and [values] what it
/// compares to, as formulas.
Map<String, Object?> valueRule(String kind, String anchor, List<String> values, String style) => formulaRule(switch (kind) {
  'greaterThan' => '$anchor>(${values[0]})',
  'lessThan' => '$anchor<(${values[0]})',
  'equal' => '$anchor=(${values[0]})',
  _ => 'AND($anchor>=(${values[0]}),$anchor<=(${values[1]}))',
}, style);

/// A rule on the cells that contain a text.
Map<String, Object?> containsRule(String anchor, String text, String style) => formulaRule('NOT(ISERROR(SEARCH("${text.replaceAll('"', '""')}",$anchor)))', style);

Map<String, Object?> duplicateRule(String style, {bool unique = false}) => {'type': unique ? 'uniqueValues' : 'duplicateValues', 'style': highlightStyles[style]};

/// The first or last [rank] values, or percent of them.
Map<String, Object?> topRule(int rank, String style, {bool percent = false, bool bottom = false}) => {
  'type': 'top10',
  'rank': rank,
  if (percent) 'percent': true,
  if (bottom) 'bottom': true,
  'style': highlightStyles[style],
};

Map<String, Object?> averageRule(String style, {bool below = false}) => {'type': 'aboveAverage', if (below) 'below': true, 'style': highlightStyles[style]};

Map<String, Object?> dataBarRule(String color) => {
  'type': 'dataBar',
  'stops': [
    {'t': 'min'},
    {'t': 'max'},
  ],
  'colors': [_rgb(dataBarColors[color] ?? dataBarColors['blue']!)],
  'minLen': 10,
  'maxLen': 90,
};

Map<String, Object?> colorScaleRule(String set) {
  final colors = colorScales[set] ?? colorScales['gyr']!;
  return {
    'type': 'colorScale',
    'stops': [
      {'t': 'min'},
      if (colors.length == 3) {'t': 'percentile', 'v': '50'},
      {'t': 'max'},
    ],
    'colors': [for (final c in colors) _rgb(c)],
  };
}

/// A set of icons, one of Excel's such as '3Arrows' or '5Rating', whose
/// icons share the values equally.
Map<String, Object?> iconSetRule(String icons) {
  final n = int.parse(icons.substring(0, 1));
  return {
    'type': 'iconSet',
    'icons': icons,
    'stops': [
      for (var i = 0; i < n; i++) {'t': 'percent', 'v': '${(i * 100 / n).round()}'},
    ],
  };
}
