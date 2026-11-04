import 'package:trame/trame.dart';

import 'formula_text.dart';
import 'input.dart';
import 'number_format.dart';
import 'workbook.dart';

/// A validation of cells, as the Go package reads those of a sheet into
/// its "valid": what the cells take, and what is said when they do not.
class Validation {
  Validation(this.json) : areas = [for (final a in (json['ref'] as String? ?? '').split(' ')) ?parseArea(a)];

  static List<Validation> of(Node sheet) {
    final all = sheet.attributes['valid'];
    if (all is! List<Object?>) return const [];
    return [
      for (final v in all)
        if (v is Map<String, Object?> && v['ref'] is String) Validation(v),
    ];
  }

  /// The operators of the types that compare, in the order the dialog
  /// lists them.
  static const operators = ['between', 'notBetween', 'equal', 'notEqual', 'greaterThan', 'lessThan', 'greaterThanOrEqual', 'lessThanOrEqual'];

  final Map<String, Object?> json;
  final List<CellArea> areas;

  /// What the cells take: 'list', 'whole', 'decimal', 'date', 'time',
  /// 'textLength' or 'custom'; anything when null.
  String? get type => json['type'] as String?;

  String get op => json['op'] as String? ?? 'between';

  /// The first bound, or the list as a formula: values in quotes
  /// separated by commas, the cells or the name that holds them.
  String get first => json['f1'] as String? ?? '';

  String get second => json['f2'] as String? ?? '';

  bool get arrow => json['noArrow'] != true;

  bool get blank => json['blank'] == true;

  /// How a value that is not taken is met: 'stop', 'warning' or
  /// 'information'; any value is taken when null.
  String? get error => json['err'] as String?;

  String get errorTitle => json['errTitle'] as String? ?? '';

  String get errorText => json['errText'] as String? ?? '';

  String get promptTitle => json['promptTitle'] as String? ?? '';

  String get prompt => json['prompt'] as String? ?? '';

  bool contains(int row, int col) => areas.any((a) => a.contains(row, col));

  /// The values of the list as the cells holding them show them, null
  /// when the validation is not a list or the list is a formula
  /// calculated by the server alone.
  List<String>? items(Workbook book, Node sheet, NumberLocale locale) {
    if (type != 'list') return null;
    final s = first.trim();
    if (s.length >= 2 && s.startsWith('"') && s.endsWith('"')) {
      return s.substring(1, s.length - 1).replaceAll('""', '"').split(',');
    }
    final found = book.resolve(sheet, s);
    if (found == null) return null;
    final (on, area) = found;
    final out = <String>[];
    for (final c in (on.grid ?? const Grid.empty()).rows(area.top, area.bottom)) {
      if (c.col < area.left || c.col > area.right || c.fields['v'] == null && c.fields['e'] == null) continue;
      final text = cellText(c.fields, book.style(c.fields['s'] as String?), locale, date1904: book.date1904).text;
      if (text.isNotEmpty) out.add(text);
    }
    return out;
  }

  /// Whether the cells take what was typed: nothing when blanks are
  /// allowed, else a value as the type asks. What the client cannot
  /// calculate, a bound that is a formula or a custom rule, is taken.
  bool takes(String typed, Workbook book, Node sheet, NumberLocale locale) {
    if (typed.trim().isEmpty) return blank;
    switch (type) {
      case 'list':
        final items = this.items(book, sheet, locale);
        final t = typed.trim().toLowerCase();
        return items == null || items.any((i) => i.trim().toLowerCase() == t);
      case 'textLength':
        return _holds(typed.runes.length.toDouble(), book, sheet);
      case 'whole' || 'decimal' || 'date' || 'time':
        final v = parseInput(typed, locale: locale, date1904: book.date1904).fields['v'];
        if (v is! num || type == 'whole' && v != v.roundToDouble()) return false;
        return _holds(v.toDouble(), book, sheet);
    }
    return true;
  }

  bool _holds(double x, Workbook book, Node sheet) {
    final a = _bound(first, book, sheet);
    if (a == null) return true;
    if (op == 'between' || op == 'notBetween') {
      final b = _bound(second, book, sheet);
      if (b == null) return true;
      final inside = x >= a && x <= b || x >= b && x <= a;
      return inside == (op == 'between');
    }
    return switch (op) {
      'equal' => x == a,
      'notEqual' => x != a,
      'greaterThan' => x > a,
      'lessThan' => x < a,
      'greaterThanOrEqual' => x >= a,
      'lessThanOrEqual' => x <= a,
      _ => true,
    };
  }

  /// A bound as a number: written out, a date as DATE(year,month,day) or
  /// the value of a cell; null when only the server can tell.
  static double? _bound(String f, Workbook book, Node sheet) {
    final s = f.trim();
    final n = double.tryParse(s);
    if (n != null) return n.isFinite ? n : null;
    final date = RegExp(r'^DATE\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)$', caseSensitive: false).firstMatch(s);
    if (date != null) return serialOf(int.parse(date[1]!), int.parse(date[2]!), int.parse(date[3]!), date1904: book.date1904);
    final found = book.resolve(sheet, s);
    if (found == null || !found.$2.single) return null;
    final v = found.$1.grid?.cell(found.$2.top, found.$2.left)?['v'];
    return v is num ? v.toDouble() : null;
  }
}

/// A bound as a person reads it: a number, a date or a time written out,
/// or a formula in French.
String boundText(String f, {String? type, NumberLocale locale = NumberLocale.fr, bool date1904 = false}) {
  final n = double.tryParse(f.trim());
  if (n == null) return formulaToFrench(f.trim());
  final format = switch (type) {
    'date' => NumberFormat('dd/mm/yyyy'),
    'time' => NumberFormat('hh:mm:ss'),
    _ => null,
  };
  return editText({'v': n}, locale: locale, format: format, date1904: date1904);
}

/// A bound as the Go package takes it, from what was typed: a number, a
/// date or a time as its number, else a formula in French, "=" allowed.
String boundFormula(String text, {NumberLocale locale = NumberLocale.fr, bool date1904 = false}) {
  final t = text.trim();
  if (t.startsWith('=')) return formulaFromFrench(t.substring(1));
  final v = parseInput(t, locale: locale, date1904: date1904).fields['v'];
  return v is num ? '${v == v.roundToDouble() ? v.round() : v}' : formulaFromFrench(t);
}
