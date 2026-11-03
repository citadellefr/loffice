import 'dart:math' as math;

import 'package:flutter/foundation.dart';

/// How a workbook's user writes numbers and dates.
@immutable
class NumberLocale {
  const NumberLocale({
    required this.decimal,
    required this.group,
    required this.shortMonths,
    required this.months,
    required this.days,
    required this.shortDays,
    required this.trueText,
    required this.falseText,
  });

  final String decimal;
  final String group;
  final List<String> shortMonths;
  final List<String> months;

  /// The days of the week, Sunday first.
  final List<String> days;
  final List<String> shortDays;
  final String trueText;
  final String falseText;

  bool get french => decimal == ',';

  static const fr = NumberLocale(
    decimal: ',',
    group: '\u202f',
    shortMonths: ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.'],
    months: ['janvier', 'février', 'mars', 'avril', 'mai', 'juin', 'juillet', 'août', 'septembre', 'octobre', 'novembre', 'décembre'],
    days: ['dimanche', 'lundi', 'mardi', 'mercredi', 'jeudi', 'vendredi', 'samedi'],
    shortDays: ['dim.', 'lun.', 'mar.', 'mer.', 'jeu.', 'ven.', 'sam.'],
    trueText: 'VRAI',
    falseText: 'FAUX',
  );

  static const en = NumberLocale(
    decimal: '.',
    group: ',',
    shortMonths: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
    months: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
    days: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
    shortDays: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
    trueText: 'TRUE',
    falseText: 'FALSE',
  );
}

/// A number written by a format, in the color its section asks for.
@immutable
class Formatted {
  const Formatted(this.text, [this.color]);

  final String text;

  /// A color name of the format: "red", "blue", "color10"…
  final String? color;
}

enum _Kind { literal, digit, decimal, exp, slash, date, text, general }

class _Token {
  _Token(this.kind, [this.text = '', this.part = '']);

  final _Kind kind;
  String text;

  /// Which part a digit writes: i integer, f decimals, e exponent, n
  /// numerator, d denominator; D marks a fixed denominator.
  String part;
}

class _Section {
  final tokens = <_Token>[];
  String? color;
  String cond = '';
  double condValue = 0;
  var date = false;
  var text = false;
  var general = false;
  var percent = 0;
  var scale = 0;
  var thousands = false;
  var exp = false;
  var fraction = false;
  var intDigits = 0;
  var fracDigits = 0;
  var denominator = 0;
  var secDigits = 0;
  var ampm = false;

  bool get lastDigit => tokens.isNotEmpty && tokens.last.kind == _Kind.digit;

  bool get hasDigit => tokens.any((t) => t.kind == _Kind.digit);

  bool get lastSeconds {
    for (var i = tokens.length - 1; i >= 0; i--) {
      final t = tokens[i];
      if (t.kind == _Kind.date) return t.text.startsWith('s') || t.text == '[s]' || t.text == '[ss]';
      if (t.kind != _Kind.literal) return false;
    }
    return false;
  }

  String placeholders(String part) => [
    for (final t in tokens)
      if (t.kind == _Kind.digit && t.part == part) t.text,
  ].join();
}

const _colors = ['black', 'blue', 'cyan', 'green', 'magenta', 'red', 'white', 'yellow'];

/// A number format code read: "0.00", "#,##0 €;[Red]-#,##0 €",
/// "dd/mm/yyyy". Its separators are those of the file: "." for decimals,
/// "," for thousands.
class NumberFormat {
  NumberFormat._(this._sections);

  factory NumberFormat(String code) {
    final cached = _cache[code];
    if (cached != null) return cached;
    final sections = [for (final s in _split(code).take(4)) _parse(s)];
    final f = NumberFormat._(
      sections.isEmpty
          ? [
              _Section()
                ..general = true
                ..tokens.add(_Token(_Kind.general)),
            ]
          : sections,
    );
    if (_cache.length > 512) _cache.clear();
    return _cache[code] = f;
  }

  static final _cache = <String, NumberFormat>{};

  final List<_Section> _sections;

  /// Whether the format writes dates or times.
  bool get isDate => _sections.first.date;

  /// Whether the format writes numbers as percentages.
  bool get isPercent => _sections.first.percent > 0;

  static List<String> _split(String code) {
    final out = <String>[];
    var start = 0;
    for (var i = 0; i < code.length; i++) {
      switch (code[i]) {
        case '"':
          final j = code.indexOf('"', i + 1);
          i = j < 0 ? code.length : j;
        case r'\' || '_' || '*':
          i++;
        case '[':
          final j = code.indexOf(']', i);
          if (j >= 0) i = j;
        case ';':
          out.add(code.substring(start, i));
          start = i + 1;
      }
    }
    return out..add(code.substring(math.min(start, code.length)));
  }

  static _Section _parse(String code) {
    final s = _Section();
    void lit(String t) => s.tokens.add(_Token(_Kind.literal, t));
    final lower = code.toLowerCase();
    var i = 0;
    while (i < code.length) {
      final c = code[i];
      final lc = c.toLowerCase();
      if (c == '"') {
        final j = code.indexOf('"', i + 1);
        if (j < 0) {
          lit(code.substring(i + 1));
          break;
        }
        lit(code.substring(i + 1, j));
        i = j + 1;
      } else if (c == r'\' && i + 1 < code.length) {
        lit(code[i + 1]);
        i += 2;
      } else if (c == '_' && i + 1 < code.length) {
        lit(' ');
        i += 2;
      } else if (c == '*' && i + 1 < code.length) {
        i += 2;
      } else if (c == '[') {
        final j = code.indexOf(']', i);
        if (j < 0) break;
        _bracket(s, code.substring(i + 1, j), lit);
        i = j + 1;
      } else if (lower.startsWith('general', i)) {
        s.general = true;
        s.tokens.add(_Token(_Kind.general));
        i += 7;
      } else if (c == '0' || c == '#' || c == '?') {
        s.tokens.add(_Token(_Kind.digit, c));
        i++;
      } else if (c == '.') {
        if (s.lastSeconds) {
          var n = 0;
          while (i + 1 + n < code.length && code[i + 1 + n] == '0') {
            n++;
          }
          if (n > 0) {
            s.secDigits = n;
            s.tokens.add(_Token(_Kind.date, '.${'0' * n}'));
            i += 1 + n;
            continue;
          }
        }
        s.tokens.add(_Token(_Kind.decimal));
        i++;
      } else if (c == ',') {
        final next = i + 1 < code.length ? code[i + 1] : '';
        if (s.lastDigit && (next == '0' || next == '#' || next == '?')) {
          s.thousands = true;
        } else if (s.lastDigit || s.scale > 0 && i > 0 && code[i - 1] == ',') {
          s.scale++;
        } else {
          lit(',');
        }
        i++;
      } else if (c == '%') {
        s.percent++;
        lit('%');
        i++;
      } else if ((c == 'E' || c == 'e') && i + 1 < code.length && (code[i + 1] == '+' || code[i + 1] == '-') && s.hasDigit) {
        s.exp = true;
        s.tokens.add(_Token(_Kind.exp, code.substring(i, i + 2)));
        i += 2;
      } else if (c == '/' && s.lastDigit) {
        s.fraction = true;
        s.tokens.add(_Token(_Kind.slash));
        i++;
        var j = i;
        while (j < code.length && (code.codeUnitAt(j) >= 0x31 && code.codeUnitAt(j) <= 0x39 || j > i && code[j] == '0')) {
          j++;
        }
        if (j > i) {
          s.denominator = int.parse(code.substring(i, j));
          s.tokens.add(_Token(_Kind.literal, code.substring(i, j), 'D'));
          i = j;
        }
      } else if (c == '@') {
        s.text = true;
        s.tokens.add(_Token(_Kind.text));
        i++;
      } else if (lower.startsWith('am/pm', i)) {
        s.ampm = s.date = true;
        s.tokens.add(_Token(_Kind.date, 'AM/PM'));
        i += 5;
      } else if (lower.startsWith('a/p', i)) {
        s.ampm = s.date = true;
        s.tokens.add(_Token(_Kind.date, 'A/P'));
        i += 3;
      } else if ('ymdhse'.contains(lc) && !(lc == 'e' && !s.date)) {
        var j = i;
        while (j < code.length && code[j].toLowerCase() == lc) {
          j++;
        }
        s.date = true;
        s.tokens.add(_Token(_Kind.date, code.substring(i, j).toLowerCase()));
        i = j;
      } else {
        final pair = i + 1 < code.length && code.codeUnitAt(i) & 0xFC00 == 0xD800;
        lit(code.substring(i, i + (pair ? 2 : 1)));
        i += pair ? 2 : 1;
      }
    }
    _parts(s);
    return s;
  }

  static void _bracket(_Section s, String b, void Function(String) lit) {
    final lower = b.toLowerCase();
    if (b.startsWith(r'$')) {
      final sym = b.substring(1).split('-').first;
      if (sym.isNotEmpty) lit(sym);
    } else if (const ['h', 'hh', 'm', 'mm', 's', 'ss'].contains(lower)) {
      s.date = true;
      s.tokens.add(_Token(_Kind.date, '[$lower]'));
    } else if (b.isNotEmpty && '<>='.contains(b[0])) {
      var op = b[0];
      if (b.length > 1 && '<>='.contains(b[1])) op = b.substring(0, 2);
      final v = double.tryParse(b.substring(op.length).trim());
      if (v != null) {
        s.cond = op;
        s.condValue = v;
      }
    } else if (lower.startsWith('color') || _colors.contains(lower)) {
      s.color = lower;
    }
  }

  static void _parts(_Section s) {
    var part = 'i';
    for (final t in s.tokens) {
      switch (t.kind) {
        case _Kind.decimal:
          if (part == 'i') part = 'f';
        case _Kind.exp:
          part = 'e';
        case _Kind.slash:
          part = 'd';
        case _Kind.digit:
          t.part = part;
        default:
      }
    }
    if (s.fraction) {
      final slash = s.tokens.indexWhere((t) => t.kind == _Kind.slash);
      for (var i = slash - 1; i >= 0 && s.tokens[i].kind == _Kind.digit; i--) {
        s.tokens[i].part = 'n';
      }
    }
    for (final t in s.tokens) {
      if (t.kind == _Kind.digit && t.part == 'i') s.intDigits++;
      if (t.kind == _Kind.digit && t.part == 'f') s.fracDigits++;
    }
    for (var i = 0; i < s.tokens.length; i++) {
      final t = s.tokens[i];
      if (t.kind != _Kind.date || !t.text.startsWith('m') || t.text.length > 2) continue;
      final prev = _near(s, i, -1), next = _near(s, i, 1);
      if (prev.startsWith('h') || prev.startsWith('[h') || next.startsWith('s') || next.startsWith('[s')) {
        t.text = t.text.toUpperCase();
      }
    }
  }

  static String _near(_Section s, int i, int step) {
    for (var j = i + step; j >= 0 && j < s.tokens.length; j += step) {
      final t = s.tokens[j];
      if (t.kind == _Kind.date && !t.text.startsWith('.')) return t.text;
    }
    return '';
  }

  (_Section, bool) _section(double n) {
    final s = _sections;
    if (s[0].cond.isNotEmpty || s.length > 1 && s[1].cond.isNotEmpty) {
      for (final sec in s.take(2)) {
        if (sec.cond.isNotEmpty && _cmp(sec.cond, n.compareTo(sec.condValue))) return (sec, false);
      }
      for (final sec in s.take(3)) {
        if (sec.cond.isEmpty && !sec.text) return (sec, false);
      }
      return (s[0], false);
    }
    if (n < 0 && s.length > 1 && !s[1].text) return (s[1], true);
    if (n == 0 && s.length > 2 && !s[2].text) return (s[2], false);
    return (s[0], false);
  }

  static bool _cmp(String op, int r) => switch (op) {
    '=' => r == 0,
    '<>' => r != 0,
    '<' => r < 0,
    '>' => r > 0,
    '<=' => r <= 0,
    _ => r >= 0,
  };

  /// Writes a number.
  Formatted format(double n, NumberLocale l, {bool date1904 = false}) {
    final (s, unsigned) = _section(n);
    if (unsigned) n = n.abs();
    if (s.general || s.text && !s.hasDigit) {
      final out = generalText(n, l);
      return Formatted(
        [
          for (final t in s.tokens)
            if (t.kind == _Kind.general || t.kind == _Kind.text) out else if (t.kind == _Kind.literal) t.text,
        ].join(),
        s.color,
      );
    }
    if (s.date) {
      if (n < 0) return Formatted('#' * 11, s.color);
      return Formatted(_date(n, s, l, date1904), s.color);
    }
    return Formatted(_number(n, s, l, _sections.length == 1 || identical(s, _sections.first) && n >= 0), s.color);
  }

  /// Writes text: in the fourth section, or the first when it holds @.
  Formatted formatText(String text) {
    final _Section s;
    if (_sections.length == 4) {
      s = _sections[3];
    } else if (_sections.first.text) {
      s = _sections.first;
    } else {
      return Formatted(text);
    }
    return Formatted(
      [
        for (final t in s.tokens)
          if (t.kind == _Kind.text) text else if (t.kind == _Kind.literal) t.text,
      ].join(),
      s.color,
    );
  }

  String _number(double n, _Section s, NumberLocale l, bool signed) {
    for (var i = 0; i < s.percent; i++) {
      n *= 100;
    }
    for (var i = 0; i < s.scale; i++) {
      n /= 1000;
    }
    var neg = n < 0 && signed;
    n = n.abs();
    var intStr = '', fracStr = '', expStr = '';
    var num = 0, den = 1;
    if (s.exp) {
      (intStr, fracStr, expStr) = _scientific(n, math.max(s.intDigits, 1), s.fracDigits);
    } else if (s.fraction) {
      final (whole, a, b) = _fraction(n, s);
      num = a;
      den = b;
      if (whole > 0) intStr = whole.toStringAsFixed(0);
    } else {
      final r = fixed(roundDigits(n, s.fracDigits, (x) => x.roundToDouble()), s.fracDigits);
      final dot = r.indexOf('.');
      intStr = dot < 0 ? r : r.substring(0, dot);
      fracStr = dot < 0 ? '' : r.substring(dot + 1);
    }
    intStr = intStr.replaceFirst(RegExp('^0+'), '');
    if (neg && '$intStr$fracStr'.replaceAll('0', '').isEmpty && num == 0) neg = false;

    final ints = _integer(s, intStr, l);
    final b = StringBuffer();
    if (neg) b.write('-');
    var fi = 0, ii = 0;
    final done = <String>{};
    for (final t in s.tokens) {
      switch (t.kind) {
        case _Kind.literal:
          b.write(t.part == 'D' ? '$den' : t.text);
        case _Kind.decimal:
          b.write(l.decimal);
        case _Kind.slash:
          if (!done.contains('-')) b.write('/');
        case _Kind.exp:
          b.write(t.text[0]);
          if (expStr.startsWith('-')) {
            b.write('-');
          } else if (t.text[1] == '+') {
            b.write('+');
          }
        case _Kind.digit:
          switch (t.part) {
            case 'i':
              b.write(ints[ii++]);
            case 'f':
              b.write(_decimal(s, fracStr, fi++));
            default:
              if (!done.add(t.part)) continue;
              switch (t.part) {
                case 'e':
                  b.write(_pad(expStr.replaceFirst('-', ''), s.placeholders('e'), left: true));
                case 'n':
                  if (num == 0 && intStr.isNotEmpty) {
                    b.write(' ' * (s.placeholders('n').length + 1 + s.placeholders('d').length));
                    done.addAll(['d', '-']);
                    continue;
                  }
                  b.write(_pad('$num', s.placeholders('n'), left: true));
                case 'd':
                  b.write(_pad('$den', s.placeholders('d'), left: false));
              }
          }
        default:
      }
    }
    return b.toString();
  }

  List<String> _integer(_Section s, String digits, NumberLocale l) {
    final ph = s.placeholders('i');
    final k = ph.length;
    final out = List.filled(k, '');
    for (var j = 0; j < k; j++) {
      final pos = digits.length - (k - j);
      if (j == 0 && pos > 0) {
        out[j] = digits.substring(0, pos + 1);
      } else if (pos >= 0) {
        out[j] = digits[pos];
      } else if (ph[j] == '0') {
        out[j] = '0';
      } else if (ph[j] == '?') {
        out[j] = ' ';
      }
    }
    if (!s.thousands) return out;
    final count = out.fold(0, (n, o) => n + o.trim().length);
    var seen = 0;
    for (var j = 0; j < k; j++) {
      final b = StringBuffer();
      for (final ch in out[j].split('')) {
        b.write(ch);
        if (ch == ' ') continue;
        seen++;
        final left = count - seen;
        if (left > 0 && left % 3 == 0) b.write(l.group);
      }
      out[j] = b.toString();
    }
    return out;
  }

  String _decimal(_Section s, String digits, int i) {
    final ph = s.placeholders('f');
    final d = i < digits.length ? digits[i] : '0';
    if (ph[i] == '0' || digits.substring(math.min(i, digits.length)).replaceAll('0', '').isNotEmpty) return d;
    return ph[i] == '?' ? ' ' : '';
  }

  static String _pad(String digits, String ph, {required bool left}) {
    final pad = StringBuffer();
    for (var i = digits.length; i < ph.length; i++) {
      final c = left ? ph[ph.length - 1 - i] : ph[i];
      if (c == '0') pad.write('0');
      if (c == '?') pad.write(' ');
    }
    return left ? '$pad$digits' : '$digits$pad';
  }

  static (String, String, String) _scientific(double n, int intDigits, int fracDigits) {
    if (n == 0) return ('0' * intDigits, '0' * fracDigits, '0');
    var exp = (math.log(n) / math.ln10).floor();
    if (intDigits > 1) {
      exp -= ((exp % intDigits) + intDigits) % intDigits;
    } else {
      exp -= intDigits - 1;
    }
    var r = fixed(roundDigits(n / math.pow(10, exp), fracDigits, (x) => x.roundToDouble()), fracDigits);
    if (double.parse(r) >= math.pow(10, intDigits)) {
      exp += math.max(intDigits, 1);
      r = fixed(roundDigits(n / math.pow(10, exp), fracDigits, (x) => x.roundToDouble()), fracDigits);
    }
    final dot = r.indexOf('.');
    return (dot < 0 ? r : r.substring(0, dot), dot < 0 ? '' : r.substring(dot + 1), '$exp');
  }

  static (double, int, int) _fraction(double n, _Section s) {
    var whole = 0.0;
    if (s.intDigits > 0) {
      whole = n.floorToDouble();
      n -= whole;
    }
    if (s.denominator > 0) {
      final d = s.denominator;
      final num = (n * d).round();
      if (s.intDigits > 0 && num == d) return (whole + 1, 0, d);
      return (whole, num, d);
    }
    final digits = s.placeholders('d').length;
    final limit = math.pow(10, math.max(digits, 1)).toInt() - 1;
    var bestN = 0, bestD = 1;
    var bestErr = double.infinity;
    for (var d = 1; d <= limit; d++) {
      final num = (n * d).round();
      final e = (n - num / d).abs();
      if (e < bestErr - 1e-12) {
        bestN = num;
        bestD = d;
        bestErr = e;
      }
    }
    if (s.intDigits > 0 && bestN == bestD) return (whole + 1, 0, 1);
    return (whole, bestN, bestD);
  }

  String _date(double n, _Section s, NumberLocale l, bool date1904) {
    final unit = 86400 * math.pow(10, s.secDigits);
    n = (n * unit).roundToDouble() / unit;
    final day = civilOf(n, date1904: date1904);
    final secs = (n - n.floorToDouble()) * 86400;
    final h = secs ~/ 3600;
    final mi = (secs ~/ 60) % 60;
    final sec = secs % 60;
    final w = weekdayOf(n, date1904: date1904);
    String two(int v) => v.toString().padLeft(2, '0');
    final b = StringBuffer();
    for (final t in s.tokens) {
      switch (t.kind) {
        case _Kind.literal:
          b.write(t.text);
        case _Kind.decimal:
          b.write(l.decimal);
        case _Kind.digit:
          b.write(t.text);
        case _Kind.date:
          final x = t.text;
          if (x == 'y' || x == 'yy') {
            b.write(two(day.$1 % 100));
          } else if (x.startsWith('yyy') || x.startsWith('e')) {
            b.write(day.$1);
          } else if (x == 'm') {
            b.write(day.$2);
          } else if (x == 'mm') {
            b.write(two(day.$2));
          } else if (x == 'mmm') {
            b.write(l.shortMonths[day.$2 - 1]);
          } else if (x == 'mmmmm') {
            b.write(l.months[day.$2 - 1][0].toUpperCase());
          } else if (x.startsWith('mmmm')) {
            b.write(l.months[day.$2 - 1]);
          } else if (x == 'd') {
            b.write(day.$3);
          } else if (x == 'dd') {
            b.write(two(day.$3));
          } else if (x == 'ddd') {
            b.write(l.shortDays[w]);
          } else if (x.startsWith('dddd')) {
            b.write(l.days[w]);
          } else if (x == 'h' || x.startsWith('hh')) {
            var hh = h;
            if (s.ampm) {
              hh = h % 12;
              if (hh == 0) hh = 12;
            }
            b.write(x == 'h' ? '$hh' : two(hh));
          } else if (x == 'M') {
            b.write(mi);
          } else if (x.startsWith('MM')) {
            b.write(two(mi));
          } else if (x == 's') {
            b.write(sec.floor());
          } else if (x.startsWith('ss')) {
            b.write(two(sec.floor()));
          } else if (x == '[h]' || x == '[hh]') {
            b.write((n * 24).floor());
          } else if (x == '[m]' || x == '[mm]') {
            b.write((n * 1440).floor());
          } else if (x == '[s]' || x == '[ss]') {
            b.write((n * 86400).floor());
          } else if (x == 'AM/PM') {
            b.write(h < 12 ? 'AM' : 'PM');
          } else if (x == 'A/P') {
            b.write(h < 12 ? 'A' : 'P');
          } else if (x.startsWith('.')) {
            final d = fixed(sec - sec.floorToDouble(), s.secDigits);
            b.write('${l.decimal}${d.substring(d.indexOf('.') + 1)}');
          }
        default:
      }
    }
    return b.toString();
  }
}

/// A number written as General does, as Excel turns it into text: 15
/// significant digits, in exponent notation past them or below 1E-9.
String generalText(double n, NumberLocale l, {int digits = 15}) {
  if (n == 0) return '0';
  final e = n.toStringAsExponential(digits - 1);
  var mant = e.substring(0, e.indexOf('e'));
  var exp = int.parse(e.substring(e.indexOf('e') + 1));
  if (exp >= 15 || exp < -9) {
    if (mant.contains('.')) mant = mant.replaceFirst(RegExp(r'0+$'), '').replaceFirst(RegExp(r'\.$'), '');
    final sign = exp < 0 ? '-' : '+';
    exp = exp.abs();
    return '${mant.replaceFirst('.', l.decimal)}E$sign${exp.toString().padLeft(2, '0')}';
  }
  return plain(double.parse(e)).replaceFirst('.', l.decimal);
}

/// The shortest decimal writing of a number, never in exponent notation.
String plain(double n) {
  final s = n.toString();
  final e = s.indexOf('e');
  if (e < 0) return s.endsWith('.0') ? s.substring(0, s.length - 2) : s;
  final neg = s.startsWith('-');
  final mant = s.substring(neg ? 1 : 0, e);
  final exp = int.parse(s.substring(e + 1));
  final dot = mant.indexOf('.');
  var digits = mant.replaceFirst('.', '');
  var point = (dot < 0 ? mant.length : dot) + exp;
  if (point <= 0) {
    digits = '${'0' * -point}$digits';
    point = 0;
  }
  if (point >= digits.length) digits = digits.padRight(point, '0');
  var out = point == 0 ? '0.$digits' : '${digits.substring(0, point)}.${digits.substring(point)}';
  if (out.endsWith('.')) out = out.substring(0, out.length - 1);
  return neg ? '-$out' : out;
}

/// [n] with [d] decimals, never in exponent notation.
String fixed(double n, int d) {
  if (n.abs() < 1e21) return n.toStringAsFixed(d);
  return plain(n);
}

/// [n] rounded by [f] to [d] decimals, on the number as Excel shows it:
/// 15 significant digits, so that 2.675 rounds up.
double roundDigits(double n, int d, double Function(double) f) {
  if (d > 15 || n == 0) return n;
  final p = math.pow(10, d).toDouble();
  final x = double.parse((n * p).toStringAsExponential(14));
  return f(x) / p;
}

/// The year, month and day a serial number falls on, the 29th of February
/// 1900 of Excel included.
(int, int, int) civilOf(double serial, {bool date1904 = false}) {
  final days = serial.floor();
  final DateTime t;
  if (date1904) {
    t = DateTime.utc(1904, 1, 1 + days);
  } else if (days == 60) {
    return (1900, 2, 29);
  } else if (days == 0) {
    return (1900, 1, 0);
  } else if (days < 60) {
    t = DateTime.utc(1899, 12, 31 + days);
  } else {
    t = DateTime.utc(1899, 12, 30 + days);
  }
  return (t.year, t.month, t.day);
}

/// The day of the week of a serial number, 0 for Sunday.
int weekdayOf(double serial, {bool date1904 = false}) {
  var d = serial.floor();
  if (date1904) d += 1462;
  return ((d - 1) % 7 + 7) % 7;
}

/// The serial number of a day.
double serialOf(int y, int m, int d, {bool date1904 = false}) {
  final t = DateTime.utc(y, m, d);
  if (date1904) return t.difference(DateTime.utc(1904)).inDays.toDouble();
  var days = t.difference(DateTime.utc(1899, 12, 30)).inDays;
  if (days < 61) days--;
  return days.toDouble();
}
