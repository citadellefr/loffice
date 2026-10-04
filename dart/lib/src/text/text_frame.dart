import 'dart:convert';
import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../drawing/color.dart';
import '../drawing/paint.dart';

/// Text properties as the flow's attributes have them, see the Go package
/// drawingml.
typedef Props = Map<String, String>;

/// Where fonts come from: the typefaces a document names, and the free
/// fonts with the same metrics that stand in for those that cannot be
/// shipped.
class Fonts {
  const Fonts({this.theme = _noTheme, this.substitutes = defaultSubstitutes, this.package});

  /// The typeface a theme font ("+mj-lt") stands for.
  final String Function(String) theme;
  final Map<String, String> substitutes;

  /// The package the substitutes are bundled in, for their family names.
  final String? package;

  static String _noTheme(String name) => name.startsWith('+') ? 'Calibri' : name;

  /// Metric-compatible free fonts: lines break where Office breaks them.
  static const defaultSubstitutes = {
    'Calibri': 'Carlito',
    'Calibri Light': 'Carlito',
    'Cambria': 'Caladea',
    'Arial': 'Liberation Sans',
    'Helvetica': 'Liberation Sans',
    'Times New Roman': 'Liberation Serif',
    'Courier New': 'Liberation Mono',
  };

  /// The typeface of a font name, theme fonts resolved.
  String typeface(String name) => name.startsWith('+') ? theme(name) : name;

  /// The families to ask for. A font with a free substitute is drawn with
  /// it, so that lines break the same for everyone editing; another with
  /// itself, or the substitute of Office's default font where it is
  /// missing.
  (String, List<String>) families(String name) {
    final face = typeface(name);
    final substitute = substitutes[face];
    if (substitute != null) {
      return package == null ? (substitute, [face]) : ('packages/$package/$substitute', [substitute, face]);
    }
    final fallback = substitutes['Calibri']!;
    return (face, [if (package != null) 'packages/$package/$fallback', fallback]);
  }
}

/// The height of a single spaced line, in ems, as Word and PowerPoint
/// measure it from the Windows metrics of the fonts; 1.2 for the others.
double lineHeight(String typeface) => switch (typeface) {
  'Calibri' || 'Calibri Light' || 'Carlito' => 1.2207,
  'Cambria' || 'Caladea' => 1.172,
  'Arial' || 'Liberation Sans' || 'Times New Roman' || 'Liberation Serif' || 'Helvetica' => 1.149,
  'Courier New' || 'Liberation Mono' => 1.133,
  _ => 1.2,
};

/// The text of a shape laid out in the box of its text, in points: each
/// paragraph with its style, bullet and spacing.
class TextFrame {
  TextFrame._(this.paragraphs, this.box, this.height, this._top);

  /// Lays out a flow. [levels] are the paragraph and run properties each
  /// level starts from; [body] the properties of the text body; [box] where
  /// the text goes in the shape, before insets.
  factory TextFrame.layout(
    Delta flow, {
    required Rect box,
    required Props body,
    required Props Function(int level) levels,
    required ColorContext colors,
    Fonts fonts = const Fonts(),
    Color? defaultColor,
  }) {
    double emu(String key, double fallback) => (double.tryParse(body[key] ?? '') ?? fallback) / 12700;
    final inset = Rect.fromLTRB(
      box.left + emu('lIns', 91440),
      box.top + emu('tIns', 45720),
      box.right - emu('rIns', 91440),
      box.bottom - emu('bIns', 45720),
    );
    final wrap = body['wrap'] != 'none';
    final fit = body['fit'] == 'norm';
    final scale = fit ? (double.tryParse(body['fontScale'] ?? '') ?? 100000) / 100000 : 1.0;
    final reduction = fit ? (double.tryParse(body['lnSpcReduction'] ?? '') ?? 0) / 100000 : 0.0;
    final ctx = _Context(colors, fonts, scale, reduction, defaultColor);

    final paragraphs = <ParagraphLayout>[];
    var offset = 0;
    var y = 0.0;
    final numbers = <int, int>{};
    for (final (ops, mark) in _paragraphs(flow)) {
      final lvl = int.tryParse(mark['lvl'] ?? '') ?? 0;
      final base = levels(lvl.clamp(0, 8));
      final para = {...base, for (final e in mark.entries) if (paraKeys.contains(e.key)) e.key: e.value};
      final runBase = {for (final e in base.entries) if (!paraKeys.contains(e.key)) e.key: e.value};
      numbers.removeWhere((l, _) => l > lvl);
      final bullet = para['bu'] ?? '';
      int? number;
      if (bullet.startsWith('auto:')) {
        final parts = bullet.split(':');
        final start = parts.length > 2 ? int.tryParse(parts[2]) ?? 1 : 1;
        number = numbers[lvl] = (numbers[lvl] ?? start - 1) + 1;
      } else {
        numbers.remove(lvl);
      }
      final p = ParagraphLayout._build(
        offset,
        ops,
        mark,
        para,
        runBase,
        ctx,
        width: inset.width,
        wrap: wrap,
        number: number,
        first: paragraphs.isEmpty,
      );
      p.top = y + p.spaceBefore;
      y = p.top + p.height + p.spaceAfter;
      paragraphs.add(p);
      offset += p.length + 1;
    }
    final height = y;
    final top = switch (body['anchor']) {
      'ctr' => inset.top + (inset.height - height) / 2,
      'b' => inset.bottom - height,
      _ => inset.top,
    };
    return TextFrame._(paragraphs, inset, height, top);
  }

  final List<ParagraphLayout> paragraphs;

  /// The box the text is laid out in, insets taken off.
  final Rect box;

  /// The height of the text.
  final double height;

  final double _top;

  /// The length of the flow, marks included.
  int get length => paragraphs.fold(0, (n, p) => n + p.length + 1);

  void paint(Canvas canvas) {
    for (final p in paragraphs) {
      p.paint(canvas, Offset(box.left, _top + p.top));
    }
  }

  /// The paragraph holding an offset of the flow, the mark ending it
  /// included.
  ParagraphLayout paragraphAt(int offset) {
    for (final p in paragraphs) {
      if (offset <= p.start + p.length) return p;
    }
    return paragraphs.last;
  }

  /// The offset of the flow nearest to a point.
  int offsetAt(Offset point) {
    var p = paragraphs.first;
    for (final q in paragraphs) {
      if (point.dy >= _top + q.top - q.spaceBefore) p = q;
    }
    return p.start + p.offsetAt(point - Offset(box.left, _top + p.top));
  }

  /// The caret before an offset of the flow.
  Rect caretAt(int offset) {
    final p = paragraphAt(offset);
    return p.caretAt(offset - p.start).shift(Offset(box.left, _top + p.top));
  }

  /// The start and end of the line holding an offset of the flow.
  (int, int) lineAt(int offset) {
    final p = paragraphAt(offset);
    final (start, end) = p.lineAt(offset - p.start);
    return (p.start + start, p.start + end);
  }

  /// The offset a line above (negative [lines]) or below the one at
  /// [offset], at the same distance from the left, or [x] when given.
  int verticalMove(int offset, int lines, {double? x}) {
    final caret = caretAt(offset);
    final target = Offset(x ?? caret.left, caret.center.dy + lines * caret.height);
    if (target.dy < _top) return 0;
    if (target.dy > _top + height) return length - 1;
    return offsetAt(target);
  }

  /// The boxes of a range of the flow.
  List<Rect> selection(int start, int end) {
    final out = <Rect>[];
    for (final p in paragraphs) {
      final from = math.max(start, p.start), to = math.min(end, p.start + p.length + 1);
      if (from >= to) continue;
      final origin = Offset(box.left, _top + p.top);
      out.addAll(p.selection(from - p.start, to - p.start).map((r) => r.shift(origin)));
    }
    return out;
  }
}

/// The keys of paragraph properties; the others are those of runs.
const paraKeys = {
  'lvl', 'algn', 'marL', 'marR', 'indent', 'lnSpc', 'spcBef', 'spcAft', 'rtl', 'defTabSz', 'fontAlgn', //
  'buClr', 'buSz', 'buFont', 'bu', 'tabs', 'p',
};

/// The paragraphs of a flow: their inserts, and the attributes of their
/// mark.
Iterable<(List<Op>, Map<String, String>)> _paragraphs(Delta flow) sync* {
  var ops = <Op>[];
  for (final op in flow.ops) {
    var text = op.insert ?? '';
    while (text.isNotEmpty) {
      final i = text.indexOf('\n');
      if (i < 0) {
        ops.add(Op.insert(text, op.attributes));
        break;
      }
      if (i > 0) ops.add(Op.insert(text.substring(0, i), op.attributes));
      yield (ops, op.attributes ?? const {});
      ops = [];
      text = text.substring(i + 1);
    }
  }
  if (ops.isNotEmpty) yield (ops, const {});
}

class _Context {
  _Context(this.colors, this.fonts, this.scale, this.reduction, this.defaultColor);

  final ColorContext colors;
  final Fonts fonts;
  final double scale;
  final double reduction;
  final Color? defaultColor;

  late final painter = DrawingPainter(colors);

  double size(Props run) => (double.tryParse(run['sz'] ?? '') ?? 1800) / 100 * scale;

  TextStyle style(Props run, double lineFactor) {
    final (family, fallback) = fonts.families(run['font'] ?? '+mn-lt');
    final size = this.size(run);
    final baseline = double.tryParse(run['baseline'] ?? '') ?? 0;
    final decorations = [
      if (run['u'] != null && run['u'] != 'none') TextDecoration.underline,
      if (run['strike'] != null && run['strike'] != 'noStrike') TextDecoration.lineThrough,
    ];
    Color? color;
    final fill = run['fill'];
    if (fill != null) color = painter.color(_json(fill));
    final hl = run['hl'];
    return TextStyle(
      fontFamily: family,
      fontFamilyFallback: fallback,
      fontSize: baseline != 0 ? size * 2 / 3 : size,
      fontWeight: run['b'] == '1' ? FontWeight.bold : FontWeight.normal,
      fontStyle: run['i'] == '1' ? FontStyle.italic : FontStyle.normal,
      color: color ?? defaultColor ?? const Color(0xFF000000),
      decoration: TextDecoration.combine(decorations),
      decorationStyle: run['u'] == 'dbl' ? TextDecorationStyle.double : null,
      letterSpacing: (double.tryParse(run['spc'] ?? '') ?? 0) / 100,
      height: lineFactor * lineHeight(fonts.typeface(run['font'] ?? '+mn-lt')) * (baseline != 0 ? 1.5 : 1),
      leadingDistribution: TextLeadingDistribution.proportional,
      fontFeatures: [
        if (run['cap'] == 'small') const FontFeature.enable('smcp'),
        if (baseline > 0) const FontFeature.superscripts(),
        if (baseline < 0) const FontFeature.subscripts(),
      ],
      background: hl == null ? null : (Paint()..color = colors.resolve(_json(hl)) ?? const Color(0x00000000)),
    );
  }
}

Map<String, Object?>? _json(String s) {
  try {
    final v = jsonDecode(s);
    return v is Map<String, Object?> ? v : null;
  } on FormatException {
    return null;
  }
}

/// A paragraph laid out: its lines, its bullet, the space around it.
class ParagraphLayout {
  ParagraphLayout._(
    this.start,
    this.length,
    this._painter,
    this._bullet,
    this._x,
    this._bulletX,
    this.height,
    this.spaceBefore,
    this.spaceAfter,
  );

  factory ParagraphLayout._build(
    int start,
    List<Op> ops,
    Map<String, String> mark,
    Props para,
    Props runBase,
    _Context ctx, {
    required double width,
    required bool wrap,
    required int? number,
    required bool first,
  }) {
    double emu(String key) => (double.tryParse(para[key] ?? '') ?? 0) / 12700;
    final lnSpc = para['lnSpc'] ?? '';
    final exact = lnSpc.startsWith('t') ? (double.tryParse(lnSpc.substring(1)) ?? 0) / 100 * ctx.scale : 0.0;
    final lineFactor = lnSpc.startsWith('p')
        ? math.max((double.tryParse(lnSpc.substring(1)) ?? 100000) / 100000 - ctx.reduction, 0.1)
        : 1 - ctx.reduction;
    final spans = <InlineSpan>[];
    var length = 0;
    Props? firstRun;
    for (final op in ops) {
      final run = {...runBase, for (final e in (op.attributes ?? const {}).entries) e.key: e.value};
      firstRun ??= run;
      var text = op.insert!;
      length += text.length;
      text = text.replaceAll('\v', '\n');
      if (run['cap'] == 'all') {
        final upper = text.toUpperCase();
        if (upper.length == text.length) text = upper;
      }
      spans.add(TextSpan(text: text, style: ctx.style(run, lineFactor)));
    }
    final markRun = {...runBase, for (final e in mark.entries) if (!paraKeys.contains(e.key)) e.key: e.value};
    final empty = length == 0;
    if (empty) spans.add(TextSpan(text: '​', style: ctx.style(markRun, lineFactor)));
    final align = switch (para['algn']) {
      'ctr' => TextAlign.center,
      'r' => TextAlign.right,
      'just' || 'justLow' || 'dist' || 'thaiDist' => TextAlign.justify,
      _ => TextAlign.left,
    };
    final rtl = para['rtl'] == '1';
    final marL = emu('marL'), marR = emu('marR'), indent = emu('indent');

    // the bullet, in the hanging space before the text or at its start
    TextPainter? bullet;
    final bu = para['bu'] ?? '';
    final lead = firstRun ?? markRun;
    if (!empty && bu.isNotEmpty && bu != 'none' && bu != 'blip') {
      var symbol = bu.startsWith('char:') ? bu.substring(5) : _number(bu, number ?? 1);
      final font = para['buFont'];
      if (font != null && font != 'tx') symbol = symbolChar(ctx.fonts.typeface(font), symbol);
      final size = ctx.size(lead);
      final buSz = para['buSz'] ?? 'tx';
      final bulletSize = buSz.startsWith('p')
          ? size * (double.tryParse(buSz.substring(1)) ?? 100000) / 100000
          : buSz.startsWith('t')
          ? (double.tryParse(buSz.substring(1)) ?? 1800) / 100 * ctx.scale
          : size;
      final style = ctx.style({
        ...lead,
        if (font != null && font != 'tx' && !symbolFonts.contains(ctx.fonts.typeface(font))) 'font': font,
      }, lineFactor).copyWith(fontSize: bulletSize, decoration: TextDecoration.none);
      final buClr = para['buClr'];
      final color = buClr == null || buClr == 'tx' ? null : ctx.colors.resolve(_json(buClr));
      bullet = TextPainter(
        text: TextSpan(text: symbol, style: color == null ? style : style.copyWith(color: color)),
        textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
      )..layout();
    }
    var textX = marL;
    var bulletX = marL + indent;
    if (bullet != null && indent >= 0) textX = math.max(textX, bulletX + bullet.width);
    if (bullet == null && indent > 0) textX = marL + indent;
    final available = math.max(width - textX - marR, 1.0);
    final painter = TextPainter(
      text: TextSpan(children: spans),
      textAlign: align,
      textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
      strutStyle: exact > 0 ? StrutStyle(fontSize: exact, height: 1, forceStrutHeight: true) : null,
    )..layout(maxWidth: wrap ? available : double.infinity);
    if (!wrap) {
      final slack = available - painter.width;
      textX += switch (align) {
        TextAlign.center => slack / 2,
        TextAlign.right => slack,
        _ => 0,
      };
    }
    bulletX = math.max(bulletX, 0);
    final size = ctx.size(lead);
    final line = size * lineHeight(ctx.fonts.typeface(lead['font'] ?? '+mn-lt'));
    double space(String key) {
      final v = para[key];
      if (v == null || v.length < 2) return 0;
      final n = double.tryParse(v.substring(1)) ?? 0;
      return v[0] == 'p' ? line * n / 100000 : n / 100;
    }

    return ParagraphLayout._(
      start,
      length,
      painter,
      bullet,
      textX,
      bulletX,
      painter.height,
      first ? 0 : space('spcBef'),
      space('spcAft'),
    );
  }

  /// Where the paragraph starts in the flow, and how long it is without its
  /// mark.
  final int start;
  final int length;

  final TextPainter _painter;
  final TextPainter? _bullet;
  final double _x;
  final double _bulletX;
  final double height;
  final double spaceBefore;
  final double spaceAfter;

  /// Where the paragraph's first line is, from the top of the text.
  double top = 0;

  void paint(Canvas canvas, Offset origin) {
    final bullet = _bullet;
    if (bullet != null) {
      final lines = _painter.computeLineMetrics();
      final baseline = lines.isEmpty ? _painter.height : lines.first.baseline;
      final bulletBaseline = bullet.computeLineMetrics().firstOrNull?.baseline ?? bullet.height;
      bullet.paint(canvas, origin + Offset(_bulletX, baseline - bulletBaseline));
    }
    _painter.paint(canvas, origin + Offset(_x, 0));
  }

  int offsetAt(Offset point) {
    final p = _painter.getPositionForOffset(point - Offset(_x, 0)).offset;
    return p.clamp(0, length);
  }

  Rect caretAt(int offset) {
    final at = TextPosition(offset: offset.clamp(0, length));
    final o = _painter.getOffsetForCaret(at, Rect.zero);
    final h = _painter.getFullHeightForCaret(at, Rect.zero);
    return Rect.fromLTWH(_x + o.dx, o.dy, 0, h);
  }

  (int, int) lineAt(int offset) {
    final range = _painter.getLineBoundary(TextPosition(offset: offset.clamp(0, length)));
    return (range.start.clamp(0, length), range.end.clamp(0, length));
  }

  List<Rect> selection(int from, int to) {
    final out = [
      for (final b in _painter.getBoxesForSelection(TextSelection(baseOffset: from, extentOffset: math.min(to, length))))
        b.toRect().shift(Offset(_x, 0)),
    ];
    if (to > length) {
      // the mark: a space's width at the end of the last line
      final end = caretAt(length);
      out.add(Rect.fromLTWH(end.left, end.top, 4, end.height));
    }
    return out;
  }
}

/// The text of an automatic number.
String _number(String bullet, int n) {
  final scheme = bullet.split(':').elementAtOrNull(1) ?? 'arabicPeriod';
  String digits() => '$n';
  String alpha(bool upper) {
    var s = '';
    var v = n;
    do {
      v--;
      s = String.fromCharCode((upper ? 65 : 97) + v % 26) + s;
      v ~/= 26;
    } while (v > 0);
    return s;
  }

  String roman(bool upper) {
    const values = [1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1];
    const symbols = ['m', 'cm', 'd', 'cd', 'c', 'xc', 'l', 'xl', 'x', 'ix', 'v', 'iv', 'i'];
    final s = StringBuffer();
    var v = n;
    for (var i = 0; i < values.length; i++) {
      while (v >= values[i]) {
        s.write(symbols[i]);
        v -= values[i];
      }
    }
    return upper ? s.toString().toUpperCase() : s.toString();
  }

  final String core;
  if (scheme.startsWith('alphaLc')) {
    core = alpha(false);
  } else if (scheme.startsWith('alphaUc')) {
    core = alpha(true);
  } else if (scheme.startsWith('romanLc')) {
    core = roman(false);
  } else if (scheme.startsWith('romanUc')) {
    core = roman(true);
  } else {
    core = digits();
  }
  if (scheme.endsWith('ParenBoth')) return '($core)';
  if (scheme.endsWith('ParenR')) return '$core)';
  if (scheme.endsWith('Period')) return '$core.';
  if (scheme.endsWith('Minus')) return '- $core';
  return core;
}

const symbolFonts = {'Wingdings', 'Wingdings 2', 'Wingdings 3', 'Symbol', 'Webdings'};

/// The Unicode character a bullet in a symbol font shows: files name it by
/// its code in the font, or that code in the private use area.
String symbolChar(String font, String char) {
  if (!symbolFonts.contains(font) || char.isEmpty) return char;
  var code = char.codeUnitAt(0);
  if (code >= 0xF000 && code <= 0xF0FF) code -= 0xF000;
  return (font == 'Symbol' ? _symbolChars : _wingdingsChars)[code] ?? '•';
}

const _wingdingsChars = {
  0x6C: '●', 0x6E: '■', 0x6F: '□', 0x71: '❑', 0x75: '◆', 0x76: '❖', 0x77: '⬧', 0x9F: '•', 0xA7: '▪', //
  0xA8: '◻', 0xD8: '➢', 0xDC: '➔', 0xE0: '➝', 0xE8: '➔', 0xFB: '✗', 0xFC: '✓', 0xFD: '☒', 0xFE: '☑',
};

const _symbolChars = {0x2D: '−', 0xAE: '→', 0xB7: '•', 0xD6: '√', 0xDE: '⇒', 0xF3: '∫'};
