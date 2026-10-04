import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import 'blocks.dart';
import 'document.dart';
import 'paragraph.dart';

/// Where text is on a page: the body, a header or footer, or a text box,
/// which is drawn but not edited.
enum PageArea { body, header, footer, drawing }

/// Lines of a paragraph drawn on a page.
class PlacedLines {
  PlacedLines(this.para, this.from, this.to, this.origin, this.area);

  final LaidPara para;

  /// The lines, and where the top-left of the first is on the page: the left
  /// of the text the paragraph is laid out in.
  final int from, to;
  final Offset origin;
  final PageArea area;

  ParaBox get box => para.box;

  /// The top of the paragraph's first line on the page, whether drawn here
  /// or not.
  double get paraTop => origin.dy - box.lines[from].top;

  double get top => origin.dy;

  double get bottom => origin.dy + box.lines[to - 1].bottom - box.lines[from].top;

  /// The offsets of the paragraph's flow the lines hold.
  int get start => para.start + box.lines[from].start;

  int get end => para.start + (to == box.lines.length ? box.length + 1 : box.lines[to - 1].end);
}

/// A cell of a table drawn on a page: its shading and borders.
class PlacedCell {
  PlacedCell(this.cell, this.rect);

  final LaidCell cell;
  final Rect rect;
}

/// A picture floating on a page.
class PlacedPicture {
  PlacedPicture(this.picture, this.rect, this.behind);

  final Map<String, Object?> picture;
  final Rect rect;
  final bool behind;
}

/// A page of the document laid out.
class WordPage {
  WordPage(this.index, this.section, this.sectionIndex, this.firstOfSection, this.number, this.size, this.body);

  final int index;
  final WordSection section;
  final int sectionIndex;
  final bool firstOfSection;

  /// The number the page shows, and its size and text area, in points.
  final int number;
  final Size size;
  final Rect body;

  final lines = <PlacedLines>[];
  final cells = <PlacedCell>[];
  final pictures = <PlacedPicture>[];

  /// Lines drawn over the text: the separator of the footnotes.
  final rules = <(Offset, Offset)>[];

  /// The header and footer drawn on the page, if any.
  String? header, footer;
}

/// A Word document laid out in pages.
class WordLayout {
  WordLayout._(this.doc, this.pages, this._index);

  /// Lays out a document; [cache] keeps the paragraphs of the last layout,
  /// so that only those an edit changed are laid out again.
  factory WordLayout(WordDocument doc, WordContext ctx, ParaCache cache) {
    final p = _Paginator(doc, ctx, cache);
    p.run();
    cache.sweep();
    final index = <String, List<(int, PlacedLines)>>{};
    for (final page in p.pages) {
      for (final l in page.lines) {
        (index[l.para.flow] ??= []).add((page.index, l));
      }
    }
    return WordLayout._(doc, p.pages, index);
  }

  final WordDocument doc;
  final List<WordPage> pages;
  final Map<String, List<(int, PlacedLines)>> _index;

  /// The lines of a flow on the pages, in order; a header's on every page
  /// it is drawn on.
  List<(int, PlacedLines)> placementsOf(String flow) => _index[flow] ?? const [];

  /// The placed lines holding an offset of a flow, on [page] if the flow is
  /// drawn more than once.
  (int, PlacedLines)? placementAt(String flow, int offset, {int? page}) {
    final all = placementsOf(flow);
    (int, PlacedLines)? found;
    for (final e in all) {
      final l = e.$2;
      if (offset >= l.start && offset < l.end || offset == l.end && l.to == l.box.lines.length && offset <= l.para.start + l.box.length) {
        if (page == null || e.$1 == page) return e;
        found ??= e;
      }
    }
    return found;
  }

  /// The caret before an offset of a flow: its page and rectangle there.
  (int, Rect)? caret(String flow, int offset, {int? page}) {
    final at = placementAt(flow, offset, page: page);
    if (at == null) return null;
    final (pageIndex, l) = at;
    final local = offset - l.para.start;
    final r = l.box.caretAt(local);
    return (pageIndex, r.shift(Offset(l.origin.dx, l.paraTop)));
  }

  /// The flow and offset nearest to a point of a page, in [area] only.
  (String, int)? hit(int page, Offset p, {PageArea area = PageArea.body}) {
    if (page < 0 || page >= pages.length) return null;
    PlacedLines? best;
    var distance = double.infinity;
    for (final l in pages[page].lines) {
      if (l.area != area) continue;
      final left = l.origin.dx - 10, right = l.origin.dx + l.box.width + 10;
      final dy = p.dy < l.top ? l.top - p.dy : (p.dy > l.bottom ? p.dy - l.bottom : 0.0);
      final dx = p.dx < left ? left - p.dx : (p.dx > right ? p.dx - right : 0.0);
      final d = dy * 4 + dx;
      if (d < distance) {
        distance = d;
        best = l;
      }
    }
    if (best == null) return null;
    final local = Offset(p.dx - best.origin.dx, p.dy - best.paraTop);
    final firstTop = best.box.lines[best.from].top, lastBottom = best.box.lines[best.to - 1].bottom;
    final clamped = Offset(local.dx, local.dy.clamp(firstTop, lastBottom - 0.01));
    return (best.para.flow, best.para.start + best.box.offsetAt(clamped));
  }

  /// The rectangles of a range of a flow, by page.
  List<(int, Rect)> selection(String flow, int start, int end) {
    final out = <(int, Rect)>[];
    for (final (page, l) in placementsOf(flow)) {
      final a = math.max(start, l.start), b = math.min(end, l.end);
      if (b <= a) continue;
      final top = l.box.lines[l.from].top, bottom = l.box.lines[l.to - 1].bottom;
      for (final r in l.box.selection(a - l.para.start, b - l.para.start)) {
        if (r.bottom <= top + 0.01 || r.top >= bottom - 0.01) continue;
        out.add((page, r.shift(Offset(l.origin.dx, l.paraTop))));
      }
    }
    return out;
  }

  /// The start and end of the line holding an offset of a flow.
  (int, int)? lineRange(String flow, int offset) {
    final at = placementAt(flow, offset);
    if (at == null) return null;
    final l = at.$2;
    final (a, b) = l.box.lineRange(offset - l.para.start);
    return (l.para.start + a, l.para.start + b);
  }
}

/// Places the blocks of the body on pages, section by section.
class _Paginator {
  _Paginator(this.doc, this.ctx, this.cache) : builder = BlockBuilder(ctx, cache);

  final WordDocument doc;
  final WordContext ctx;
  final ParaCache cache;
  final BlockBuilder builder;
  final pages = <WordPage>[];

  late List<WordSection> _sections;
  final _heights = <(String?, double), double>{};

  late WordSection _section;
  var _sectionIndex = 0;
  var _pageInSection = 0;
  var _number = 0;
  var _column = 0;
  late double _colLeft, _colWidth, _top, _bottom;
  var _y = 0.0;
  var _atTop = true;
  var _natural = false;
  ParaBox? _prev;
  var _started = false;

  /// The heights of the page the text may not run over: those of pictures
  /// it goes around.
  final _bands = <(double, double)>[];

  /// The footnotes of the page, and the room they take at its bottom; the
  /// endnotes referred to so far; the notes laid out, by node and width.
  final _pageNotes = <List<LaidBlock>>[];
  var _notesHeight = 0.0;
  final _endnotes = <(String, String)>[];
  final _laidNotes = <(String, double), List<LaidBlock>>{};

  /// The room the separator of the footnotes takes.
  static const _separator = 12.0;

  /// Where the text of the column ends: above the footnotes.
  double get _limit => _bottom - _notesHeight;

  WordPage get _page => pages.last;

  static double _columnWidth(WordSection s) {
    final text = (s.width - s.left - s.right - s.gutter) / 20;
    final n = s.columns;
    return math.max((text - (n - 1) * s.columnSpace / 20) / n, 10);
  }

  void run() {
    _sections = [for (final (s, _, _) in doc.sections) s];
    final items = <(LaidBlock, int)>[];
    var si = 0;
    void walk(String parent) {
      for (final n in doc.tree.children(parent)) {
        final width = _columnWidth(_sections[math.min(si, _sections.length - 1)]);
        switch (n.type) {
          case 'text':
            for (final (source, section) in builder.sources(n)) {
              items.add((LaidPara(cache.get(source, ctx, width, FieldValues.none), n.id, source.start, section), math.min(si, _sections.length - 1)));
              if (section != null) si++;
            }
          case 'tbl':
            items.add((builder.table(n, width), math.min(si, _sections.length - 1)));
          case 'sdt':
            walk(n.id);
        }
      }
    }

    walk('body');
    for (var i = 0; i < items.length; i++) {
      final (block, section) = items[i];
      if (i == 0 || section != items[i - 1].$2) _startSection(section, first: i == 0);
      switch (block) {
        case final LaidPara p:
          _para(p, items, i);
        case final LaidTable t:
          _table(t);
      }
    }
    if (pages.isEmpty) _startSection(0, first: true);
    _endnotesAfter();
    _flushNotes();
    _headers();
  }

  // pages

  void _startSection(int index, {required bool first}) {
    _section = _sections[index];
    _sectionIndex = index;
    if (!first && _section.type == 'continuous') {
      _column = 0;
      _setColumn();
      return;
    }
    _pageInSection = 0;
    if (_section.pageStart != null) _number = _section.pageStart! - 1;
    if (!first && (_section.type == 'evenPage' || _section.type == 'oddPage')) {
      final next = _number + 1;
      if ((next.isEven) != (_section.type == 'evenPage')) _number++;
    }
    _newPage(natural: false);
  }

  void _newPage({required bool natural}) {
    final s = _section;
    _pageInSection++;
    _number++;
    final size = Size(s.width / 20, s.height / 20);
    final even = _number.isEven;
    var left = s.left / 20, right = s.right / 20;
    if (doc.settings.mirror && even) (left, right) = (right, left);
    final gutter = s.gutter / 20;
    var topMargin = s.top / 20;
    if (doc.settings.gutterTop) {
      topMargin += gutter;
    } else {
      left += gutter;
    }
    final width = size.width - left - right;
    final header = _headerOf(s, _sectionIndex, _pageInSection == 1, even, true);
    final footer = _headerOf(s, _sectionIndex, _pageInSection == 1, even, false);
    final top = topMargin < 0 ? -topMargin : math.max(topMargin, s.header / 20 + _height(header, width));
    final bottomMargin = s.bottom / 20;
    final bottom = bottomMargin < 0 ? -bottomMargin : math.max(bottomMargin, s.footer / 20 + _height(footer, width));
    _flushNotes();
    final page = WordPage(pages.length, s, _sectionIndex, _pageInSection == 1, _number, size, Rect.fromLTRB(left, top, size.width - right, size.height - bottom))
      ..header = header
      ..footer = footer;
    pages.add(page);
    _bands.clear();
    _column = 0;
    _natural = natural;
    _setColumn();
  }

  void _setColumn() {
    final body = _page.body;
    _colWidth = _columnWidth(_section);
    _colLeft = body.left + _column * (_colWidth + _section.columnSpace / 20);
    _top = body.top;
    _bottom = body.bottom;
    _y = _top;
    _atTop = true;
    _prev = null;
  }

  void _nextColumn({required bool natural}) {
    if (_column + 1 < _section.columns) {
      _column++;
      _natural = natural;
      _setColumn();
    } else {
      _newPage(natural: natural);
    }
  }

  /// The header or footer node of a page: the section's of its kind, or
  /// those of the sections before when it has none.
  String? _headerOf(WordSection s, int index, bool first, bool even, bool header) {
    final kind = first && s.titlePage ? 'first' : (even && doc.settings.evenOdd ? 'even' : 'default');
    for (var i = index; i >= 0; i--) {
      final map = header ? _sections[i].headers : _sections[i].footers;
      final id = map[kind];
      if (id != null && doc.tree[id] != null) return id;
    }
    return null;
  }

  double _height(String? node, double width) {
    if (node == null) return 0;
    return _heights[(node, width)] ??= stackHeight(BlockBuilder(ctx, cache).blocks(node, width));
  }

  // paragraphs

  /// The space above a paragraph where it is placed: none at the top of a
  /// page the text flowed to.
  double _spacing(ParaBox p) {
    if (!_started && p.autoBefore) return 0;
    if (_atTop && _natural) return 0;
    return _atTop ? p.spaceBefore : gap(_prev, p);
  }

  void _para(LaidPara lp, List<(LaidBlock, int)> items, int index) {
    final p = lp.box;
    final lines = p.lines;
    if (p.breakBefore && !_atTop) _newPage(natural: false);
    if (!_atTop) {
      final need = _keepHeight(items, index);
      if (_y + need > _limit + 0.01 && need <= _bottom - _top) _nextColumn(natural: true);
    }
    var space = _spacing(p);
    var i = 0;
    while (i < lines.length) {
      var n = 0;
      var y = _y + space;
      double? below;
      // the footnotes of the lines take room at the bottom of the page
      var pending = 0.0;
      while (i + n < lines.length) {
        final line = lines[i + n];
        var extra = _footnotes(lp, line).fold(0.0, (h, n) => h + _noteHeight(n.$1, n.$2));
        if (extra > 0 && _pageNotes.isEmpty && pending == 0) extra += _separator;
        if (y + line.height > _limit - pending - extra + 0.01) break;
        below = _below(y, y + line.height);
        if (below != null) break;
        y += line.height;
        pending += extra;
        n++;
        if (line.breakAfter != null) break;
      }
      final hard = n > 0 ? lines[i + n - 1].breakAfter : null;
      if (hard == null && below == null && i + n < lines.length) n = _keep(p, i, n);
      if (n == 0 && _atTop && below == null) n = 1;
      if (n > 0) {
        _place(lp, i, i + n, space);
        final h = lines[i + n - 1].bottom - lines[i].top;
        _y += space + h;
        _atTop = false;
        space = 0;
      }
      i += n;
      if (below != null) {
        // the text goes on under a picture it may not run over
        _y = below;
        continue;
      }
      if (hard == 'page') {
        _newPage(natural: false);
      } else if (hard == 'column') {
        _nextColumn(natural: false);
      } else if (i < lines.length) {
        _nextColumn(natural: true);
        if (i == 0) space = _spacing(p);
      }
    }
    _prev = p;
    _started = true;
  }

  /// Widow and orphan control and keeping lines together, for a paragraph
  /// that would break after its line i+n-1.
  int _keep(ParaBox p, int i, int n) {
    if (p.keepLines && i == 0) return 0;
    final total = p.lines.length;
    if (p.widowControl && total > 1) {
      if (total - (i + n) == 1 && n > 0) n--;
      if (i == 0 && n == 1) n = 0;
    }
    return n;
  }

  /// How much of the column a paragraph needs to start in it: its first
  /// lines, and those of the paragraphs it is kept with.
  double _keepHeight(List<(LaidBlock, int)> items, int index) {
    double first(ParaBox p) {
      var n = 1;
      if (p.keepLines) {
        n = p.lines.length;
      } else if (p.widowControl && p.lines.length > 1) {
        n = 2;
      }
      return p.lines.take(n).fold(0.0, (h, l) => h + l.height);
    }

    final p = (items[index].$1 as LaidPara).box;
    var need = _spacing(p);
    if (!p.keepNext) return need + first(p);
    ParaBox? prev;
    for (var j = index; j < items.length; j++) {
      switch (items[j].$1) {
        case LaidPara(:final box, :final section):
          if (j > index) need += gap(prev, box);
          if (!box.keepNext || j + 1 == items.length || section != null) return need + first(box);
          need += box.height;
          prev = box;
        case final LaidTable t:
          return need + (t.rows.isEmpty ? 0 : t.rows.first.height);
      }
    }
    return need;
  }

  void _place(LaidPara lp, int from, int to, double space) {
    final origin = Offset(_colLeft, _y + space);
    final placed = PlacedLines(lp, from, to, origin, PageArea.body);
    _page.lines.add(placed);
    if (from == 0) _floats(lp, placed);
    for (var k = from; k < to; k++) {
      for (final (id, number) in _footnotes(lp, lp.box.lines[k])) {
        if (_pageNotes.isEmpty) _notesHeight += _separator;
        _pageNotes.add(_note(id, number));
        _notesHeight += _noteHeight(id, number);
      }
      for (final ref in _refs(lp, lp.box.lines[k], 'endnote')) {
        if (!_endnotes.contains(ref)) _endnotes.add(ref);
      }
    }
  }

  // notes

  /// The notes a line of a paragraph refers to, of a kind: their nodes and
  /// numbers.
  List<(String, String)> _refs(LaidPara lp, ParaLine line, String kind) {
    final source = lp.box.source;
    final out = <(String, String)>[];
    var offset = 0;
    for (final op in source.ops) {
      final note = op.attributes?['note'];
      final n = op.insert!.length;
      if (note != null && note.startsWith('$kind:') && offset + n > line.start && offset < line.end) {
        final id = '${kind[0]}n${note.substring(kind.length + 1)}';
        if (doc.tree[id] != null) out.add((id, source.notes[math.max(offset, line.start)] ?? ''));
      }
      offset += n;
    }
    return out;
  }

  List<(String, String)> _footnotes(LaidPara lp, ParaLine line) => _refs(lp, line, 'footnote');

  /// A note laid out in the width of the column, its mark showing its
  /// number.
  List<LaidBlock> _note(String id, String number) => _laidNotes[(id, _colWidth)] ??= (BlockBuilder(ctx, cache)..noteNumber = number).blocks(id, _colWidth);

  double _noteHeight(String id, String number) => stackHeight(_note(id, number));

  /// Draws the footnotes of the page at its bottom, under their separator.
  void _flushNotes() {
    if (_pageNotes.isEmpty || pages.isEmpty) return;
    final page = _page;
    var y = page.body.bottom - _notesHeight + _separator;
    page.rules.add((Offset(page.body.left, y - _separator / 2), Offset(page.body.left + 144, y - _separator / 2)));
    for (final blocks in _pageNotes) {
      _placeBlocks(page, blocks, Offset(page.body.left, y), PageArea.body);
      y += stackHeight(blocks);
    }
    _pageNotes.clear();
    _notesHeight = 0;
  }

  /// The endnotes after the text, under a separator.
  void _endnotesAfter() {
    if (_endnotes.isEmpty) return;
    final items = <(LaidBlock, int)>[
      for (final (id, number) in _endnotes)
        for (final b in _note(id, number)) (b, _sectionIndex),
    ];
    if (_y + _separator > _limit) _nextColumn(natural: true);
    _page.rules.add((Offset(_colLeft, _y + _separator / 2), Offset(_colLeft + 144, _y + _separator / 2)));
    _y += _separator;
    _prev = null;
    for (var i = 0; i < items.length; i++) {
      switch (items[i].$1) {
        case final LaidPara p:
          _para(p, items, i);
        case final LaidTable t:
          _table(t);
      }
    }
  }

  void _floats(LaidPara lp, PlacedLines placed) {
    for (final f in lp.box.floats) {
      final pic = f.picture;
      final fl = pic['float'] as Map<String, Object?>? ?? const {};
      final w = ((pic['w'] as num?) ?? 0) / 12700, h = ((pic['h'] as num?) ?? 0) / 12700;
      final page = _page;
      Rect refX() => switch (fl['relX']) {
        'page' => Rect.fromLTWH(0, 0, page.size.width, 0),
        'margin' || 'leftMargin' || 'rightMargin' || 'insideMargin' || 'outsideMargin' => Rect.fromLTRB(page.body.left, 0, page.body.right, 0),
        _ => Rect.fromLTWH(_colLeft, 0, _colWidth, 0),
      };
      Rect refY() => switch (fl['relY']) {
        'page' => Rect.fromLTWH(0, 0, 0, page.size.height),
        'margin' || 'topMargin' || 'bottomMargin' => Rect.fromLTRB(0, page.body.top, 0, page.body.bottom),
        'line' => Rect.fromLTWH(0, placed.origin.dy, 0, 0),
        _ => Rect.fromLTWH(0, placed.paraTop, 0, 0),
      };
      final rx = refX(), ry = refY();
      final x = switch (fl['alignX']) {
        'center' => rx.left + (rx.width - w) / 2,
        'right' || 'outside' => rx.right - w,
        'left' || 'inside' => rx.left,
        _ => rx.left + ((fl['x'] as num?) ?? 0) / 12700,
      };
      final y = switch (fl['alignY']) {
        'center' => ry.top + (ry.height - h) / 2,
        'bottom' || 'outside' => ry.bottom - h,
        'top' || 'inside' => ry.top,
        _ => ry.top + ((fl['y'] as num?) ?? 0) / 12700,
      };
      final rect = Rect.fromLTWH(x, y, w, h);
      page.pictures.add(PlacedPicture(pic, rect, fl['behind'] == true));
      final wrap = fl['wrap'];
      final wide = w >= _colWidth * 0.6;
      if (fl['behind'] != true && (wrap == 'topAndBottom' || wide && (wrap == 'square' || wrap == 'tight' || wrap == 'through'))) {
        _bands.add((rect.top, rect.bottom));
      }
      final text = Delta.fromJson(pic['text']);
      if (text != null && text.length > 0) {
        final box = Node(id: 'drawing:${lp.flow}:${lp.start + f.offset}', type: 'text', key: 'V', text: text);
        final width = math.max(w - 14.4, 1.0);
        final blocks = [for (final (source, _) in builder.sources(box)) LaidPara(cache.get(source, ctx, width, FieldValues.none), box.id, source.start, null)];
        _placeBlocks(page, blocks, Offset(x + 7.2, y + 3.6), PageArea.drawing);
      }
    }
  }

  // tables

  /// The bottom of the picture the text between [top] and [bottom] would
  /// run over, null when it runs over none.
  double? _below(double top, double bottom) {
    double? out;
    for (final (a, b) in _bands) {
      if (top < b && bottom > a) out = math.max(out ?? b, b);
    }
    return out;
  }

  void _table(LaidTable t) {
    if (_prev != null && !_atTop) _y += _prev!.spaceAfter;
    _prev = null;
    final headers = t.rows.takeWhile((r) => r.header).toList();
    for (var r = 0; r < t.rows.length; r++) {
      final row = t.rows[r];
      if (_y + row.height > _limit + 0.01 && !_atTop) {
        _nextColumn(natural: true);
        if (r >= headers.length) {
          for (final h in headers) {
            _row(t, h, t.rows.indexOf(h));
          }
        }
      }
      _row(t, row, r);
    }
    _started = true;
  }

  void _row(LaidTable t, LaidRow row, int r) {
    final below = _below(_y, _y + row.height);
    if (below != null) {
      _y = below;
      if (_y + row.height > _limit + 0.01) _nextColumn(natural: true);
    }
    _placeRow(_page, t, row, r, Offset(_colLeft + t.x, _y));
    _y += row.height;
    _atTop = false;
  }

  void _placeRow(WordPage page, LaidTable t, LaidRow row, int r, Offset at, {PageArea area = PageArea.body}) {
    for (final c in row.cells) {
      if (c.merge == 'continue') continue;
      var h = row.height;
      for (var k = 1; k < c.span && r + k < t.rows.length; k++) {
        h += t.rows[r + k].height;
      }
      final rect = Rect.fromLTWH(at.dx + c.x, at.dy, c.width, h);
      page.cells.add(PlacedCell(c, rect));
      final content = stackHeight(c.blocks);
      final free = h - c.margins.vertical - content;
      final shift = switch (c.vAlign) {
        'center' => math.max(free / 2, 0.0),
        'bottom' => math.max(free, 0.0),
        _ => 0.0,
      };
      _placeBlocks(page, c.blocks, Offset(rect.left + c.margins.left, rect.top + c.margins.top + shift), area);
    }
  }

  /// Places blocks one under the other from [at], without breaking them
  /// across pages: those of a cell, a header or a footer.
  void _placeBlocks(WordPage page, List<LaidBlock> blocks, Offset at, PageArea area) {
    final offsets = stackOffsets(blocks);
    for (var i = 0; i < blocks.length; i++) {
      final origin = at + Offset(0, offsets[i]);
      switch (blocks[i]) {
        case final LaidPara p:
          if (p.box.lines.isEmpty) continue;
          page.lines.add(PlacedLines(p, 0, p.box.lines.length, origin, area));
        case final LaidTable t:
          var y = origin.dy;
          for (var r = 0; r < t.rows.length; r++) {
            _placeRow(page, t, t.rows[r], r, Offset(origin.dx + t.x, y), area: area);
            y += t.rows[r].height;
          }
      }
    }
  }

  // headers and footers

  void _headers() {
    final total = '${pages.length}';
    for (final page in pages) {
      final number = formatNumber(page.number, page.section.pageFormat);
      final fields = FieldValues(page: number, pages: total, sectionPages: '${pages.where((p) => p.sectionIndex == page.sectionIndex).length}');
      final builder = BlockBuilder(ctx, cache, fields: fields);
      final width = page.body.width;
      final header = page.header;
      if (header != null) {
        final blocks = builder.blocks(header, width);
        _placeBlocks(page, blocks, Offset(page.body.left, page.section.header / 20), PageArea.header);
      }
      final footer = page.footer;
      if (footer != null) {
        final blocks = builder.blocks(footer, width);
        final h = stackHeight(blocks);
        _placeBlocks(page, blocks, Offset(page.body.left, page.size.height - page.section.footer / 20 - h), PageArea.footer);
      }
    }
  }
}

/// The node a header or footer is in: its "hdr" or "ftr" ancestor.
String? headerNodeOf(Tree tree, String id) {
  var n = tree[id];
  while (n != null) {
    if (n.type == 'hdr' || n.type == 'ftr') return n.id;
    n = tree[n.parent];
  }
  return null;
}
