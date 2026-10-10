import 'dart:convert';
import 'dart:io';

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/page_painter.dart';
import 'package:loffice/src/excel/print.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/trame.dart';

/// A workbook of one sheet, [rows] by [cols] of numbers.
Workbook book(int rows, int cols, {Map<String, Object?> print = const {}, List<Map<String, Object?>> names = const [], List<Cell> lines = const []}) {
  final cells = [
    ...lines,
    for (var r = 1; r <= rows; r++)
      for (var c = 1; c <= cols; c++) Cell(r, c, {'v': r * c}),
  ];
  return Workbook(Tree.fromEdit(Edit([
    Change.create(Node(id: 'book', type: 'book', key: 'V', attributes: {'names': names})),
    Change.create(Node(id: 'S1', type: 'sheet', parent: 'book', key: 'V', attributes: {'name': 'Feuil1', 'print': print}, grid: Grid(cells))),
  ]))!);
}

List<String> cut(Workbook b) => [for (final p in SheetPages(b, b.sheets.first).pages) '${p.cells}'];

void main() {
  // A4 between the normal margins leaves 10 columns of 64 pixels and 48 rows of 20
  test('cells are cut where the paper ends, down then over', () {
    expect(cut(book(100, 12)), ['A1:J48', 'A49:J96', 'A97:J100', 'K1:L48', 'K49:L96', 'K97:L100']);
    expect(cut(book(100, 12, print: {'across': true})).take(3), ['A1:J48', 'K1:L48', 'A49:J96']);
    expect(cut(book(0, 0)), isEmpty);
  });

  test('paper on its side, margins and scale change what a page holds', () {
    expect(cut(book(60, 12, print: {'land': true})), ['A1:L32', 'A33:L60']);
    expect(cut(book(100, 12, print: {'scale': 50})), ['A1:L97', 'A98:L100']);
    expect(cut(book(50, 9, print: {'margins': {'l': 0, 'r': 0, 't': 0, 'b': 0, 'head': 0, 'foot': 0}})), ['A1:I50']);
    expect(cut(book(40, 10, print: {'heads': true})), ['A1:I40', 'J1:J40']);
  });

  test('breaks start pages, hidden rows take no room', () {
    expect(cut(book(60, 4, print: {'rows': [11], 'cols': [3]})), ['A1:B10', 'A11:B58', 'A59:B60', 'C1:D10', 'C11:D58', 'C59:D60']);
    final hidden = [for (var r = 1; r <= 20; r++) Cell(r, 0, const {'hide': true})];
    expect(cut(book(60, 4, lines: hidden)), ['A1:D60']);
  });

  test('a sheet fitted to pages shrinks, and no more than it has to', () {
    final b = book(100, 12, print: {'fit': [1, 0]});
    final pages = SheetPages(b, b.sheets.first);
    expect(pages.scale, 0.85);
    expect([for (final p in pages.pages) '${p.cells}'], ['A1:L57', 'A58:L100']);
    final one = book(100, 12, print: {'fit': [1, 1], 'rows': [5]});
    expect(SheetPages(one, one.sheets.first).pages.length, 1);
    final small = book(3, 3, print: {'fit': [1, 1]});
    expect(SheetPages(small, small.sheets.first).scale, 1);
  });

  test('the area named is what prints, under the rows every page repeats', () {
    final b = book(120, 6, names: [
      {'name': '_xlnm.Print_Area', 'sheet': 'S1', 'ref': r'Feuil1!$B$3:$D$10,Feuil1!$A$20:$B$22'},
      {'name': '_xlnm.Print_Titles', 'sheet': 'S2', 'ref': r'Autre!$1:$2'},
    ]);
    expect(cut(b), ['B3:D10', 'A20:B22']);
    final titled = book(100, 3, names: [
      {'name': '_xlnm.Print_Titles', 'sheet': 'S1', 'ref': r'Feuil1!$A:$A,Feuil1!$1:$2'},
    ]);
    final pages = SheetPages(titled, titled.sheets.first).pages;
    expect([for (final p in pages) '${p.cells} ${p.titleRows} ${p.titleCols}'], ['A1:C48 null null', 'A49:C94 (1, 2) null', 'A95:C100 (1, 2) null']);
  });

  test('headers and footers are read in their three parts', () {
    final info = PageInfo(number: 2, total: 5, first: false, sheet: 'Feuil1', file: 'Budget.xlsx', now: DateTime(2026, 10, 11, 9, 5));
    const base = TextStyle(fontFamily: 'Carlito', fontSize: 11);
    final parts = headerParts('&L&A&C&"Arial,Bold"&14Page &P sur &N && &KFF0000rouge&R&D &T &F', info, base);
    expect(parts[0]!.toPlainText(), 'Feuil1');
    expect(parts[1]!.toPlainText(), 'Page 2 sur 5 & rouge');
    expect(parts[2]!.toPlainText(), '11/10/2026 09:05 Budget.xlsx');
    final [page, red] = parts[1]!.children!.cast<TextSpan>();
    expect(page.style!.fontFamily, 'Liberation Sans');
    expect(page.style!.fontSize, 14);
    expect(page.style!.fontWeight, FontWeight.bold);
    expect(red.style!.color, const Color(0xFFFF0000));
    expect(headerParts('Seul', info, base).map((p) => p?.toPlainText()), [null, 'Seul', null]);
    const setup = PrintSetup({'head': 'h', 'foot': 'f', 'even': ['eh', 'ef'], 'firstPage': ['', 'ff']});
    expect(setup.headerFooter(1, first: true), ('', 'ff'));
    expect(setup.headerFooter(2, first: false), ('eh', 'ef'));
    expect(setup.headerFooter(3, first: false), ('h', 'f'));
  });

  test('the workbooks of ../testdata print on the pages Excel gives them', () {
    Workbook open(String name) => Workbook(Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/xlsx/$name.json').readAsStringSync()))!)!);
    expect(workbookPages(open('49156')).length, 2);
    expect(workbookPages(open('WithChart')).length, 2);
    final titled = open('47737');
    final pages = SheetPages(titled, titled.sheets.first);
    expect(pages.scale, 0.63);
    expect(pages.pages[1].titleRows, (1, 6));
  });
}
