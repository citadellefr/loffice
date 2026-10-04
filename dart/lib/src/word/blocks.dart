import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../text/text_frame.dart';
import 'document.dart';
import 'paragraph.dart';

/// A block laid out in a width: a paragraph or a table.
sealed class LaidBlock {
  double get height;
}

class LaidPara extends LaidBlock {
  LaidPara(this.box, this.flow, this.start, this.section);

  final ParaBox box;

  /// The text node the paragraph is in, and where it starts there.
  final String flow;
  final int start;

  /// The section the paragraph ends, if its mark ends one.
  final WordSection? section;

  @override
  double get height => box.height;
}

/// A cell of a table laid out: where it is in its row, its blocks and what
/// frames it.
class LaidCell {
  LaidCell({
    required this.node,
    required this.x,
    required this.width,
    required this.blocks,
    required this.margins,
    required this.borders,
    required this.shading,
    required this.vAlign,
    required this.merge,
  });

  final Node node;

  /// From the table's left, and its width, margins included.
  final double x, width;
  final List<LaidBlock> blocks;

  /// Left, top, right, bottom.
  final EdgeInsets margins;
  final Map<String, Map<String, Object?>> borders;
  final Color? shading;
  final String vAlign;

  /// "restart" for a cell merged with those below, "continue" for one merged
  /// into the cell above.
  final String? merge;

  /// How many rows the cell spans, set once the table is laid out.
  var span = 1;

  double get contentHeight => stackHeight(blocks) + margins.vertical;
}

class LaidRow {
  LaidRow(this.node, this.cells, this.minHeight, this.exact, this.header, this.cantSplit);

  final Node node;
  final List<LaidCell> cells;
  final double minHeight;
  final bool exact, header, cantSplit;
  double height = 0;
}

class LaidTable extends LaidBlock {
  LaidTable(this.node, this.x, this.width, this.rows);

  final Node node;

  /// From the text's left, and the width of the grid.
  final double x, width;
  final List<LaidRow> rows;

  @override
  double get height => rows.fold(0, (h, r) => h + r.height);
}

/// The space between two paragraphs: the space after the first and before
/// the second, which Word adds up, unless contextual spacing drops them
/// between paragraphs of the same style, or both are HTML automatic
/// spacing, which collapses. A null [prev] is a table or nothing.
double gap(ParaBox? prev, ParaBox p) {
  if (prev == null) return p.spaceBefore;
  var after = prev.spaceAfter, before = p.spaceBefore;
  if (prev.style == p.style) {
    if (p.contextual) before = 0;
    if (prev.contextual) after = 0;
  }
  if (prev.autoAfter && p.autoBefore) return math.max(after, before);
  return after + before;
}

/// The height of blocks one under the other, with the space between them.
double stackHeight(List<LaidBlock> blocks) {
  var h = 0.0;
  ParaBox? prev;
  for (var i = 0; i < blocks.length; i++) {
    switch (blocks[i]) {
      case LaidPara(:final box):
        h += i == 0 && box.autoBefore ? 0 : gap(prev, box);
        h += box.height;
        prev = box;
      case final LaidTable table:
        if (prev != null) h += prev.spaceAfter;
        h += table.height;
        prev = null;
    }
  }
  if (prev != null && !prev.autoAfter) h += prev.spaceAfter;
  return h;
}

/// The offsets of blocks one under the other, as [stackHeight] stacks them.
List<double> stackOffsets(List<LaidBlock> blocks) {
  final out = <double>[];
  var h = 0.0;
  ParaBox? prev;
  for (var i = 0; i < blocks.length; i++) {
    switch (blocks[i]) {
      case LaidPara(:final box):
        h += i == 0 && box.autoBefore ? 0 : gap(prev, box);
        out.add(h);
        h += box.height;
        prev = box;
      case final LaidTable table:
        if (prev != null) h += prev.spaceAfter;
        out.add(h);
        h += table.height;
        prev = null;
    }
  }
  return out;
}

/// Paragraphs laid out already, found again by what they hold: an edit
/// lays out again only the paragraphs it changed.
class ParaCache {
  var _old = <_ParaKey, ParaBox>{};
  var _new = <_ParaKey, ParaBox>{};

  ParaBox get(ParaSource source, WordContext ctx, double width, FieldValues fields) {
    final key = _ParaKey(source, width, fields);
    return _new[key] ??= _old[key] ?? ParaBox.layout(source, ctx, width, fields: fields);
  }

  /// Forgets what the last layout did not use.
  void sweep() {
    _old = _new;
    _new = {};
  }
}

/// What a paragraph's layout depends on: not where it is in its flow.
@immutable
class _ParaKey {
  _ParaKey(ParaSource s, this.width, this.fields)
    : ops = s.ops,
      mark = s.mark,
      para = s.para,
      runBase = s.runBase,
      label = s.label?.text,
      notes = s.notes;

  final List<Op> ops;
  final Props mark, para, runBase;
  final String? label;
  final Map<int, String> notes;
  final double width;
  final FieldValues fields;

  @override
  bool operator ==(Object other) =>
      other is _ParaKey &&
      other.width == width &&
      other.label == label &&
      other.fields.page == fields.page &&
      other.fields.pages == fields.pages &&
      listEquals(other.ops, ops) &&
      mapEquals(other.mark, mark) &&
      mapEquals(other.para, para) &&
      mapEquals(other.runBase, runBase) &&
      mapEquals(other.notes, notes);

  @override
  int get hashCode => Object.hash(width, label, Object.hashAll(ops), mark.length, para.length, fields.page);
}

/// The counters of the lists and notes of a document, as its paragraphs
/// come in order.
class Counters {
  final _lists = <String, List<int?>>{};
  final _started = <String>{};
  var _footnotes = 0;
  var _endnotes = 0;

  /// The number of the next list paragraph of a level.
  ListLabel? label(WordDocument doc, Props para, Props markRun) {
    final num = para['num'];
    if (num == null || num == '0') return null;
    final numbering = doc.numbering[num];
    if (numbering == null) return null;
    final lvl = (int.tryParse(para['lvl'] ?? '') ?? 0).clamp(0, numbering.levels.length - 1);
    final levels = numbering.levels;
    final key = levels[lvl].override ? 'num:$num' : 'abs:${numbering.abstract}';
    final counts = _lists[key] ??= List.filled(9, null);
    if (_started.add('$num:$lvl') && levels[lvl].override) counts[lvl] = null;
    final level = levels[lvl];
    counts[lvl] = (counts[lvl] ?? level.start - 1) + 1;
    for (var i = lvl + 1; i < counts.length; i++) {
      final restart = i < levels.length ? levels[i].restart : null;
      if (restart == null || restart > 0) counts[i] = null;
    }
    if (level.format == 'none') return ListLabel('', {...markRun, ...level.r}, level);
    final run = {...markRun, ...level.r};
    String text;
    if (level.format == 'bullet') {
      final font = doc.typeface(level.r['font'] ?? markRun['font']);
      text = level.text.isEmpty ? '' : symbolChar(font, level.text);
      if (symbolFonts.contains(font)) run.remove('font');
    } else {
      text = level.text.replaceAllMapped(RegExp(r'%(\d)'), (m) {
        final i = int.parse(m[1]!) - 1;
        if (i < 0 || i >= levels.length) return '';
        final n = counts[i] ?? levels[i].start;
        return formatNumber(n, level.legal && i < lvl ? 'decimal' : levels[i].format);
      });
    }
    return ListLabel(text, run, level);
  }

  /// The number of a note referred to.
  String note(String kind) => kind.startsWith('endnote')
      ? formatNumber(++_endnotes, 'lowerRoman')
      : '${++_footnotes}';
}

/// A number as a list format writes it.
String formatNumber(int n, String format) {
  String letters(bool upper) {
    if (n <= 0) return '';
    final letter = String.fromCharCode((upper ? 65 : 97) + (n - 1) % 26);
    return letter * ((n - 1) ~/ 26 + 1);
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

  return switch (format) {
    'upperLetter' => letters(true),
    'lowerLetter' => letters(false),
    'upperRoman' => roman(true),
    'lowerRoman' => roman(false),
    'decimalZero' => n < 10 ? '0$n' : '$n',
    'ordinal' => n == 1 ? '1er' : '${n}e',
    'none' => '',
    _ => '$n',
  };
}

/// The formatting a table's style gives its parts: paragraphs and runs by
/// cell, and the cells' own.
class TableStyling {
  TableStyling(this.doc, Node table)
    : style = _styleOf(doc, table),
      attrs = table.attributes {
    final base = doc.tableProps(style, 'tbl');
    look = _look(attrs['look'] ?? base['look']);
    rowBand = ((attrs['rowBand'] ?? base['rowBand']) as num?)?.toInt() ?? 1;
    colBand = ((attrs['colBand'] ?? base['colBand']) as num?)?.toInt() ?? 1;
    tbl = {...base, ...attrs};
  }

  final WordDocument doc;
  final String? style;
  final Map<String, Object?> attrs;
  late final Map<String, bool> look;
  late final int rowBand, colBand;

  /// The table's attributes, those of its style under them.
  late final Map<String, Object?> tbl;

  static String? _styleOf(WordDocument doc, Node table) {
    final s = table.attributes['style'];
    return s is String && doc.styles.containsKey(s) ? s : doc.tableStyle;
  }

  static Map<String, bool> _look(Object? v) {
    if (v is Map<String, Object?>) return {for (final e in v.entries) e.key: e.value == true};
    return const {'firstRow': true, 'firstColumn': true, 'noVBand': true};
  }

  /// The parts of the table a cell is in, in the order their formatting
  /// applies.
  List<String> parts(int row, int rows, int col, int cols) {
    final out = <String>[];
    final firstRow = look['firstRow'] == true && row == 0;
    final lastRow = look['lastRow'] == true && row == rows - 1;
    final firstCol = look['firstColumn'] == true && col == 0;
    final lastCol = look['lastColumn'] == true && col == cols - 1;
    if (look['noVBand'] != true && !firstCol && !lastCol) {
      final c = col - (look['firstColumn'] == true ? 1 : 0);
      out.add((c ~/ math.max(colBand, 1)).isEven ? 'band1Vert' : 'band2Vert');
    }
    if (look['noHBand'] != true && !firstRow && !lastRow) {
      final r = row - (look['firstRow'] == true ? 1 : 0);
      out.add((r ~/ math.max(rowBand, 1)).isEven ? 'band1Horz' : 'band2Horz');
    }
    if (firstCol) out.add('firstCol');
    if (lastCol) out.add('lastCol');
    if (firstRow) out.add('firstRow');
    if (lastRow) out.add('lastRow');
    if (firstRow && firstCol) out.add('nwCell');
    if (firstRow && lastCol) out.add('neCell');
    if (lastRow && firstCol) out.add('swCell');
    if (lastRow && lastCol) out.add('seCell');
    return out;
  }

  WordStyle? get _style => style == null ? null : doc.styles[style];

  /// The paragraph and run keys the style gives a cell's text.
  (Props, Props) text(List<String> parts) {
    final p = doc.styleProps(style, 'p');
    final r = doc.styleProps(style, 'r');
    final s = _style;
    return (
      {...p, for (final part in parts) ...?s?.cond[part]?.p},
      {...r, for (final part in parts) ...?s?.cond[part]?.r},
    );
  }

  /// The cell attributes the style gives a cell: shading, borders.
  Map<String, Object?> cell(List<String> parts) {
    final s = _style;
    return {
      ...doc.tableProps(style, 'tc'),
      for (final part in parts) ...?s?.cond[part]?.tc,
    };
  }
}

Color? hexColor(Object? hex) {
  if (hex is! String || hex.length != 6) return null;
  final v = int.tryParse(hex, radix: 16);
  return v == null ? null : Color(0xFF000000 | v);
}

/// Builds the blocks of a document and lays them out, in order: the lists
/// and notes are numbered as their paragraphs come.
class BlockBuilder {
  BlockBuilder(this.ctx, this.cache, {this.fields = FieldValues.none, Counters? counters}) : counters = counters ?? Counters();

  final WordContext ctx;
  final ParaCache cache;
  final FieldValues fields;
  final Counters counters;

  /// The number of the note whose blocks are built, which its own
  /// reference mark shows.
  String? noteNumber;

  WordDocument get doc => ctx.doc;

  /// The blocks under a node, laid out in [width]; [table] the keys a
  /// table's style gives their text.
  List<LaidBlock> blocks(String parent, double width, {(Props, Props)? table}) {
    final out = <LaidBlock>[];
    for (final n in doc.tree.children(parent)) {
      switch (n.type) {
        case 'text':
          out.addAll(paragraphs(n, width, table: table));
        case 'tbl':
          out.add(this.table(n, width));
        case 'sdt':
          out.addAll(blocks(n.id, width, table: table));
      }
    }
    return out;
  }

  /// The paragraphs of a text node laid out.
  List<LaidPara> paragraphs(Node text, double width, {(Props, Props)? table}) => [
    for (final (source, section) in sources(text, table: table)) LaidPara(cache.get(source, ctx, width, fields), text.id, source.start, section),
  ];

  /// The paragraphs of a text node, numbered, and the sections they end.
  List<(ParaSource, WordSection?)> sources(Node text, {(Props, Props)? table}) {
    final out = <(ParaSource, WordSection?)>[];
    var ops = <Op>[];
    var start = 0, at = 0;
    for (final op in text.text!.ops) {
      var s = op.insert ?? '';
      while (s.isNotEmpty) {
        final i = s.indexOf('\n');
        if (i < 0) {
          ops.add(Op.insert(s, op.attributes));
          at += s.length;
          break;
        }
        if (i > 0) ops.add(Op.insert(s.substring(0, i), op.attributes));
        at += i;
        final mark = op.attributes ?? const <String, String>{};
        out.add((_source(text.id, start, ops, mark, table), mark['sect'] == null ? null : WordSection.fromJson(jsonObject(mark['sect']) ?? const {})));
        at++;
        start = at;
        ops = [];
        s = s.substring(i + 1);
      }
    }
    return out;
  }

  ParaSource _source(String flow, int start, List<Op> ops, Props mark, (Props, Props)? table) {
    final para = doc.paragraph(mark, table: table?.$1 ?? const {});
    final runBase = doc.paragraphRun(mark, table: table?.$2 ?? const {});
    final markRun = doc.run(runBase, mark);
    final notes = <int, String>{};
    var offset = 0;
    for (final op in ops) {
      final note = op.attributes?['note'];
      if (note != null) {
        for (var i = 0; i < op.insert!.length; i++) {
          final number = note == 'ref' ? noteNumber : counters.note(note);
          if (number != null) notes[offset + i] = number;
        }
      }
      offset += op.insert!.length;
    }
    return ParaSource(
      flow: flow,
      start: start,
      ops: ops,
      mark: mark,
      para: para,
      runBase: runBase,
      label: counters.label(doc, para, markRun),
      notes: notes,
    );
  }

  /// A table laid out in [width]: its grid, cells and their text.
  LaidTable table(Node node, double width) {
    final styling = TableStyling(doc, node);
    final tbl = styling.tbl;
    final rows = [for (final r in doc.tree.children(node.id)) if (r.type == 'tr') r];
    final grid = [for (final w in (tbl['grid'] as List<Object?>? ?? const [])) ((w as num?) ?? 0) / 20];
    final columns = grid.isNotEmpty ? grid.length : rows.fold(1, (n, r) => math.max(n, doc.tree.children(r.id).where((c) => c.type == 'tc').length));
    final widths = grid.isNotEmpty ? grid : List.filled(columns, width / columns);
    final gridWidth = widths.fold(0.0, (a, b) => a + b);
    final mar = tbl['mar'] as Map<String, Object?>? ?? const {};
    double margin(Map<String, Object?> m, String side, double fallback) => ((m[side] as num?)?.toDouble() ?? fallback * 20) / 20;
    final defaultMargins = EdgeInsets.fromLTRB(margin(mar, 'left', 5.4), margin(mar, 'top', 0), margin(mar, 'right', 5.4), margin(mar, 'bottom', 0));
    final ind = tbl['ind'] as Map<String, Object?>?;
    var x = ((ind?['w'] as num?) ?? 0) / 20;
    if (doc.settings.compat < 15) x -= defaultMargins.left;
    x += switch (tbl['jc']) {
      'center' => (width - gridWidth) / 2,
      'right' || 'end' => width - gridWidth,
      _ => 0.0,
    };
    final borders = (tbl['borders'] as Map<String, Object?>?) ?? const {};

    final laidRows = <LaidRow>[];
    for (var r = 0; r < rows.length; r++) {
      final row = rows[r];
      final trStyle = doc.tableProps(styling.style, 'tr');
      final tr = {...trStyle, ...row.attributes};
      var col = ((tr['before'] as num?) ?? 0).toInt();
      final cells = <LaidCell>[];
      final nodes = [for (final c in doc.tree.children(row.id)) if (c.type == 'tc') c];
      for (var k = 0; k < nodes.length; k++) {
        final c = nodes[k];
        final span = ((c.attributes['span'] as num?) ?? 1).toInt().clamp(1, 63);
        final cx = widths.take(col).fold(0.0, (a, b) => a + b);
        var cw = widths.skip(col).take(span).fold(0.0, (a, b) => a + b);
        if (cw <= 0) cw = ((c.attributes['w'] as Map<String, Object?>?)?['w'] as num? ?? 0) / 20;
        final parts = styling.parts(r, rows.length, col, columns);
        final tc = {...styling.cell(parts), ...c.attributes};
        final cm = tc['mar'] as Map<String, Object?>? ?? const {};
        final margins = EdgeInsets.fromLTRB(
          margin(cm, 'left', defaultMargins.left),
          margin(cm, 'top', defaultMargins.top),
          margin(cm, 'right', defaultMargins.right),
          margin(cm, 'bottom', defaultMargins.bottom),
        );
        final own = (tc['borders'] as Map<String, Object?>?) ?? const {};
        Map<String, Object?>? side(String name, String outer, String inner, bool edge) {
          final v = own[name] ?? borders[edge ? outer : inner];
          return v is Map<String, Object?> ? v : null;
        }

        final cellBorders = {
          'top': ?side('top', 'top', 'insideH', r == 0),
          'bottom': ?side('bottom', 'bottom', 'insideH', r == rows.length - 1),
          'left': ?side('left', 'left', 'insideV', col == 0) ?? side('start', 'start', 'insideV', col == 0),
          'right': ?side('right', 'right', 'insideV', col + span >= columns) ?? side('end', 'end', 'insideV', col + span >= columns),
        };
        final text = styling.text(parts);
        cells.add(LaidCell(
          node: c,
          x: cx,
          width: cw,
          blocks: blocks(c.id, math.max(cw - margins.horizontal, 1), table: text),
          margins: margins,
          borders: cellBorders,
          shading: hexColor(tc['shd']),
          vAlign: tc['vAlign'] as String? ?? 'top',
          merge: tc['vMerge'] as String?,
        ));
        col += span;
      }
      laidRows.add(LaidRow(
        row,
        cells,
        ((tr['h'] as num?) ?? 0) / 20,
        tr['hRule'] == 'exact',
        tr['header'] == true,
        tr['cantSplit'] == true,
      ));
    }
    _heights(laidRows);
    return LaidTable(node, x, gridWidth, laidRows);
  }

  /// The heights of rows: their tallest cell, cells merged down across rows
  /// growing the last row they span.
  static void _heights(List<LaidRow> rows) {
    for (final row in rows) {
      var h = 0.0;
      for (final c in row.cells) {
        if (c.merge == null) h = math.max(h, c.contentHeight);
      }
      row.height = row.exact ? row.minHeight : math.max(h, row.minHeight);
    }
    for (var r = 0; r < rows.length; r++) {
      for (final c in rows[r].cells) {
        if (c.merge != 'restart') continue;
        var last = r;
        while (last + 1 < rows.length && rows[last + 1].cells.any((d) => (d.x - c.x).abs() < 0.5 && d.merge == 'continue')) {
          last++;
        }
        c.span = last - r + 1;
        final spanned = rows.sublist(r, last + 1).fold(0.0, (h, x) => h + x.height);
        final need = c.contentHeight;
        if (need > spanned && !rows[last].exact) rows[last].height += need - spanned;
      }
    }
  }
}
