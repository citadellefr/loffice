import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/sheet_view.dart';
import 'package:loffice/src/excel/tables.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/testing.dart';

const _existing = {
  'id': 1,
  'name': 'Tableau1',
  'ref': 'F1:G3',
  'filter': true,
  'style': 'TableStyleMedium2',
  'rows': true,
  'columns': [
    {'id': 1, 'name': 'Nom'},
    {'id': 2, 'name': 'Prix'},
  ],
};

Edit _workbook({List<Cell> cells = const [], List<Object?> tables = const [_existing]}) => Edit([
  Change.create(
    Node(
      id: 'book',
      type: 'book',
      key: 'V',
      attributes: {
        'names': [
          {'name': 'Tableau2', 'ref': 'Feuil1!\$A\$1'},
        ],
      },
    ),
  ),
  Change.create(Node(id: 'S1', type: 'sheet', parent: 'book', key: 'K', attributes: {'name': 'Feuil1', 'tables': tables}, grid: Grid(cells))),
]);

const _region = [
  Cell(1, 1, {'v': 'Nom'}),
  Cell(1, 2, {'v': 'Nom'}),
  Cell(1, 3, {'v': 'qté'}),
  Cell(1, 4, {'v': 'QTÉ'}),
  Cell(1, 5, {}),
  Cell(2, 1, {'v': 'a'}),
];

void main() {
  Workbook book(Tree tree) => Workbook(tree);
  Tree tree({List<Cell> cells = const [], List<Object?> tables = const [_existing]}) => Tree.fromEdit(_workbook(cells: cells, tables: tables))!;

  group('Insertion > Tableau', () {
    test('makes a table named after the free names of the workbook, its columns after its headers', () {
      final t = tree(cells: _region);
      final sheet = t['S1']!;
      final edits = insertTableEdits(book(t), sheet, const CellArea(1, 1, 3, 5), headers: true, locale: NumberLocale.fr);
      expect(edits.length, 1);
      expect(t.apply(edits.single), isNotNull);
      final tables = SheetTable.of(t['S1']!);
      expect(tables.length, 2);
      final made = tables.last;
      expect(made.json['id'], 0);
      expect(made.name, 'Tableau3');
      expect(made.area, const CellArea(1, 1, 3, 5));
      expect(made.filter && made.rows && !made.cols, isTrue);
      expect(made.style, 'TableStyleMedium2');
      expect([for (final c in made.json['columns']! as List<Object?>) (c! as Map)['name']], ['Nom', 'Nom2', 'qté', 'QTÉ2', 'Colonne5']);
      final header = [for (var c = 1; c <= 5; c++) t['S1']!.grid!.cell(1, c)?['v']];
      expect(header, ['Nom', 'Nom2', 'qté', 'QTÉ2', 'Colonne5']);
    });

    test('inserts a row of Colonne1… above the range without headers', () {
      final t = tree(
        cells: [
          const Cell(1, 1, {'v': 1}),
          const Cell(1, 2, {'v': 2}),
        ],
      );
      final edits = insertTableEdits(book(t), t['S1']!, const CellArea(1, 1, 1, 2), headers: false, locale: NumberLocale.fr);
      expect(edits.length, 2);
      expect(edits.first.changes.single.dim, dimRows);
      expect(edits.last.changes.map((c) => c.kind), isNot(contains(ChangeKind.insert)));
      for (final e in edits) {
        expect(t.apply(e), isNotNull);
      }
      final made = SheetTable.of(t['S1']!).last;
      expect(made.area, const CellArea(1, 1, 2, 2));
      expect([for (final c in made.json['columns']! as List<Object?>) (c! as Map)['name']], ['Colonne1', 'Colonne2']);
      expect(t['S1']!.grid!.cell(1, 1)?['v'], 'Colonne1');
      expect(t['S1']!.grid!.cell(2, 1)?['v'], 1);
    });

    test('adds a row of data to a header row alone', () {
      final t = tree(
        cells: const [
          Cell(1, 1, {'v': 'A'}),
          Cell(1, 2, {'v': 'B'}),
        ],
      );
      expect(tableProblem(book(t), t['S1']!, const CellArea(1, 1, 1, 2), headers: true), isNull);
      final edits = insertTableEdits(book(t), t['S1']!, const CellArea(1, 1, 1, 2), headers: true, locale: NumberLocale.fr);
      expect(t.apply(edits.single), isNotNull);
      expect(SheetTable.of(t['S1']!).last.area, const CellArea(1, 1, 2, 2));
    });

    test('refuses a range that overlaps a table, merged cells or is not cells', () {
      final t = tree(
        cells: const [
          Cell(2, 2, {
            'm': [1, 2],
          }),
        ],
      );
      final b = book(t), sheet = t['S1']!;
      expect(tableProblem(b, sheet, const CellArea(1, 1, 3, 3), headers: true), TableProblem.merged);
      expect(tableProblem(b, sheet, const CellArea(2, 7, 5, 8), headers: true), TableProblem.overlap);
      expect(tableProblem(b, sheet, const CellArea(4, 4, 6, 5), headers: true), isNull);
      expect(tableProblem(b, sheet, null, headers: true), TableProblem.range);
      expect(tableProblem(b, sheet, const CellArea(1, 1, maxRows, 3), headers: true), TableProblem.range);
    });
  });

  group('table edits', () {
    test('typing under the last row makes the table one row longer in the same edit', () {
      final t = tree();
      final edit = growTableEdit(t['S1']!, 4, 6)!;
      expect(t.apply(edit), isNotNull);
      expect(SheetTable.of(t['S1']!).single.area, const CellArea(1, 6, 4, 7));
      expect(growTableEdit(t['S1']!, 5, 6), isNotNull);
      expect(growTableEdit(tree()['S1']!, 5, 6), isNull);
      expect(growTableEdit(tree()['S1']!, 4, 8), isNull);
    });

    test('the row under a table with a total row is not taken', () {
      final t = tree(
        tables: [
          {..._existing, 'totals': true},
        ],
      );
      expect(growTableEdit(t['S1']!, 4, 6), isNull);
    });

    test('the total row is turned on and off', () {
      final t = tree();
      final table = SheetTable.of(t['S1']!).single;
      final on = totalsEdits(t['S1']!, table, true).single;
      expect(on.changes.length, 1);
      expect(on.changes.single.attributes['tables'], [
        {..._existing, 'totals': true},
      ]);
      expect(t.apply(on), isNotNull);
      final off = totalsEdits(t['S1']!, SheetTable.of(t['S1']!).single, false).single;
      expect(off.changes.single.attributes['tables'], [_existing]);
    });

    test('a row is inserted under a table first when the total row would cover something', () {
      final t = tree(
        cells: const [
          Cell(4, 7, {'v': 9}),
        ],
      );
      final table = SheetTable.of(t['S1']!).single;
      final edits = totalsEdits(t['S1']!, table, true);
      expect(edits.length, 2);
      expect(edits.first.changes.single.dim, dimRows);
      expect(edits.first.changes.single.at, 4);
      expect(edits.last.changes.single.attributes['tables'], [
        {..._existing, 'totals': true},
      ]);
    });

    test('converting to a range takes the table out of the list', () {
      final t = tree(
        tables: [
          _existing,
          {..._existing, 'id': 2, 'name': 'Tableau4', 'ref': 'J1:K3'},
        ],
      );
      final edit = changeTable(t['S1']!, SheetTable.of(t['S1']!).first, null);
      expect(edit.changes.single.attributes['tables'], [
        {..._existing, 'id': 2, 'name': 'Tableau4', 'ref': 'J1:K3'},
      ]);
      expect(changeTable(t['S1']!, SheetTable.of(t['S1']!).last, null).changes.single.attributes['tables'], [_existing]);
      final alone = tree();
      expect(changeTable(alone['S1']!, SheetTable.of(alone['S1']!).single, null).changes.single.attributes.containsKey('tables'), isTrue);
      expect(changeTable(alone['S1']!, SheetTable.of(alone['S1']!).single, null).changes.single.attributes['tables'], isNull);
    });
  });

  group('table styles', () {
    final theme = Workbook(tree()).themeColors;

    test('the header takes the accent color of its family and the bands a tint of it', () {
      final medium2 = tableLook('TableStyleMedium2', theme)!;
      expect(medium2.header, theme[4]);
      expect(medium2.headerText, theme[0]);
      expect(medium2.band, tinted(theme[4], 0.8));
      expect(tableLook('TableStyleMedium9', theme)!.header, theme[4]);
      expect(tableLook('TableStyleMedium4', theme)!.header, theme[6]);
      expect(tableLook('TableStyleMedium1', theme)!.header, theme[1]);
      expect(tableLook('TableStyleDark1', theme)!.header, theme[1]);
      expect(tableLook('TableStyleCustom', theme), isNull);
      expect(tableLook(null, theme), isNull);
    });

    test('a cell takes the look of its place in the table, its own format first', () {
      final t = tree(
        tables: [
          {..._existing, 'rows': true, 'cols': true, 'first': true, 'totals': true, 'ref': 'F1:G5'},
        ],
      );
      final b = book(t);
      final tables = b.tables(t['S1']!);
      final base = b.style(null);
      final look = tableLook('TableStyleMedium2', b.themeColors)!;
      final header = tables.style(base, 1, 6);
      expect(header.fill, look.header);
      expect(header.font.bold, isTrue);
      expect(header.font.color, look.headerText);
      expect(tables.style(base, 2, 6).fill, look.band);
      expect(tables.style(base, 2, 6).font.bold, isTrue);
      expect(tables.style(base, 2, 7).fill, look.band);
      expect(tables.style(base, 3, 7).fill, isNull);
      expect(tables.style(base, 3, 6).fill, look.band);
      expect(tables.style(base, 5, 7).top?.style, 'double');
      expect(tables.style(base, 5, 7).font.bold, isTrue);
      expect(tables.style(base, 6, 6).fill, isNull);
      expect(tables.style(base, 6, 6), base);
      final own = base.copyWith(fill: const Color(0xFF00FF00));
      expect(tables.style(own, 1, 6).fill, const Color(0xFF00FF00));
    });
  });

  group('in the editor', () {
    late FakeHub hub;
    late DocSession session;

    Future<void> settle(WidgetTester tester) async {
      await tester.runAsync(hub.settle);
      await tester.pump();
    }

    Future<void> open(WidgetTester tester) async {
      tester.view.physicalSize = const Size(1400, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      hub = FakeHub.tree(
        _workbook(
          tables: [_existing.map((k, v) => MapEntry(k, k == 'ref' ? 'A1:B3' : v))],
          cells: const [
            Cell(1, 1, {'v': 'Nom'}),
            Cell(1, 2, {'v': 'Prix'}),
            Cell(2, 1, {'v': 'a'}),
          ],
        ),
      );
      session = DocSession(hub.connect)..start();
      addTearDown(session.dispose);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SpreadsheetEditor(session: session, title: 'classeur.xlsx'),
          ),
        ),
      );
      await settle(tester);
    }

    Future<void> finish(WidgetTester tester) async {
      await tester.pumpWidget(const SizedBox());
      await tester.pump(const Duration(milliseconds: 100));
    }

    Future<void> type(WidgetTester tester, String text) async {
      await tester.sendKeyEvent(LogicalKeyboardKey.f2);
      await tester.pump();
      await tester.enterText(find.byType(EditableText).last, text);
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await settle(tester);
    }

    Future<void> rightClick(WidgetTester tester) async {
      final mouse = await tester.startGesture(tester.getTopLeft(find.byType(SheetView)) + const Offset(50, 36), kind: PointerDeviceKind.mouse, buttons: kSecondaryMouseButton);
      await mouse.up();
      await tester.pumpAndSettle();
    }

    List<SheetTable> tables() => SheetTable.of(hub.doc['S1']!);

    testWidgets('a value typed under the last row joins the table in the same edit', (tester) async {
      await open(tester);
      for (var i = 0; i < 3; i++) {
        await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      }
      await tester.pump();
      await type(tester, 'b');
      expect(hub.doc['S1']!.grid!.cell(4, 1)?['v'], 'b');
      expect(tables().single.area, const CellArea(1, 1, 4, 2));
      await finish(tester);
    });

    testWidgets('a value typed elsewhere leaves the table as it is', (tester) async {
      await open(tester);
      for (var i = 0; i < 4; i++) {
        await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      }
      await tester.pump();
      await type(tester, 'b');
      expect(tables().single.area, const CellArea(1, 1, 3, 2));
      await finish(tester);
    });

    testWidgets('Insertion > Tableau makes a table of the selection and refuses an overlap', (tester) async {
      await open(tester);
      await tester.tap(find.text('Insertion'));
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Tableau').last);
      await tester.pumpAndSettle();
      expect(find.text('Créer un tableau'), findsOneWidget);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      expect(find.text('Cette plage chevauche un autre tableau.'), findsOneWidget);
      await tester.enterText(find.byType(TextField).last, 'D1:E3');
      await tester.tap(find.text('OK'));
      await settle(tester);
      await tester.pumpAndSettle();
      expect(find.text('Créer un tableau'), findsNothing);
      expect(tables().map((t) => (t.name, t.area)), [('Tableau1', const CellArea(1, 1, 3, 2)), ('Tableau3', const CellArea(1, 4, 3, 5))]);
      await finish(tester);
    });

    testWidgets('the menu of a cell of a table turns the total row on and converts to a range', (tester) async {
      await open(tester);
      await rightClick(tester);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Ligne Total'), warnIfMissed: false);
      await settle(tester);
      expect(tables().single.totals, isTrue);
      await rightClick(tester);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Convertir en plage'));
      await settle(tester);
      expect(tables(), isEmpty);
      await finish(tester);
    });
  });
}
