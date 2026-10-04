import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/widgets.dart';
import 'package:trame/trame.dart';

import '../text/text_frame.dart';
import 'document.dart';

/// Points in a twentieth of a point, the unit of Word.
double twips(String? v, [double fallback = 0]) => (double.tryParse(v ?? '') ?? fallback * 20) / 20;

/// What paragraphs are laid out with: the document, its fonts, and what the
/// fields of a page read.
class WordContext {
  WordContext(this.doc, {Fonts? fonts})
    : fonts = fonts ?? Fonts(theme: doc.typeface),
      defaultTab = math.max(doc.settings.tab / 20, 1);

  final WordDocument doc;
  final Fonts fonts;
  final double defaultTab;

  /// The size of a run in points: Word's is 10 when nothing says.
  double size(Props run) => (double.tryParse(run['sz'] ?? '') ?? 20) / 2;

  /// The height of a single spaced line of a run, in points.
  double lineOf(Props run) => size(run) * lineHeight(doc.typeface(run['font']));

  TextStyle style(Props run, {double factor = 1, double scale = 1, Color? mark}) {
    final face = doc.typeface(run['font']);
    final (family, fallback) = fonts.families(face);
    final size = this.size(run) * scale;
    final u = run['u'];
    final struck = run['strike'] == '1' || run['dstrike'] == '1' || run['del'] != null;
    final underline = u != null && u != 'none' || run['ins'] != null;
    final color = mark ?? _color(run['color']) ?? const Color(0xFF000000);
    final highlight = _highlights[run['hl']] ?? _color(run['shd']);
    return TextStyle(
      fontFamily: family,
      fontFamilyFallback: fallback,
      fontSize: size,
      fontWeight: run['b'] == '1' ? FontWeight.bold : FontWeight.normal,
      fontStyle: run['i'] == '1' ? FontStyle.italic : FontStyle.normal,
      color: color,
      decoration: TextDecoration.combine([if (underline) TextDecoration.underline, if (struck) TextDecoration.lineThrough]),
      decorationColor: color,
      decorationStyle: switch (u) {
        'double' => TextDecorationStyle.double,
        'dotted' || 'dottedHeavy' => TextDecorationStyle.dotted,
        'dash' || 'dashedHeavy' || 'dashLong' || 'dashLongHeavy' || 'dotDash' || 'dotDotDash' => TextDecorationStyle.dashed,
        'wave' || 'wavyHeavy' || 'wavyDouble' => TextDecorationStyle.wavy,
        _ => run['dstrike'] == '1' ? TextDecorationStyle.double : null,
      },
      letterSpacing: twips(run['spc']),
      height: factor * lineHeight(face),
      leadingDistribution: TextLeadingDistribution.proportional,
      background: highlight == null ? null : (Paint()..color = highlight),
    );
  }
}

Color? _color(String? hex) {
  if (hex == null || hex.length != 6) return null;
  final v = int.tryParse(hex, radix: 16);
  return v == null ? null : Color(0xFF000000 | v);
}

const _highlights = {
  'black': Color(0xFF000000), 'blue': Color(0xFF0000FF), 'cyan': Color(0xFF00FFFF), 'green': Color(0xFF00FF00), //
  'magenta': Color(0xFFFF00FF), 'red': Color(0xFFFF0000), 'yellow': Color(0xFFFFFF00), 'white': Color(0xFFFFFFFF),
  'darkBlue': Color(0xFF000080), 'darkCyan': Color(0xFF008080), 'darkGreen': Color(0xFF008000),
  'darkMagenta': Color(0xFF800080), 'darkRed': Color(0xFF800000), 'darkYellow': Color(0xFF808000),
  'darkGray': Color(0xFF808080), 'lightGray': Color(0xFFC0C0C0),
};

/// Draws a picture stretched over a rectangle.
/// Draws an object of the text, a picture or a chart, in a box; tells
/// whether it could.
typedef DrawObject = bool Function(Canvas canvas, Map<String, Object?> picture, Rect box);

void drawPicture(Canvas canvas, ui.Image image, Rect rect) {
  final src = Rect.fromLTWH(0, 0, image.width.toDouble(), image.height.toDouble());
  canvas.drawImageRect(image, src, rect, Paint()..filterQuality = FilterQuality.medium);
}

/// The colors of the revisions of the people who made them, as Word gives
/// each author one.
const revisionColors = [Color(0xFFB5082E), Color(0xFF2E74B5), Color(0xFF538135), Color(0xFF7030A0), Color(0xFFC55A11)];

Color revisionColor(String author) => revisionColors[author.hashCode.abs() % revisionColors.length];

/// The number of a list paragraph, drawn before its first line.
class ListLabel {
  const ListLabel(this.text, this.run, this.level);

  final String text;
  final Props run;
  final WordLevel level;
}

/// A paragraph to lay out: its text in the flow and what formats it.
class ParaSource {
  const ParaSource({
    required this.flow,
    required this.start,
    required this.ops,
    required this.mark,
    required this.para,
    required this.runBase,
    this.label,
    this.notes = const {},
  });

  /// The text node and where the paragraph starts in it.
  final String flow;
  final int start;

  /// Its text, the mark left out.
  final List<Op> ops;
  final Props mark;

  /// The paragraph formatting in effect, and the runs' base.
  final Props para;
  final Props runBase;
  final ListLabel? label;

  /// The numbers of the notes referred to, by offset in the paragraph.
  final Map<int, String> notes;

  int get length => ops.fold(0, (n, o) => n + o.insert!.length);
}

/// What the fields of a paragraph read where it is drawn: the number of the
/// page, the number of pages.
class FieldValues {
  const FieldValues({this.page = '1', this.pages = '1', this.sectionPages = '1'});

  final String page, pages, sectionPages;

  static const none = FieldValues();

  String? of(String instructions) {
    final name = instructions.trim().split(RegExp(r'\s+')).firstOrNull?.toUpperCase() ?? '';
    return switch (name) {
      'PAGE' => page,
      'NUMPAGES' => pages,
      'SECTIONPAGES' => sectionPages,
      _ => null,
    };
  }
}

enum _SlotKind { hidden, tab, image, glyph, lead }

/// A placeholder of the text: what stands for a flow character that is not
/// drawn as text.
class _Slot {
  _Slot(this.kind, {this.width = 0, this.height = 0, this.text, this.style, this.rise = 0, this.picture});

  final _SlotKind kind;
  double width;
  final double height;

  /// A glyph drawn in the slot, raised by [rise].
  final String? text;
  final TextStyle? style;
  final double rise;
  final Map<String, Object?>? picture;

  /// A tab's leader: "dot", "hyphen", "underscore"…
  String? leader;
}

class _TabStop {
  const _TabStop(this.pos, this.kind, this.leader);

  final double pos;
  final String kind;
  final String? leader;
}

/// A picture that floats: drawn where its anchor says, from the paragraph
/// it is anchored in.
class FloatingPicture {
  const FloatingPicture(this.picture, this.offset);

  final Map<String, Object?> picture;

  /// Where it is anchored in the paragraph.
  final int offset;
}

/// Part of a paragraph laid out by one painter: most paragraphs are one; a
/// hanging indent without a number puts the first line in its own.
class _Piece {
  _Piece(this.painter, this.x, this.y, this.from, this.lead, this.slots);

  final TextPainter painter;

  /// Where it is in the paragraph.
  final double x, y;

  /// The offset in the paragraph of its first character, and the slots it
  /// puts before it, which the flow does not hold.
  final int from, lead;
  final List<_Slot> slots;

  int painterOffset(int offset) => offset - from + lead;

  int flowOffset(int offset) => offset - lead + from;
}

/// A line of a paragraph, in the paragraph's coordinates.
class ParaLine {
  ParaLine._(this.top, this.height, this.baseline, this._painterTop, this.start, this.end, this._piece);

  final double top, height;

  /// The baseline, from the line's top.
  final double baseline;

  /// The line's top in its painter.
  final double _painterTop;

  /// The offsets in the paragraph of its first character and after its last.
  final int start, end;
  final _Piece _piece;

  /// A break of the page or column the line ends with.
  String? breakAfter;

  double get bottom => top + height;
}

/// A paragraph laid out in a width: its lines, number and spacing.
class ParaBox {
  ParaBox._(this.source, this.width, this._pieces, this.lines, this.floats, this._label, this._labelX);

  factory ParaBox.layout(ParaSource source, WordContext ctx, double width, {FieldValues fields = FieldValues.none}) {
    final para = source.para;
    final rule = para['sp.rule'] ?? 'auto';
    final line = double.tryParse(para['sp.line'] ?? '');
    final factor = rule == 'auto' && line != null ? math.max(line / 240, 0.06) : 1.0;
    final strut = line == null || line <= 0
        ? null
        : rule == 'exact'
        ? StrutStyle(fontSize: line / 20, height: 1, forceStrutHeight: true, leading: 0)
        : rule == 'atLeast'
        ? StrutStyle(fontSize: line / 20, height: 1, leading: 0)
        : null;
    final left = twips(para['ind.left']), right = twips(para['ind.right']), first = twips(para['ind.first']);
    final stops = _stops(para['tabs'], left, first);
    final floats = <FloatingPicture>[];

    // the number, and where the text of the first line starts after it
    TextPainter? label;
    var labelX = left + first;
    var textStart = left + first;
    final l = source.label;
    if (l != null && l.text.isNotEmpty) {
      final run = l.run;
      label = TextPainter(text: TextSpan(text: l.text, style: ctx.style(run, factor: factor)), textDirection: TextDirection.ltr)..layout();
      final w = label.width;
      labelX = switch (l.level.jc) {
        'right' || 'end' => left + first - w,
        'center' => left + first - w / 2,
        _ => left + first,
      };
      final end = labelX + w;
      textStart = switch (l.level.suffix) {
        'space' => end + ctx.style(run).fontSize! * 0.25,
        'nothing' => end,
        _ => _nextStop(end, stops, left, first, ctx.defaultTab),
      };
    }

    final build = _Builder(source, ctx, factor, fields, floats);
    final ops = build.run();
    final contentWidth = math.max(width - right, 1.0);

    List<_Piece> pieces;
    if (textStart >= left - 0.01) {
      final lead = textStart - left;
      final slots = [if (lead > 0.01) _Slot(_SlotKind.lead, width: lead)];
      final piece = _layoutPiece(ops, slots, left, contentWidth - left, strut, 0, 0, source.para, ctx, stops, first: true);
      pieces = [piece];
    } else {
      // a hanging first line: laid out alone, from its own start
      final firstPiece = _layoutPiece(ops, const [], textStart, contentWidth - textStart, strut, 0, 0, source.para, ctx, stops, first: true);
      final metrics = firstPiece.painter.computeLineMetrics();
      if (metrics.length <= 1) {
        pieces = [firstPiece];
      } else {
        final end = firstPiece.painter.getLineBoundary(const TextPosition(offset: 0)).end;
        final firstOps = _slice(ops, 0, end);
        final restOps = _slice(ops, end, null);
        final a = _layoutPiece(firstOps, const [], textStart, contentWidth - textStart, strut, 0, 0, source.para, ctx, stops, first: true);
        final b = _layoutPiece(restOps, const [], left, contentWidth - left, strut, end, a.painter.height, source.para, ctx, stops, first: false);
        pieces = [a, b];
      }
    }

    final lines = <ParaLine>[];
    for (final piece in pieces) {
      final painter = piece.painter;
      final text = painter.plainText;
      var top = 0.0;
      for (final m in painter.computeLineMetrics()) {
        final range = painter.getLineBoundary(painter.getPositionForOffset(Offset(-1e6, top + m.height / 2)));
        final start = piece.flowOffset(math.max(range.start, piece.lead));
        var end = piece.flowOffset(math.min(range.end, text.length));
        // a hard break belongs to the line it ends
        if (range.end < text.length && text.codeUnitAt(range.end) == 0x0A) end++;
        lines.add(ParaLine._(piece.y + top, m.height, m.baseline - top, top, start, end, piece));
        top += m.height;
      }
    }
    for (final b in build.breaks.entries) {
      for (final ln in lines) {
        if (b.key >= ln.start && b.key < ln.end) ln.breakAfter = b.value;
      }
    }
    return ParaBox._(source, width, pieces, lines, floats, label, labelX);
  }

  final ParaSource source;
  final double width;
  final List<_Piece> _pieces;
  final List<ParaLine> lines;
  final List<FloatingPicture> floats;
  final TextPainter? _label;
  final double _labelX;

  int get length => source.length;
  Props get para => source.para;


  late final double spaceBefore = autoBefore ? 14 : twips(source.para['sp.before']);
  late final double spaceAfter = autoAfter ? 14 : twips(source.para['sp.after']);
  late final bool autoBefore = source.para['sp.beforeAuto'] == '1';
  late final bool autoAfter = source.para['sp.afterAuto'] == '1';
  late final bool contextual = source.para['contextualSpacing'] == '1';
  late final bool keepNext = source.para['keepNext'] == '1';
  late final bool keepLines = source.para['keepLines'] == '1';
  late final bool widowControl = source.para['widowControl'] == '1';
  late final bool breakBefore = source.para['pageBreakBefore'] == '1';
  late final String? style = source.mark['pstyle'];
  late final double height = lines.isEmpty ? 0 : lines.last.bottom;

  /// The left and right of the text area the paragraph's shading and
  /// borders cover.
  (double, double) get band => (twips(para['ind.left']), width - twips(para['ind.right']));

  ParaLine lineAt(int offset) {
    for (final l in lines) {
      if (offset < l.end || l == lines.last) return l;
    }
    return lines.last;
  }

  int lineIndexOf(int offset) => lines.indexOf(lineAt(offset));

  /// Draws lines [from]…[to] with their top at [origin].
  void paint(Canvas canvas, Offset origin, int from, int to, {DrawObject? objects}) {
    if (lines.isEmpty) return;
    final top = lines[from].top;
    final bottom = lines[to - 1].bottom;
    canvas.save();
    canvas.clipRect(Rect.fromLTRB(-1e5, origin.dy - 1, 1e5, origin.dy + bottom - top + 1));
    final shift = origin - Offset(0, top);
    final label = _label;
    if (label != null && from == 0) {
      final metrics = label.computeLineMetrics().firstOrNull;
      final base = lines.first.baseline;
      label.paint(canvas, shift + Offset(_labelX, base - (metrics?.baseline ?? label.height)));
    }
    for (final piece in _pieces) {
      final at = shift + Offset(piece.x, piece.y);
      piece.painter.paint(canvas, at);
      final boxes = piece.painter.inlinePlaceholderBoxes ?? const [];
      for (var i = 0; i < boxes.length && i < piece.slots.length; i++) {
        _paintSlot(canvas, piece.slots[i], boxes[i].toRect().shift(at), objects);
      }
    }
    canvas.restore();
  }

  void _paintSlot(Canvas canvas, _Slot slot, Rect box, DrawObject? objects) {
    switch (slot.kind) {
      case _SlotKind.glyph:
        final p = TextPainter(text: TextSpan(text: slot.text, style: slot.style), textDirection: TextDirection.ltr)..layout();
        final base = p.computeLineMetrics().firstOrNull?.baseline ?? p.height;
        p.paint(canvas, Offset(box.left, box.bottom - base - slot.rise));
      case _SlotKind.image:
        if (objects == null || !objects(canvas, slot.picture ?? const {}, box)) {
          canvas.drawRect(box, Paint()..color = const Color(0xFFE8E8E8));
          canvas.drawRect(box.deflate(0.5), Paint()
            ..style = PaintingStyle.stroke
            ..color = const Color(0xFFB0B0B0));
        }
      case _SlotKind.tab:
        final leader = slot.leader;
        if (leader == null || leader == 'none' || box.width < 2) return;
        final char = switch (leader) {
          'hyphen' => '-',
          'underscore' || 'heavy' => '_',
          'middleDot' => '·',
          _ => '.',
        };
        final style = slot.style ?? const TextStyle(fontSize: 11, color: Color(0xFF000000));
        final unit = TextPainter(text: TextSpan(text: char, style: style), textDirection: TextDirection.ltr)..layout();
        if (unit.width <= 0) return;
        final n = (box.width / unit.width).floor();
        final dots = TextPainter(text: TextSpan(text: char * n, style: style), textDirection: TextDirection.ltr)..layout();
        final base = dots.computeLineMetrics().firstOrNull?.baseline ?? dots.height;
        dots.paint(canvas, Offset(box.right - dots.width, box.bottom - base));
      case _SlotKind.hidden || _SlotKind.lead:
    }
  }

  /// The offset in the paragraph nearest to a point of the paragraph.
  int offsetAt(Offset p) {
    var line = lines.first;
    for (final l in lines) {
      if (p.dy >= l.top) line = l;
    }
    final piece = line._piece;
    final pos = piece.painter.getPositionForOffset(Offset(p.dx - piece.x, line._painterTop + line.height / 2));
    final o = piece.flowOffset(math.max(pos.offset, piece.lead));
    return math.min(math.max(o, line.start), math.max(line.start, _lineEnd(line)));
  }

  /// The last caret position of a line: before its break, or the mark.
  int _lineEnd(ParaLine l) {
    if (l == lines.last) return length;
    final text = l._piece.painter.plainText;
    final end = l._piece.painterOffset(l.end);
    if (end > 0 && end <= text.length && text.codeUnitAt(end - 1) == 0x0A) return l.end - 1;
    return l.end;
  }

  /// The caret before an offset, in the paragraph's coordinates.
  Rect caretAt(int offset) {
    final line = lineAt(offset);
    final piece = line._piece;
    final at = TextPosition(offset: piece.painterOffset(offset.clamp(0, length)));
    final o = piece.painter.getOffsetForCaret(at, Rect.zero);
    final h = piece.painter.getFullHeightForCaret(at, Rect.zero);
    final top = line.top + line.baseline - h * 0.8;
    return Rect.fromLTWH(piece.x + o.dx, math.max(top, line.top), 0, math.min(h, line.height));
  }

  /// The start and end offsets of the line holding [offset].
  (int, int) lineRange(int offset) {
    final l = lineAt(offset);
    return (l.start, _lineEnd(l));
  }

  /// The boxes covering a range of the paragraph, the mark included when
  /// [to] passes the text.
  List<Rect> selection(int from, int to) {
    final out = <Rect>[];
    for (final l in lines) {
      final a = math.max(from, l.start), b = math.min(to, math.min(l.end, length));
      if (b <= a) continue;
      final piece = l._piece;
      for (final box in piece.painter.getBoxesForSelection(TextSelection(baseOffset: piece.painterOffset(a), extentOffset: piece.painterOffset(b)))) {
        out.add(Rect.fromLTRB(piece.x + box.left, l.top, piece.x + math.max(box.right, box.left + 2), l.bottom));
      }
    }
    if (to > length) {
      final end = caretAt(length);
      final l = lines.last;
      out.add(Rect.fromLTRB(end.left, l.top, end.left + 5, l.bottom));
    }
    return out;
  }
}

/// The ops [from]…[to] of a list whose inserts are text, in UTF-16 units.
List<_Op> _slice(List<_Op> ops, int from, int? to) {
  final out = <_Op>[];
  var at = 0;
  for (final o in ops) {
    final n = o.length;
    final a = math.max(from - at, 0), b = to == null ? n : math.min(to - at, n);
    if (b > a) out.add(o.slice(a, b));
    at += n;
  }
  return out;
}

/// A span of the painter's text: text in a style, or a slot.
class _Op {
  _Op.text(this.text, this.style) : slot = null;
  _Op.slot(_Slot this.slot, this.style) : text = '￼';

  final String text;
  final TextStyle style;
  final _Slot? slot;

  int get length => text.length;

  _Op slice(int a, int b) => slot != null ? this : _Op.text(text.substring(a, b), style);
}

/// Turns the flow of a paragraph into the spans of a painter, one
/// character of the painter for each of the flow.
class _Builder {
  _Builder(this.source, this.ctx, this.factor, this.fields, this.floats);

  final ParaSource source;
  final WordContext ctx;
  final double factor;
  final FieldValues fields;
  final List<FloatingPicture> floats;

  /// The page and column breaks, by offset in the paragraph.
  final breaks = <int, String>{};

  final _out = <_Op>[];
  final _fields = <_Field>[];
  String? _simpleWrap;

  List<_Op> run() {
    var offset = 0;
    for (final op in source.ops) {
      final attrs = op.attributes ?? const <String, String>{};
      final run = ctx.doc.run(source.runBase, attrs);
      final revision = attrs['del'] ?? attrs['ins'];
      final color = revision == null ? null : revisionColor(revision);
      final style = ctx.style({...run, if (attrs['del'] != null) 'del': '1', if (attrs['ins'] != null) 'ins': '1'}, factor: factor, mark: color);
      final text = op.insert!;
      if (attrs.keys.any(wordObjectKeys.contains)) {
        for (var i = 0; i < text.length; i++) {
          _object(attrs, run, style, offset + i);
        }
      } else {
        _text(text, attrs, run, style, offset);
      }
      offset += text.length;
    }
    // the mark: the height of an empty last line
    final mark = ctx.doc.run(source.runBase, source.mark);
    _out.add(_Op.text('​', ctx.style(mark, factor: factor)));
    return _out;
  }

  void _hidden(TextStyle style) => _out.add(_Op.slot(_Slot(_SlotKind.hidden), style));

  void _glyph(String text, Props run, TextStyle style, {double rise = 0, double scale = 1}) {
    final glyphStyle = scale == 1 ? style : style.copyWith(fontSize: style.fontSize! * scale);
    final p = TextPainter(text: TextSpan(text: text, style: glyphStyle), textDirection: TextDirection.ltr)..layout();
    final ascent = p.computeLineMetrics().firstOrNull?.ascent ?? p.height;
    _out.add(_Op.slot(_Slot(_SlotKind.glyph, width: p.width, height: ascent + rise, text: text, style: glyphStyle, rise: rise), style));
  }

  void _object(Attributes attrs, Props run, TextStyle style, int offset) {
    final fld = attrs['fld'];
    if (fld != null) {
      switch (fld) {
        case 'begin':
          _fields.add(_Field());
        case 'separate':
          final f = _fields.lastOrNull;
          if (f != null) {
            f.result = true;
            f.value = fields.of(f.instructions.toString());
          }
        case 'end':
          final f = _fields.isEmpty ? null : _fields.removeLast();
          if (f != null && f.value != null && !f.shown) {
            _glyph(f.value!, run, style);
            return;
          }
      }
      _hidden(style);
      return;
    }
    final instr = attrs['instr'];
    if (instr != null) {
      _fields.lastOrNull?.instructions.write(instr);
      _hidden(style);
      return;
    }
    final br = attrs['br'];
    if (br != null) {
      if (br == 'page' || br == 'column') breaks[offset] = br;
      _out.add(_Op.text('\n', style));
      return;
    }
    final img = jsonObject(attrs['img']);
    if (img != null) {
      final w = ((img['w'] as num?) ?? 0) / 12700, h = ((img['h'] as num?) ?? 0) / 12700;
      if (img['float'] != null) {
        floats.add(FloatingPicture(img, offset));
        _hidden(style);
      } else {
        _out.add(_Op.slot(_Slot(_SlotKind.image, width: w, height: h, picture: img), style));
      }
      return;
    }
    final sym = attrs['sym'];
    if (sym != null) {
      final i = sym.indexOf(':');
      final font = i < 0 ? '' : sym.substring(0, i);
      final code = int.tryParse(i < 0 ? sym : sym.substring(i + 1), radix: 16);
      final char = code == null ? '□' : symbolChar(font, String.fromCharCode(code));
      _glyph(char, run, style);
      return;
    }
    final note = attrs['note'];
    if (note != null) {
      final n = source.notes[offset] ?? '';
      if (n.isEmpty) {
        _hidden(style);
      } else {
        _glyph(n, run, style, rise: style.fontSize! * 0.33, scale: 0.67);
      }
      return;
    }
    _hidden(style);
  }

  void _text(String text, Attributes attrs, Props run, TextStyle style, int offset) {
    final hide = run['vanish'] == '1';
    final va = run['va'];
    final field = _fields.lastOrNull;
    final simple = attrs['field'];
    final simpleValue = simple == null ? null : fields.of(simple);
    // a field the client made has no wrap: its runs follow one another
    final wrap = simple == null ? null : attrs['wrap'] ?? 'field:$simple';
    final firstOfSimple = simple != null && wrap != _simpleWrap;
    _simpleWrap = wrap;
    if (run['caps'] == '1') {
      final upper = text.toUpperCase();
      if (upper.length == text.length) text = upper;
    }
    var plain = StringBuffer();
    void flush() {
      if (plain.isNotEmpty) _out.add(_Op.text(plain.toString(), style));
      plain = StringBuffer();
    }

    for (var i = 0; i < text.length; i++) {
      final c = text[i];
      // what a field of the page reads, drawn in place of its result
      if (field != null && field.result && field.value != null) {
        flush();
        if (!field.shown) {
          field.shown = true;
          _glyph(field.value!, run, style);
        } else {
          _hidden(style);
        }
        continue;
      }
      if (simpleValue != null) {
        flush();
        if (i == 0 && firstOfSimple) {
          _glyph(simpleValue, run, style);
        } else {
          _hidden(style);
        }
        continue;
      }
      if (hide) {
        flush();
        _hidden(style);
        continue;
      }
      if (c == '\t') {
        flush();
        _out.add(_Op.slot(_Slot(_SlotKind.tab), style));
        continue;
      }
      if (c == '\v') {
        plain.write('\n');
        continue;
      }
      if (va == 'superscript' || va == 'subscript') {
        flush();
        final size = style.fontSize!;
        _glyph(c, run, style, rise: va == 'superscript' ? size * 0.33 : -size * 0.08, scale: 0.67);
        continue;
      }
      plain.write(c);
    }
    flush();
  }
}

class _Field {
  final instructions = StringBuffer();
  var result = false;
  String? value;
  var shown = false;
}

/// The tab stops of a paragraph, in points from the text's left.
List<_TabStop> _stops(String? tabs, double left, double first) {
  final out = <_TabStop>[];
  for (final t in (tabs ?? '').split(' ')) {
    final parts = t.split(':');
    if (parts.length < 2) continue;
    final pos = double.tryParse(parts[1]);
    if (pos == null || parts[0] == 'clear' || parts[0] == 'bar') continue;
    out.add(_TabStop(pos / 20, parts[0], parts.length > 2 ? parts[2] : null));
  }
  out.sort((a, b) => a.pos.compareTo(b.pos));
  return out;
}

/// The next stop after [x]: a custom one, the hanging indent, or a default
/// one.
double _nextStop(double x, List<_TabStop> stops, double left, double first, double interval) =>
    _stopAfter(x, stops, left, first, interval).pos;

_TabStop _stopAfter(double x, List<_TabStop> stops, double left, double first, double interval) {
  for (final s in stops) {
    if (s.pos > x + 0.01) {
      if (first < 0 && left > x + 0.01 && left < s.pos) return _TabStop(left, 'left', null);
      return s;
    }
  }
  if (first < 0 && left > x + 0.01) return _TabStop(left, 'left', null);
  return _TabStop((x / interval + 1e-6).floor() * interval + interval, 'left', null);
}

/// Lays out spans with a painter, their tabs set one after another where
/// their stops put the text after them.
_Piece _layoutPiece(
  List<_Op> ops,
  List<_Slot> lead,
  double x,
  double width,
  StrutStyle? strut,
  int from,
  double y,
  Props para,
  WordContext ctx,
  List<_TabStop> stops, {
  required bool first,
}) {
  final slots = <_Slot>[...lead, for (final o in ops) ?o.slot];
  final align = switch (para['jc']) {
    'center' => TextAlign.center,
    'right' || 'end' => TextAlign.right,
    'both' || 'distribute' || 'mediumKashida' || 'highKashida' || 'lowKashida' || 'thaiDistribute' => TextAlign.justify,
    _ => TextAlign.left,
  };
  final base = ops.isEmpty ? const TextStyle() : ops.first.style;
  TextPainter paint() {
    final spans = <InlineSpan>[
      for (final _ in lead) _slotSpan(base),
      for (final o in ops) o.slot != null ? _slotSpan(o.style) : TextSpan(text: o.text, style: o.style),
    ];
    return TextPainter(
      text: TextSpan(children: spans),
      textAlign: align,
      textDirection: para['bidi'] == '1' ? TextDirection.rtl : TextDirection.ltr,
      strutStyle: strut,
    )
      ..setPlaceholderDimensions([for (final s in slots) PlaceholderDimensions(size: Size(s.width, s.height), alignment: ui.PlaceholderAlignment.baseline, baseline: TextBaseline.alphabetic, baselineOffset: s.height)])
      ..layout(maxWidth: math.max(width, 1));
  }

  var painter = paint();
  final tabs = [for (var i = 0; i < slots.length; i++) if (slots[i].kind == _SlotKind.tab) i];
  final left = twips(para['ind.left']), firstIndent = twips(para['ind.first']);
  // the tabs of a line are set one after the other, each moving those
  // after it; laid out again until the lines stay as they are
  for (var pass = 0; pass < 5 && tabs.isNotEmpty; pass++) {
    final boxes = painter.inlinePlaceholderBoxes ?? const [];
    var changed = false;
    var shift = 0.0;
    double? line;
    for (var k = 0; k < tabs.length; k++) {
      final i = tabs[k];
      if (i >= boxes.length) break;
      final box = boxes[i];
      if (line == null || (box.top - line).abs() > 0.5) shift = 0;
      line = box.top;
      final at = x + box.left + shift;
      final stop = _stopAfter(at, stops, left, firstIndent, ctx.defaultTab);
      // the text after the tab, up to the next tab or the end of its line
      final next = k + 1 < tabs.length && tabs[k + 1] < boxes.length && (boxes[tabs[k + 1]].top - box.top).abs() < 0.5 ? boxes[tabs[k + 1]].left : null;
      final segment = math.max((next ?? _lineRight(painter, box.top, box.bottom)) - box.right, 0.0);
      final w = math.max(switch (stop.kind) {
        'right' || 'end' || 'decimal' => stop.pos - at - segment,
        'center' => stop.pos - at - segment / 2,
        _ => stop.pos - at,
      }, 0.0);
      shift += w - slots[i].width;
      if ((w - slots[i].width).abs() > 0.1) changed = true;
      slots[i].width = w;
      slots[i].leader = stop.leader;
    }
    if (!changed) break;
    painter = paint();
  }
  return _Piece(painter, x, y, from, lead.length, slots);
}

/// The right end of the text of the line between [top] and [bottom].
double _lineRight(TextPainter painter, double top, double bottom) {
  for (final m in painter.computeLineMetrics()) {
    final lineTop = m.baseline - m.ascent;
    if (lineTop <= bottom && lineTop + m.height >= top) return m.left + m.width;
  }
  return painter.width;
}

/// The span of a slot: the painter asks for its size, nothing builds it.
InlineSpan _slotSpan(TextStyle style) =>
    WidgetSpan(child: const SizedBox.shrink(), alignment: ui.PlaceholderAlignment.baseline, baseline: TextBaseline.alphabetic, style: style);
