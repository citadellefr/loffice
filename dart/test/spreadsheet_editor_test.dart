import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/excel/sheet_view.dart';
import 'package:trame/testing.dart';

Edit workbook({bool csv = false}) => Edit([
  Change.create(Node(id: 'book', type: 'book', key: 'V', attributes: {if (csv) 'csv': true})),
  Change.create(
    Node(
      id: 'S1',
      type: 'sheet',
      parent: 'book',
      key: 'K',
      attributes: const {
        'name': 'Feuil1',
        'valid': [
          {'ref': 'D1:D5', 'type': 'list', 'f1': '"Oui,Non"', 'err': 'stop', 'prompt': 'Oui ou non'},
          {'ref': 'E1', 'type': 'list', 'f1': r'Feuil1!$A$1:$A$3', 'err': 'warning'},
          {'ref': 'F1:F3', 'type': 'whole', 'op': 'between', 'f1': '1', 'f2': '10', 'err': 'stop'},
          {'ref': 'G1', 'type': 'textLength', 'op': 'lessThanOrEqual', 'f1': '3', 'err': 'stop', 'errText': 'Trop long'},
        ],
      },
      grid: Grid([
        const Cell(1, 1, {'v': 2}),
        const Cell(2, 1, {'v': 3}),
        const Cell(3, 1, {'f': 'SUM(A1:A2)', 'v': 5}),
        if (csv) const Cell(1, 2, {'src': '2,0', 'v': 2}),
      ]),
    ),
  ),
  Change.create(
    const Node(
      id: 'x0',
      type: 'xf',
      key: 'V',
      attributes: {
        'style': {
          'fmt': 'General',
          'font': {'name': 'Calibri', 'sz': 11},
        },
      },
    ),
  ),
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

  Future<void> typeIn(WidgetTester tester, String text) async {
    await tester.sendKeyEvent(LogicalKeyboardKey.f2);
    await tester.pump();
    await tester.enterText(find.byType(EditableText).last, text);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
  }

  Future<void> goRight(WidgetTester tester, int n) async {
    for (var i = 0; i < n; i++) {
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    }
    await tester.pump();
  }

  testWidgets('refuses numbers and texts the validation does not take', (tester) async {
    await open(tester);
    await goRight(tester, 5);
    await typeIn(tester, '11');
    expect(find.text('Valeur attendue : nombre entier entre 1 et 10.'), findsOneWidget);
    await tester.tap(find.text('Réessayer'));
    await tester.pumpAndSettle();
    expect(cell(1, 6), isNull);
    await tester.enterText(find.byType(EditableText).last, '2,5');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(find.text('Valeur attendue : nombre entier entre 1 et 10.'), findsOneWidget);
    await tester.tap(find.text('Annuler'));
    await tester.pumpAndSettle();
    await typeIn(tester, '10');
    await settle(tester);
    expect(cell(1, 6)?['v'], 10);

    await goRight(tester, 1);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowUp);
    await tester.pump();
    await typeIn(tester, 'abcd');
    expect(find.text('Trop long'), findsOneWidget);
    await tester.tap(find.text('Annuler'));
    await tester.pumpAndSettle();
    expect(cell(1, 7), isNull);
    await typeIn(tester, 'abc');
    await settle(tester);
    expect(cell(1, 7)?['v'], 'abc');
    await finish(tester);
  });

  group('conditional formats', () {
    Future<void> choose(WidgetTester tester, List<String> path) async {
      await tester.tap(find.byTooltip('Mise en forme conditionnelle'));
      await tester.pumpAndSettle();
      for (final label in path) {
        await tester.tap(find.text(label).last);
        await tester.pumpAndSettle();
      }
      await settle(tester);
    }

    Future<void> selectDown(WidgetTester tester, int rows) async {
      await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
      for (var i = 0; i < rows; i++) {
        await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      }
      await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
      await tester.pump();
    }

    List<Map<String, Object?>> rules() => [for (final r in hub.doc['S1']!.attributes['cf'] as List<Object?>? ?? const []) r! as Map<String, Object?>];

    Finder inDialog(Finder f) => find.descendant(of: find.byType(AlertDialog), matching: f);

    testWidgets('highlight the cells above a value, in French', (tester) async {
      await open(tester);
      await selectDown(tester, 2);
      await choose(tester, ['Règles de mise en surbrillance des cellules', 'Supérieur à…']);
      await tester.enterText(inDialog(find.byType(TextField)), '2,5');
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules(), hasLength(1));
      expect(rules().single['ref'], 'A1:A3');
      expect(rules().single['type'], 'formula');
      expect(rules().single['f'], ['A1>(2.5)']);
      expect(rules().single['pri'], 1);
      expect(((rules().single['style']! as Map)['fill']! as Map)['fg'], {'rgb': 'FFC7CE'});
      await finish(tester);
    });

    testWidgets('highlight cells between values, with a text and in doubles', (tester) async {
      await open(tester);
      await choose(tester, ['Règles de mise en surbrillance des cellules', 'Entre…']);
      await tester.enterText(inDialog(find.byType(TextField)).first, '1');
      await tester.enterText(inDialog(find.byType(TextField)).last, '=SOMME(B1;2)');
      await tester.tap(inDialog(find.byType(DropdownButton<String>)));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Remplissage vert et texte vert foncé').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules().single['f'], ['AND(A1>=(1),A1<=(SUM(B1,2)))']);
      expect(((rules().single['style']! as Map)['font']! as Map)['color'], {'rgb': '006100'});

      await choose(tester, ['Règles de mise en surbrillance des cellules', 'Texte qui contient…']);
      await tester.enterText(inDialog(find.byType(TextField)), 'a"b');
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules().first['f'], ['NOT(ISERROR(SEARCH("a""b",A1)))']);

      await choose(tester, ['Règles de mise en surbrillance des cellules', 'Valeurs en double…']);
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules().first['type'], 'duplicateValues');
      expect(rules().map((r) => r['pri']), [1, 2, 3]);
      await finish(tester);
    });

    testWidgets('rank the highest values and those above the average', (tester) async {
      await open(tester);
      await selectDown(tester, 2);
      await choose(tester, ['Règles des valeurs plus/moins élevées', '10 % derniers…']);
      await tester.enterText(inDialog(find.byType(TextField)), '20');
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules().single, containsPair('type', 'top10'));
      expect(rules().single, containsPair('rank', 20));
      expect(rules().single, containsPair('percent', true));
      expect(rules().single, containsPair('bottom', true));

      await choose(tester, ['Règles des valeurs plus/moins élevées', 'En dessous de la moyenne…']);
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(rules().first, containsPair('type', 'aboveAverage'));
      expect(rules().first, containsPair('below', true));
      await finish(tester);
    });

    testWidgets('add data bars, color scales and icon sets', (tester) async {
      await open(tester);
      await selectDown(tester, 1);
      await choose(tester, ['Barres de données', 'Vert']);
      await choose(tester, ['Nuances de couleurs', 'Bleu - Blanc - Rouge']);
      await choose(tester, ['Jeux d’icônes', '5 notes']);
      expect(rules().map((r) => r['type']), ['iconSet', 'colorScale', 'dataBar']);
      expect(rules()[0]['icons'], '5Rating');
      expect((rules()[0]['stops']! as List).map((s) => (s! as Map)['v']), ['0', '20', '40', '60', '80']);
      expect(rules()[1]['colors']! as List, hasLength(3));
      expect(rules()[2]['ref'], 'A1:A2');
      await finish(tester);
    });

    testWidgets('clear the rules of the selection, or of the sheet', (tester) async {
      await open(tester);
      await choose(tester, ['Barres de données', 'Bleu']);
      await goRight(tester, 1);
      await choose(tester, ['Barres de données', 'Rouge']);
      await choose(tester, ['Effacer les règles', 'Effacer les règles des cellules sélectionnées']);
      expect(rules().map((r) => r['ref']), ['A1']);
      await choose(tester, ['Barres de données', 'Rouge']);
      await choose(tester, ['Effacer les règles', 'Effacer les règles de la feuille entière']);
      expect(hub.doc['S1']!.attributes['cf'], isNull);
      await finish(tester);
    });

    testWidgets('manage the rules: move, delete and add one by a formula', (tester) async {
      await open(tester);
      await choose(tester, ['Barres de données', 'Bleu']);
      await choose(tester, ['Jeux d’icônes', '3 étoiles']);
      await choose(tester, ['Gérer les règles…']);
      expect(find.text('Jeu d’icônes : 3 étoiles'), findsOneWidget);
      expect(find.text('Barre de données'), findsOneWidget);
      await tester.tap(inDialog(find.byTooltip('Monter')).last);
      await tester.pump();
      await tester.tap(inDialog(find.byTooltip('Supprimer la règle')).first);
      await tester.pump();
      await tester.tap(find.text('Nouvelle règle…'));
      await tester.pumpAndSettle();
      await tester.enterText(find.widgetWithText(TextField, 'S’applique à'), 'B2:C3');
      await tester.enterText(find.widgetWithText(TextField, 'Formule (relative à la première cellule)'), '=ET(B2>1;B2<5)');
      await tester.tap(find.text('OK').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('OK').last);
      await settle(tester);
      expect(rules(), hasLength(2));
      expect(rules()[0]['f'], ['AND(B2>1,B2<5)']);
      expect(rules()[0]['ref'], 'B2:C3');
      expect(rules()[1]['type'], 'iconSet');
      expect(rules().map((r) => r['pri']), [1, 2]);
      await finish(tester);
    });
  });

  group('data validation', () {
    List<Map<String, Object?>> valid() => [for (final r in hub.doc['S1']!.attributes['valid'] as List<Object?>? ?? const []) r! as Map<String, Object?>];

    Future<void> openDialog(WidgetTester tester) async {
      await tester.tap(find.text('Données').first);
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Validation des données'));
      await tester.pumpAndSettle();
    }

    Finder field(String label) => find.widgetWithText(TextField, label);

    testWidgets('allow whole numbers between bounds', (tester) async {
      await open(tester);
      await openDialog(tester);
      await tester.tap(find.text('Tout'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Nombre entier').last);
      await tester.pumpAndSettle();
      await tester.enterText(field('Minimum'), '1');
      await tester.enterText(field('Maximum'), '9');
      await tester.tap(find.text('Message de saisie'));
      await tester.pumpAndSettle();
      await tester.enterText(field('Titre'), 'Note');
      await tester.enterText(field('Message'), 'De 1 à 9');
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(valid().last, {'ref': 'A1', 'type': 'whole', 'f1': '1', 'f2': '9', 'blank': true, 'err': 'stop', 'promptTitle': 'Note', 'prompt': 'De 1 à 9'});
      expect(valid(), hasLength(5));
      expect(find.byWidgetPredicate((w) => w is Text && w.data == 'De 1 à 9'), findsOneWidget);

      await typeIn(tester, '10');
      expect(find.text('Valeur attendue : nombre entier entre 1 et 9.'), findsOneWidget);
      await finish(tester);
    });

    testWidgets('allow a list of values, a date or a formula', (tester) async {
      await open(tester);
      await goRight(tester, 7);
      await openDialog(tester);
      await tester.tap(find.text('Tout'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Liste').last);
      await tester.pumpAndSettle();
      await tester.enterText(field('Source (valeurs séparées par des points-virgules, ou =plage)'), 'rouge;vert');
      await tester.tap(find.text('Liste déroulante dans la cellule'));
      await tester.pump();
      await tester.tap(find.text('Alerte d’erreur'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Arrêt'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Avertissement').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(valid().last, {'ref': 'H1', 'type': 'list', 'f1': '"rouge,vert"', 'blank': true, 'noArrow': true, 'err': 'warning'});

      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await openDialog(tester);
      await tester.tap(find.text('Tout'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Date').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('entre'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('supérieur ou égal à').last);
      await tester.pumpAndSettle();
      await tester.enterText(field('Valeur'), '01/01/2024');
      await tester.tap(find.text('OK'));
      await settle(tester);
      expect(valid().last, {'ref': 'H2', 'type': 'date', 'op': 'greaterThanOrEqual', 'f1': '45292', 'blank': true, 'err': 'stop'});
      await typeIn(tester, '31/12/2023');
      expect(find.textContaining('Valeur attendue : date supérieur ou égal à 01/01/2024.'), findsOneWidget);
      await finish(tester);
    });

    testWidgets('start from the validation of the cell and clear it', (tester) async {
      await open(tester);
      await goRight(tester, 5);
      await openDialog(tester);
      expect(find.text('Nombre entier'), findsOneWidget);
      expect(tester.widget<TextField>(field('Minimum')).controller!.text, '1');
      expect(tester.widget<TextField>(field('Maximum')).controller!.text, '10');
      await tester.tap(find.text('Effacer tout'));
      await settle(tester);
      expect(valid().map((r) => r['ref']), ['D1:D5', 'E1', 'F2:F3', 'G1']);
      await finish(tester);
    });
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
        {
          'col': 0,
          'vals': ['3'],
        },
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

  testWidgets('the page layout tab sets how the sheet prints', (tester) async {
    await open(tester);
    Object? setup() => hub.doc['S1']!.attributes['print'];
    await tester.tap(find.text('Mise en page').first);
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Orientation').first);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(CheckedPopupMenuItem<bool>, 'Paysage'));
    await tester.pumpAndSettle();
    await settle(tester);
    expect(setup(), {'land': true});

    await tester.tap(find.byTooltip('Marges').first);
    await tester.pumpAndSettle();
    await tester.tap(find.textContaining('Étroites'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Imprimer le quadrillage'));
    await settle(tester);
    expect(setup(), {
      'land': true,
      'margins': {'l': 0.25, 'r': 0.25, 't': 0.75, 'b': 0.75, 'head': 0.3, 'foot': 0.3},
      'grid': true,
    });

    // a break over the third row, then the selection as what prints
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.tap(find.byTooltip('Sauts de page').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Insérer un saut de page'));
    await tester.pumpAndSettle();
    await settle(tester);
    expect((setup()! as Map)['rows'], [3]);
    expect((setup()! as Map)['cols'], isNull);
    await tester.tap(find.byTooltip('Zone d’impression').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Définir'));
    await tester.pumpAndSettle();
    await settle(tester);
    expect(hub.doc['book']!.attributes['names'], [
      {'name': '_xlnm.Print_Area', 'ref': r"'Feuil1'!$A$3:$A$3", 'sheet': 'S1'},
    ]);
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
