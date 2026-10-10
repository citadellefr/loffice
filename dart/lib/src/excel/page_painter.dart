import 'dart:math' as math;

import 'package:flutter/painting.dart';
import 'package:trame/trame.dart';

import '../chart/chart.dart';
import '../pdf/canvas.dart';
import 'cell_painter.dart';
import 'charts.dart';
import 'number_format.dart';
import 'print.dart';
import 'workbook.dart';

const _white = Color(0xFFFFFFFF), _black = Color(0xFF000000);

/// A page of a workbook as it prints: its size in points, and what draws
/// it.
class PrintedPage {
  const PrintedPage(this.size, this.paint);

  final Size size;
  final void Function(Canvas canvas) paint;
}

/// What a header or a footer tells of its page.
class PageInfo {
  const PageInfo({required this.number, required this.total, required this.first, required this.sheet, required this.file, required this.now});

  final int number, total;

  /// Whether the page is the first of its sheet.
  final bool first;
  final String sheet, file;
  final DateTime now;
}

/// The pages a workbook prints on: those of its sheets in view, one after
/// the other, numbered through unless a sheet says where it starts. A
/// chart sheet takes a page laid on its side.
List<PrintedPage> workbookPages(Workbook book, {NumberLocale locale = NumberLocale.fr, String? fonts, String title = '', DateTime? now}) {
  now ??= DateTime.now();
  final jobs = <(Size, void Function(Canvas, PageInfo), int, bool, String)>[];
  var number = 0;
  for (final sheet in book.sheets) {
    if (sheet.attributes['state'] != null) continue;
    final name = '${sheet.attributes['name'] ?? ''}';
    if (sheet.grid == null) {
      final spec = ChartSpec.fromJson(sheet.attributes['chart']);
      if (spec == null) continue;
      const setup = PrintSetup({'land': true});
      jobs.add((
        setup.page,
        (canvas, _) => paintSheetChart(canvas, setup.margins.deflateRect(Offset.zero & setup.page), spec, book, sheet, zoom: 1 / pixelsPerPoint, fonts: fonts),
        ++number,
        true,
        name,
      ));
      continue;
    }
    final pages = SheetPages(book, sheet);
    final painter = SheetPagePainter(pages, locale: locale, fonts: fonts);
    if (pages.pages.isNotEmpty && pages.setup.first != null) number = pages.setup.first! - 1;
    for (final (i, page) in pages.pages.indexed) {
      jobs.add((pages.setup.page, (canvas, info) => painter.paint(canvas, page, info), ++number, i == 0, name));
    }
  }
  return [
    for (final (size, paint, n, first, sheet) in jobs)
      PrintedPage(size, (canvas) => paint(canvas, PageInfo(number: n, total: jobs.length, first: first, sheet: sheet, file: title, now: now!))),
  ];
}

/// Paints the pages of a sheet: its cells at the scale they print at,
/// under the rows and beside the columns every page repeats, between the
/// header and the footer.
class SheetPagePainter {
  SheetPagePainter(this.pages, {this.locale = NumberLocale.fr, this.fonts});

  final SheetPages pages;
  final NumberLocale locale;
  final String? fonts;
  final _texts = TextCache();

  Workbook get _book => pages.book;
  Node get _sheet => pages.sheet;
  SheetLayout get _l => pages.layout;
  PrintSetup get _setup => pages.setup;

  double _width((int, int)? cols) => cols == null ? 0 : _l.x(cols.$2 + 1) - _l.x(cols.$1);
  double _height((int, int)? rows) => rows == null ? 0 : _l.y(rows.$2 + 1) - _l.y(rows.$1);

  void paint(Canvas canvas, SheetPage page, PageInfo info) {
    final size = _setup.page;
    canvas.drawRect(Offset.zero & size, Paint()..color = _white);
    final inner = _setup.margins.deflateRect(Offset.zero & size);
    final k = pages.scale / pixelsPerPoint;
    final cells = page.cells;
    final cols = (cells.left, cells.right), rows = (cells.top, cells.bottom);
    final heads = _setup.headings ? const Offset(headingWidth, headingHeight) : Offset.zero;
    final main = heads + Offset(_width(page.titleCols), _height(page.titleRows));
    final content = Size(main.dx + _width(cols), main.dy + _height(rows)) * k;

    canvas.save();
    canvas.clipRect(inner);
    canvas.translate(
      inner.left + (_setup.centerH ? math.max(0, (inner.width - content.width) / 2) : 0),
      inner.top + (_setup.centerV ? math.max(0, (inner.height - content.height) / 2) : 0),
    );
    canvas.scale(k);
    final panes = <(Offset, (int, int), (int, int))>[
      if (page.titleRows != null && page.titleCols != null) (heads, page.titleRows!, page.titleCols!),
      if (page.titleRows != null) (Offset(main.dx, heads.dy), page.titleRows!, cols),
      if (page.titleCols != null) (Offset(heads.dx, main.dy), rows, page.titleCols!),
      (main, rows, cols),
    ];
    for (final (at, r, c) in panes) {
      _pane(canvas, at, CellArea(r.$1, c.$1, r.$2, c.$2));
    }
    _charts(canvas, main, cells);
    if (_setup.headings) _headings(canvas, panes);
    canvas.restore();

    final (header, footer) = _setup.headerFooter(info.number, first: info.first);
    _margin(canvas, header, info, inner, top: true);
    _margin(canvas, footer, info, inner, top: false);
  }

  void _pane(Canvas canvas, Offset at, CellArea area) {
    final left = _l.x(area.left), top = _l.y(area.top);
    canvas.save();
    canvas.clipRect(at & Size(_l.x(area.right + 1) - left, _l.y(area.bottom + 1) - top));
    CellPainter(
      book: _book,
      sheet: _sheet,
      locale: locale,
      x: (col) => at.dx + _l.x(col) - left,
      y: (row) => at.dy + _l.y(row) - top,
      fonts: fonts,
      gridlines: _setup.gridlines,
      gridColor: _black,
      texts: _texts,
    ).paint(canvas, area);
    canvas.restore();
  }

  void _charts(Canvas canvas, Offset at, CellArea area) {
    final shown = Rect.fromLTRB(_l.x(area.left), _l.y(area.top), _l.x(area.right + 1), _l.y(area.bottom + 1));
    canvas.save();
    canvas.clipRect(at & shown.size);
    for (final c in SheetChart.of(_sheet)) {
      final r = c.rect(_l);
      if (r.overlaps(shown)) paintSheetChart(canvas, r.shift(at - shown.topLeft), c.spec, _book, _sheet, fonts: fonts);
    }
    canvas.restore();
  }

  /// The letters of the columns over the cells and the numbers of the rows
  /// beside them.
  void _headings(Canvas canvas, List<(Offset, (int, int), (int, int))> panes) {
    final style = TextStyle(fontFamily: substituteFont(_book.minorFont), fontFamilyFallback: fallbackFonts, package: fonts, fontSize: 11 * pixelsPerPoint, color: _black);
    final line = Paint()
      ..color = _black
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;
    void label(Rect box, String text) {
      if (box.isEmpty) return;
      canvas.drawRect(box, line);
      final t = _texts.get(text, style, 0, false);
      canvas.save();
      canvas.clipRect(box);
      paintText(canvas, t, box.center - Offset(t.width / 2, t.height / 2));
      canvas.restore();
    }

    final done = <String>{};
    for (final (at, rows, cols) in panes) {
      for (var c = cols.$1; c <= cols.$2; c++) {
        if (done.add('c$c')) label(Rect.fromLTWH(at.dx + _l.x(c) - _l.x(cols.$1), 0, _l.width(c), headingHeight), columnName(c));
      }
      for (var r = rows.$1; r <= rows.$2; r++) {
        if (done.add('r$r')) label(Rect.fromLTWH(0, at.dy + _l.y(r) - _l.y(rows.$1), headingWidth, _l.height(r)), '$r');
      }
    }
  }

  /// Paints a header or a footer between the margins of the page: its
  /// left, center and right parts.
  void _margin(Canvas canvas, String codes, PageInfo info, Rect inner, {required bool top}) {
    if (codes.isEmpty) return;
    final font = _book.style(null).font;
    final base = TextStyle(fontFamily: substituteFont(font.name), fontFamilyFallback: fallbackFonts, package: fonts, fontSize: font.size * pages.scale, color: _black, height: 1.2);
    final parts = headerParts(codes, info, base, scale: pages.scale);
    for (final (i, part) in parts.indexed) {
      if (part == null) continue;
      final painter = TextPainter(text: part, textAlign: const [TextAlign.left, TextAlign.center, TextAlign.right][i], textDirection: TextDirection.ltr)
        ..layout(minWidth: inner.width, maxWidth: inner.width);
      final y = top ? _setup.headerMargin : _setup.page.height - _setup.footerMargin - painter.height;
      paintText(canvas, painter, Offset(inner.left, y));
    }
  }
}

/// The left, center and right parts of a header or footer written in the
/// codes of Excel: "&L", "&C" and "&R" start a part, "&P" is the number of
/// the page, "&N" how many there are, "&D" and "&T" the date and the time,
/// "&F" the file and "&A" the sheet; "&B", "&I", "&U", "&S", a size or a
/// font in quotes format what follows. A part that says nothing is null.
List<TextSpan?> headerParts(String codes, PageInfo info, TextStyle base, {double scale = 1}) {
  final parts = [<TextSpan>[], <TextSpan>[], <TextSpan>[]];
  final text = StringBuffer();
  var part = 1;
  var style = base;
  var bold = false, italic = false, underline = false, strike = false;
  void flush() {
    if (text.isEmpty) return;
    parts[part].add(TextSpan(
      text: '$text',
      style: style.copyWith(
        fontWeight: bold ? FontWeight.bold : FontWeight.normal,
        fontStyle: italic ? FontStyle.italic : FontStyle.normal,
        decoration: TextDecoration.combine([if (underline) TextDecoration.underline, if (strike) TextDecoration.lineThrough]),
      ),
    ));
    text.clear();
  }

  String two(int n) => '$n'.padLeft(2, '0');
  for (var i = 0; i < codes.length; i++) {
    if (codes[i] != '&' || i + 1 == codes.length) {
      text.write(codes[i]);
      continue;
    }
    final code = codes[++i];
    switch (code.toUpperCase()) {
      case 'L' || 'C' || 'R':
        flush();
        part = 'LCR'.indexOf(code.toUpperCase());
        style = base;
        bold = italic = underline = strike = false;
      case 'P':
        text.write(info.number);
      case 'N':
        text.write(info.total);
      case 'D':
        text.write('${two(info.now.day)}/${two(info.now.month)}/${info.now.year}');
      case 'T':
        text.write('${two(info.now.hour)}:${two(info.now.minute)}');
      case 'F':
        text.write(info.file);
      case 'A':
        text.write(info.sheet);
      case '&':
        text.write('&');
      case 'B':
        flush();
        bold = !bold;
      case 'I':
        flush();
        italic = !italic;
      case 'U' || 'E':
        flush();
        underline = !underline;
      case 'S':
        flush();
        strike = !strike;
      case '"':
        final end = codes.indexOf('"', i + 1);
        if (end < 0) break;
        flush();
        final [name, ...styles] = codes.substring(i + 1, end).split(',');
        if (name != '-' && name.isNotEmpty) style = style.copyWith(fontFamily: substituteFont(name));
        final look = styles.join(' ').toLowerCase();
        bold = look.contains('bold');
        italic = look.contains('italic');
        i = end;
      case 'K':
        final rgb = i + 6 < codes.length ? int.tryParse(codes.substring(i + 1, i + 7), radix: 16) : null;
        flush();
        if (rgb != null) style = style.copyWith(color: Color(0xFF000000 | rgb));
        i = math.min(i + 6, codes.length - 1);
      default:
        final digits = RegExp(r'\d+').matchAsPrefix(codes, i);
        if (digits == null) break;
        flush();
        style = style.copyWith(fontSize: int.parse(digits[0]!) * scale);
        i = digits.end - 1;
    }
  }
  flush();
  return [for (final p in parts) p.isEmpty ? null : TextSpan(children: p)];
}
