import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/excel/sheet_view.dart';

import 'fakes.dart';

Edit workbook({bool csv = false}) => Edit([
  Change.create(Node(id: 'book', type: 'book', key: 'V', attributes: {if (csv) 'csv': true})),
  Change.create(Node(
    id: 'S1',
    type: 'sheet',
    parent: 'book',
    key: 'K',
    attributes: const {
      'name': 'Feuil1',
      'lists': [
        {'ref': 'D1:D5', 'src': '"Oui,Non"', 'err': 'stop', 'prompt': 'Oui ou non'},
        {'ref': 'E1', 'src': r'Feuil1!$A$1:$A$3', 'err': 'warning'},
      ],
    },
    grid: Grid([
      const Cell(1, 1, {'v': 2}),
      const Cell(2, 1, {'v': 3}),
      const Cell(3, 1, {'f': 'SUM(A1:A2)', 'v': 5}),
      if (csv) const Cell(1, 2, {'src': '2,0', 'v': 2}),
    ]),
  )),
  Change.create(const Node(id: 'x0', type: 'xf', key: 'V', attributes: {
    'style': {'fmt': 'General', 'font': {'name': 'Calibri', 'sz': 11}},
  })),
]);

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> open(WidgetTester tester, {bool csv = false, Size size = const Size(1400, 900)}) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(workbook(csv: csv));
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: SpreadsheetEditor(session: session, title: 'classeur.xlsx'))));
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  Map<String, Object?>? cell(int row, int col) => hub.doc['S1']!.grid!.cell(row, col);

  testWidgets('shows the ribbon, the sheet tabs and the formula of a cell', (tester) async {
    await open(tester);
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Formules'), findsOneWidget);
    expect(find.text('Feuil1'), findsOneWidget);
    expect(find.text('A1'), findsOneWidget);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.pump();
    expect(find.text('A3'), findsOneWidget);
    expect(find.text('=SOMME(A1:A2)'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('typing sets values and French formulas', (tester) async {
    await open(tester);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.f2);
    await tester.pump();
    await tester.enterText(find.byType(EditableText).last, '1 234,5');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(cell(1, 2)?['v'], 1234.5);

    await tester.sendKeyEvent(LogicalKeyboardKey.f2);
    await tester.pump();
    await tester.enterText(find.byType(EditableText).last, '=SOMME(A1:A3;B1)');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(cell(2, 2)?['f'], 'SUM(A1:A3,B1)');
    await finish(tester);
  });

  testWidgets('formats cells, inserts rows and adds sheets', (tester) async {
    await open(tester);
    await tester.tap(find.byTooltip('Gras (Ctrl+G)'));
    await settle(tester);
    final style = hub.doc[cell(1, 1)!['s']! as String]!.attributes['style']! as Map<String, Object?>;
    expect((style['font']! as Map)['b'], true);
    expect(style['base'], 'x0');

    await tester.tap(find.byTooltip('Insérer').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Insérer des lignes dans la feuille').last);
    await settle(tester);
    expect(cell(1, 1), isNull);
    expect(cell(2, 1)?['v'], 2);

    await tester.tap(find.byTooltip('Insérer une feuille').last);
    await settle(tester);
    expect(find.text('Feuil2'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('offers the values of lists and refuses the others', (tester) async {
    await open(tester);
    for (var i = 0; i < 3; i++) {
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    }
    await tester.pump();
    expect(find.text('Oui ou non'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.arrow_drop_down).last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Non').last);
    await settle(tester);
    expect(cell(1, 4)?['v'], 'Non');

    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.f2);
    await tester.pump();
    await tester.enterText(find.byType(EditableText).last, 'Peut-être');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(find.textContaining('ne correspond pas'), findsOneWidget);
    await tester.tap(find.text('Réessayer'));
    await tester.pumpAndSettle();
    expect(cell(2, 4), isNull);
    await tester.enterText(find.byType(EditableText).last, 'oui');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(cell(2, 4)?['v'], 'oui');

    // the values of cells, and a warning that lets a value in
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowUp);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowUp);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.altLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.altLeft);
    await tester.pumpAndSettle();
    expect(find.text('5'), findsOneWidget);
    await tester.tapAt(Offset.zero);
    await tester.pumpAndSettle();
    await tester.sendKeyEvent(LogicalKeyboardKey.f2);
    await tester.pump();
    await tester.enterText(find.byType(EditableText).last, '7');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(find.textContaining('Continuer ?'), findsOneWidget);
    await tester.tap(find.text('Oui'));
    await settle(tester);
    expect(cell(1, 5)?['v'], 7);
    await finish(tester);
  });

  testWidgets('filters rows and takes the filter away', (tester) async {
    await open(tester);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyL);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(hub.doc['S1']!.attributes['filter'], {'ref': 'A1:A3'});

    await tester.tap(find.byIcon(Icons.arrow_drop_down).last);
    await tester.pumpAndSettle();
    expect(find.text('(Sélectionner tout)'), findsOneWidget);
    await tester.tap(find.widgetWithText(CheckboxListTile, '5'));
    await tester.pump();
    await tester.tap(find.text('OK'));
    await settle(tester);
    expect(hub.doc['S1']!.attributes['filter'], {
      'ref': 'A1:A3',
      'cols': [
        {'col': 0, 'vals': ['3']},
      ],
    });
    expect(hub.doc['S1']!.grid!.cell(3, 0), {'hide': true});
    expect(find.byIcon(Icons.filter_alt), findsOneWidget);

    // a filtered list is summed without the rows it hides
    for (var i = 0; i < 4 && find.text('A4').evaluate().isEmpty; i++) {
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await tester.pump();
    }
    await tester.tap(find.byTooltip('Somme automatique (Alt+=)').first);
    await settle(tester);
    expect(cell(4, 1)?['f'], 'SUBTOTAL(9,A1:A3)');

    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyL);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(hub.doc['S1']!.attributes['filter'], isNull);
    expect(hub.doc['S1']!.grid!.cell(3, 0), isNull);
    await finish(tester);
  });

  testWidgets('zooms with two fingers', (tester) async {
    await open(tester);
    expect(find.text('100 %'), findsOneWidget);
    final center = tester.getCenter(find.byType(SheetView));
    final a = await tester.startGesture(center - const Offset(50, 0), kind: PointerDeviceKind.touch);
    final b = await tester.startGesture(center + const Offset(50, 0), kind: PointerDeviceKind.touch);
    await a.moveBy(const Offset(-50, 0));
    await b.moveBy(const Offset(50, 0));
    await a.up();
    await b.up();
    await tester.pump();
    expect(find.text('200 %'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('fits a phone, its tabs in a list', (tester) async {
    await open(tester, size: const Size(390, 844));
    await tester.tap(find.text('Accueil'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Données').last);
    await tester.pumpAndSettle();
    expect(find.byTooltip('Filtrer (Ctrl+Maj+L)'), findsOneWidget);
    await tester.tap(find.byIcon(Icons.expand_less));
    await tester.pumpAndSettle();
    expect(find.byTooltip('Filtrer (Ctrl+Maj+L)'), findsNothing);
    await finish(tester);
  });

  testWidgets('keeps a CSV file to one sheet, and says what it saves', (tester) async {
    await open(tester, csv: true);
    expect(find.textContaining('Fichier CSV'), findsOneWidget);
    expect(find.byIcon(Icons.add_circle_outline), findsNothing);

    // a copy leaves behind the text the cell was read from
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'Clipboard.setData') copied = (call.arguments as Map)['text'] as String?;
      return call.method == 'Clipboard.getData' ? {'text': copied} : null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyC);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyV);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 10)));
    await settle(tester);
    expect(cell(1, 3), {'v': 2});
    await finish(tester);
  });
}
