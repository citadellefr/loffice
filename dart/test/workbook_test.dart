import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/chart/chart.dart';
import 'package:loffice/src/excel/charts.dart';
import 'package:loffice/src/excel/conditional.dart';
import 'package:loffice/src/excel/input.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:loffice/src/ot/grid.dart';
import 'package:loffice/src/ot/tree.dart';

Tree book(List<Cell> cells, {Map<String, Object?> attributes = const {}}) => Tree.fromEdit(Edit([
  Change.create(const Node(id: 'book', type: 'book', key: 'V', attributes: {
    'theme': {'colors': ['FFFFFF', '000000', 'E7E6E6', '44546A', '4472C4', 'ED7D31', 'A5A5A5', 'FFC000', '5B9BD5', '70AD47', '0563C1', '954F72']},
  })),
  Change.create(Node(id: 'S1', type: 'sheet', parent: 'book', key: 'V', attributes: {'name': 'Feuil1', ...attributes}, grid: Grid(cells))),
  Change.create(const Node(id: 'x1', type: 'xf', key: 'V', attributes: {
    'style': {
      'fmt': '#,##0.00 "€"',
      'font': {'name': 'Calibri', 'sz': 12, 'b': true, 'color': {'theme': 4, 'tint': 0.4}},
      'fill': {'pattern': 'solid', 'fg': {'rgb': 'FFFF00'}},
      'border': {'b': {'style': 'double', 'color': {'rgb': 'FF0000'}}},
      'align': {'h': 'center', 'wrap': true},
    },
  })),
]))!;

void main() {
  test('styles resolve colors, fonts and formats', () {
    final w = Workbook(book(const []));
    final s = w.style('x1');
    expect(s.font.bold, isTrue);
    expect(s.font.size, 12);
    expect(s.fill, const Color(0xFFFFFF00));
    expect(s.bottom!.style, 'double');
    expect(s.horizontal, 'center');
    expect(s.font.color!.toARGB32(), tinted(const Color(0xFF4472C4), 0.4).toARGB32());
    final t = cellText({'v': 1234.5, 's': 'x1'}, s, NumberLocale.fr);
    expect(t.text, '1 234,50 €');
    expect(t.align, 'center');
    expect(cellText({'e': '#VALUE!'}, w.style(null), NumberLocale.fr).text, '#VALEUR!');
    expect(cellText({'v': true}, w.style(null), NumberLocale.fr).text, 'VRAI');
    expect(cellText({'v': 12}, w.style(null), NumberLocale.fr).align, 'right');
  });

  test('conditional formats change the formats of cells', () {
    final tree = book(const [], attributes: {
      'cf': [
        {'type': 'formula', 'style': {'font': {'color': {'rgb': '9C0006'}}, 'fill': {'pattern': 'solid', 'fg': {'rgb': 'FFC7CE'}}}},
        {'type': 'formula', 'style': {'font': {'i': true, 'color': {'rgb': '00B050'}}}},
        {'type': 'colorScale', 'colors': [{'rgb': 'FF0000'}, {'rgb': 'FFFF00'}, {'rgb': '00FF00'}]},
        {'type': 'dataBar', 'colors': [{'rgb': '638EC6'}], 'noValue': true},
      ],
    });
    tree.apply(Edit([
      Change.create(Node(id: 'S1.cf', type: 'looks', parent: 'S1', key: 'V', grid: Grid(const [
        Cell(1, 1, {'r': [0, 1], 's': [2, 0.75]}),
        Cell(2, 1, {'s': [2, 0.25], 'b': [3, 40]}),
      ]))),
    ]));
    final w = Workbook(tree);
    final looks = w.looks(tree['S1']!);
    final a1 = looks.style(w.style('x1'), 'x1', 1, 1);
    expect(a1.font.color, const Color(0xFF9C0006));
    expect(a1.font.italic, isTrue);
    expect(a1.font.bold, isTrue);
    expect(a1.fill, const Color(0xFFFFC7CE));
    expect(looks.style(w.style(null), null, 2, 1).fill, scaleColor(const [Color(0xFFFF0000), Color(0xFFFFFF00), Color(0xFF00FF00)], 0.25));
    expect(looks.bar(2, 1), (40.0, const Color(0xFF638EC6)));
    expect(looks.hidesValue(2, 1), isTrue);
    expect(looks.hidesValue(1, 1), isFalse);
    expect(scaleColor(const [Color(0xFF000000), Color(0xFFFFFFFF)], 0.5)!.toARGB32(), 0xFF808080);
  });

  test('palette colors become theme tints', () {
    expect(cellColor({'scheme': 'accent1', 'mods': [['lumMod', 20000], ['lumOff', 80000]]}), {'theme': 4, 'tint': 0.8});
    expect(cellColor({'scheme': 'lt1', 'mods': [['lumMod', 85000]]}), {'theme': 0, 'tint': -0.15});
    expect(cellColor({'rgb': 'FF0000'}), {'rgb': 'FF0000'});
  });

  test('layout of rows and columns', () {
    final tree = book([
      const Cell(0, 2, {'w': 20}),
      const Cell(0, 4, {'hide': true}),
      const Cell(3, 0, {'h': 30}),
      const Cell(5, 0, {'hide': true}),
      const Cell(2, 2, {'m': [2, 3], 'v': 'fusion'}),
    ], attributes: {'frozen': {'r': 1, 'c': 1}});
    final l = SheetLayout(tree['S1']!);
    expect(l.width(1), 64);
    expect(l.width(2), 140);
    expect(l.width(4), 0);
    expect(l.x(3), 204);
    expect(l.y(3), 40);
    expect(l.y(4), 80);
    expect(l.y(6), 100);
    expect(l.rowAt(79), 3);
    expect(l.rowAt(81), 4);
    expect(l.colAt(203), 2);
    expect(l.mergeAt(3, 4), const CellArea(2, 2, 3, 4));
    expect(l.frozenRows, 1);
    expect(l.used, 5);
  });

  test('typing into cells', () {
    final now = DateTime(2024, 6, 1);
    expect(parseInput('1 234,5', now: now).fields['v'], 1234.5);
    expect(parseInput('1 234,5', now: now).format, '#,##0');
    expect(parseInput('12,5 %', now: now).fields['v'], closeTo(0.125, 1e-12));
    expect(parseInput('12,5 %', now: now).format, '0.0%');
    expect(parseInput('12,50 €', now: now).fields['v'], 12.5);
    expect(parseInput('25/12/2024', now: now).fields['v'], 45651);
    expect(parseInput('25/12/2024', now: now).format, 'dd/mm/yyyy');
    expect(parseInput('12:30', now: now).fields['v'], closeTo(0.5208333, 1e-6));
    expect(parseInput('=SOMME(A1;A2)', now: now).fields['f'], 'SUM(A1,A2)');
    expect(parseInput("'123", now: now).fields['v'], '123');
    expect(parseInput('vrai', now: now).fields['v'], true);
    expect(parseInput('#N/A', now: now).fields['e'], '#N/A');
    expect(parseInput('1.5', now: now).fields['v'], '1.5');
    expect(parseInput('Bonjour', now: now).fields['v'], 'Bonjour');
    expect(parseInput('', now: now).fields, {'v': null, 'f': null, 'e': null, 'rich': null});
  });

  test('text a cell is edited from', () {
    expect(editText({'f': 'SUM(A1,A2)', 'v': 3}), '=SOMME(A1;A2)');
    expect(editText({'v': 0.1 + 0.2}), '0,3');
    expect(editText({'v': 45651}, format: NumberFormat('dd/mm/yyyy')), '25/12/2024');
    expect(editText({'v': 0.25}, format: NumberFormat('0%')), '25%');
  });

  test('names of cells', () {
    expect(columnName(28), 'AB');
    expect(cellName(3, 16384), 'XFD3');
    expect(parseCell(r'$B$7'), (7, 2));
    expect(const CellArea(3, 2, 1, 1).name, 'A1:B3');
    expect(const CellArea(1, 2, maxRows, 3).name, 'B:C');
    expect(parseArea(r'$B:C'), const CellArea(1, 2, maxRows, 3));
    expect(parseArea('2:4'), const CellArea(2, 1, 4, maxCols));
    expect(parseArea('A:4'), isNull);
  });

  test('charts read their series from the cells, hidden rows left out', () {
    final w = Workbook(book([
      const Cell(3, 0, {'hide': true}),
      const Cell(1, 1, {'v': 'Mois'}),
      const Cell(2, 1, {'v': 'Janvier'}),
      const Cell(3, 1, {'v': 'Février'}),
      const Cell(4, 1, {'v': 'Mars'}),
      const Cell(2, 2, {'v': 1234.5, 's': 'x1'}),
      const Cell(3, 2, {'v': 7}),
      const Cell(4, 2, {'e': '#N/A'}),
    ], attributes: {
      'charts': [
        {
          'from': {'col': 3, 'row': 1, 'dx': 9525 * 10},
          'to': {'col': 5, 'row': 10},
          'chart': {'plots': <Object?>[]},
        },
      ],
    }));
    final sheet = w.sheets.first;
    final values = liveData(w, sheet, DataSpec(ref: r'Feuil1!$B$2:$B$4', numbers: const [0, 0, 0], count: 3), NumberLocale.fr)!;
    expect(values.numbers, [1234.5, null]);
    expect(values.format, '#,##0.00 "€"');
    final names = liveData(w, sheet, DataSpec(ref: r"('Feuil1'!$A$2:$A$4)", texts: const ['', '', ''], count: 3), NumberLocale.fr)!;
    expect(names.texts, ['Janvier', 'Mars']);
    expect(liveData(w, sheet, DataSpec(ref: r'Autre!$A$1'), NumberLocale.fr), isNull);
    expect(liveData(w, sheet, DataSpec(ref: r'Feuil1!$A$1:$B$2'), NumberLocale.fr), isNull);

    final charts = SheetChart.of(sheet);
    expect(charts, hasLength(1));
    final l = w.layout(sheet);
    expect(charts.first.rect(l), Rect.fromPoints(Offset(l.x(4) + 10, l.y(2)), Offset(l.x(6), l.y(11))));
  });
}
