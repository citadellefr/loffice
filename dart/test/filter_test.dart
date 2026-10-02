import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/filter.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:loffice/src/ot/grid.dart';
import 'package:loffice/src/ot/tree.dart';

void main() {
  final tree = Tree.fromEdit(Edit([
    Change.create(const Node(id: 'book', type: 'book', key: 'V')),
    Change.create(Node(id: 'S1', type: 'sheet', parent: 'book', key: 'V', attributes: const {'name': 'Feuil1'}, grid: Grid(const [
      Cell(2, 2, {'v': 'Ville'}), Cell(2, 3, {'v': 'Ventes'}),
      Cell(3, 2, {'v': 'Lyon'}), Cell(3, 3, {'v': 12}),
      Cell(4, 2, {'v': 'Paris'}), Cell(4, 3, {'v': 3}),
      Cell(5, 2, {'v': 'Lyon'}),
      Cell(6, 2, {'v': 'Nantes'}), Cell(6, 3, {'v': 7}),
      Cell(9, 2, {'v': 'ailleurs'}),
    ]))),
  ]))!;
  final book = Workbook(tree);
  final sheet = tree['S1']!;

  test('the region around a cell', () {
    expect(currentRegion(sheet, 4, 3), const CellArea(2, 2, 6, 3));
    expect(currentRegion(sheet, 9, 2), const CellArea(9, 2, 10, 2));
  });

  test('the values of a column, numbers first', () {
    const f = SheetFilter(CellArea(2, 2, 6, 3));
    final (cities, noCity) = columnValues(book, sheet, f, 2, NumberLocale.fr);
    expect(cities, ['Lyon', 'Nantes', 'Paris']);
    expect(noCity, isFalse);
    final (sales, noSale) = columnValues(book, sheet, f, 3, NumberLocale.fr);
    expect(sales, ['3', '7', '12']);
    expect(noSale, isTrue);
  });

  test('a filter hides the rows it leaves out', () {
    final f = const SheetFilter(CellArea(2, 2, 5, 3)).withColumn(2, const FilterColumn(values: {'Lyon'}));
    final e = filterEdit(book, sheet, f, NumberLocale.fr);
    expect(e.changes.first.attributes['filter'], {
      'ref': 'B2:C6',
      'cols': [
        {'col': 0, 'vals': ['Lyon']},
      ],
    });
    expect([for (final c in e.changes.last.cells!) (c.row, c.fields['hide'])], [(4, true), (6, true)]);
  });
}
