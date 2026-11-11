import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chart/chart_edits.dart';
import 'package:loffice/src/chrome/ribbon.dart';
import 'package:loffice/src/excel/chart_edits.dart';
import 'package:loffice/src/excel/charts.dart';
import 'package:loffice/src/excel/sheet_view.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/testing.dart';

const _key = 'xl/charts/chart1.xml';

Map<String, Object?> placed() => {
  'id': _key,
  'from': {'col': 4, 'row': 1, 'dx': 0, 'dy': 0},
  'to': {'col': 10, 'row': 16, 'dx': 0, 'dy': 0},
  'chart': {
    'space': {'fill': 'FFFFFF'},
    'title': {'text': 'Ventes', 'overlay': false},
    'legend': {'pos': 'r', 'overlay': false},
    'plots': [
      {
        'kind': 'bar',
        'dir': 'col',
        'grouping': 'clustered',
        'gap': 80,
        '3d': true,
        'axes': [1, 2],
        'series': [
          {
            'index': 0,
            'order': 0,
            'name': {
              'ref': r'Feuil1!$B$1',
              'count': 1,
              'str': ['Prix'],
            },
            'cat': {
              'ref': r'Feuil1!$A$2:$A$3',
              'count': 2,
              'str': ['a', 'b'],
            },
            'val': {
              'ref': r'Feuil1!$B$2:$B$3',
              'count': 2,
              'num': [2, 3],
            },
            'smooth': true,
          },
        ],
      },
    ],
    'axes': [
      {'id': 1, 'kind': 'cat', 'pos': 'b', 'tick': 'out'},
      {
        'id': 2,
        'kind': 'val',
        'pos': 'l',
        'title': {'text': 'Euros'},
      },
    ],
  },
};

Edit workbook(List<Map<String, Object?>> charts) => Edit([
  Change.create(const Node(id: 'book', type: 'book', key: 'V')),
  Change.create(
    Node(
      id: 'S1',
      type: 'sheet',
      parent: 'book',
      key: 'K',
      attributes: {'name': 'Feuil1', if (charts.isNotEmpty) 'charts': charts},
      grid: Grid([
        const Cell(1, 2, {'v': 'Prix'}),
        const Cell(2, 1, {'v': 'a'}),
        const Cell(2, 2, {'v': 2}),
        const Cell(3, 1, {'v': 'b'}),
        const Cell(3, 2, {'v': 3}),
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

  Future<void> open(WidgetTester tester, {bool chart = true}) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(workbook([if (chart) placed()]));
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

  List<Map<String, Object?>> charts() => [...?(hub.doc['S1']!.attributes['charts'] as List<Object?>?)?.cast<Map<String, Object?>>()];

  Map<String, Object?> chart() => charts().single['chart']! as Map<String, Object?>;

  Map<String, Object?> plot() => (chart()['plots']! as List<Object?>).first! as Map<String, Object?>;

  List<Object?> axes() => chart()['axes']! as List<Object?>;

  Rect chartRect() {
    final sheet = hub.doc['S1']!;
    return SheetChart.of(sheet).single.rect(Workbook(session.document).layout(sheet));
  }

  /// Where the chart is on the screen.
  Rect onScreen(WidgetTester tester) {
    final view = tester.state<SheetViewState>(find.byType(SheetView));
    return view.screenRect(chartRect()).shift(tester.getTopLeft(find.byType(SheetView)));
  }

  Future<void> select(WidgetTester tester) async {
    await tester.tapAt(onScreen(tester).center);
    await tester.pump();
  }

  Future<void> tab(WidgetTester tester, String name) async {
    final label = find.text(name);
    await tester.scrollUntilVisible(
      label,
      100,
      scrollable: find.descendant(of: find.byType(Ribbon), matching: find.byType(Scrollable)).first,
    );
    await tester.tap(label.first);
    await tester.pumpAndSettle();
  }

  Future<void> command(WidgetTester tester, String name) async {
    await tester.tap(find.text(name).last);
    await tester.pumpAndSettle();
  }

  Future<void> menu(WidgetTester tester, String name, String item) async {
    await tester.tap(find.byTooltip(name).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text(item).last, warnIfMissed: false);
    await settle(tester);
    await tester.pumpAndSettle();
  }

  Future<void> confirm(WidgetTester tester) async {
    await tester.tap(find.text('OK'));
    await settle(tester);
    await tester.pumpAndSettle();
  }

  Future<void> drag(WidgetTester tester, Offset from, Offset by) async {
    final gesture = await tester.startGesture(from, kind: PointerDeviceKind.mouse);
    await gesture.moveBy(by ~/ 2);
    await gesture.moveBy(by - by ~/ 2);
    await gesture.up();
    await settle(tester);
  }

  testWidgets('inserts a chart from the selection, drawn before the Go package gives it its series', (tester) async {
    await open(tester, chart: false);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    await tab(tester, 'Insertion');
    await menu(tester, 'Graphique', 'Courbes');

    final added = charts().single;
    expect(added.containsKey('id'), isFalse);
    expect(added['source'], r"'Feuil1'!$A$1:$B$3");
    expect(added['from'], {'col': 2, 'row': 0, 'dx': 0, 'dy': 0});
    expect(added['to'], {'col': 10, 'row': 15, 'dx': 0, 'dy': 0});
    expect(plot()['kind'], 'line');
    expect(plot()['grouping'], 'standard');
    expect(plot()['series'], isEmpty);
    expect(chart()['legend'], {'pos': 'r'});
    expect(chart()['title'], isEmpty);
    expect(find.text('Création de graphique'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await finish(tester);
  });

  test('every type of the menu is a plot', () {
    final sheet = Workbook(Tree.fromEdit(workbook([]))!).sheets.single;
    final layout = Workbook(Tree.fromEdit(workbook([]))!).layout(sheet);
    final kinds = {for (final t in ChartType.values) t: (ChartEdits(sheet, layout).add(const CellArea(1, 1, 2, 2), 'Feuil1', t).single['chart']! as Map)['plots'] as List};
    expect(
      {for (final e in kinds.entries) e.key: ((e.value.single as Map)['kind'], (e.value.single as Map)['dir'])},
      {
        ChartType.column: ('bar', 'col'),
        ChartType.bar: ('bar', 'bar'),
        ChartType.line: ('line', null),
        ChartType.pie: ('pie', null),
        ChartType.scatter: ('scatter', null),
        ChartType.area: ('area', null),
      },
    );
  });

  testWidgets('selects a chart and changes its type, keeping what it does not touch', (tester) async {
    await open(tester);
    expect(find.text('Création de graphique'), findsNothing);
    await select(tester);
    await tab(tester, 'Création de graphique');
    await menu(tester, 'Modifier le type de graphique', 'Barres groupées');
    expect(plot()['kind'], 'bar');
    expect(plot()['dir'], 'bar');
    expect(plot().containsKey('3d'), isFalse);
    expect(plot()['gap'], 80);

    await menu(tester, 'Modifier le type de graphique', 'Aires');
    expect(plot()['kind'], 'area');
    expect(plot().containsKey('dir'), isFalse);
    expect(plot()['grouping'], 'standard');

    expect(charts().single['id'], _key);
    expect(charts().single['from'], placed()['from']);
    expect(chart()['space'], {'fill': 'FFFFFF'});
    final original = ((placed()['chart']! as Map<String, Object?>)['plots']! as List<Object?>).first! as Map<String, Object?>;
    expect(plot()['series'], original['series']);
    await finish(tester);
  });

  testWidgets('sets the title, the legend and the titles of the axes', (tester) async {
    await open(tester);
    await select(tester);
    await tab(tester, 'Création de graphique');

    await command(tester, 'Titre du graphique');
    await tester.enterText(find.widgetWithText(TextField, 'Texte (vide : titre automatique)'), 'Chiffre d’affaires');
    await confirm(tester);
    expect(chart()['title'], {'text': 'Chiffre d’affaires', 'overlay': false});

    await command(tester, 'Titre du graphique');
    await tester.tap(find.byType(Checkbox));
    await tester.pump();
    await confirm(tester);
    expect(chart().containsKey('title'), isFalse);

    await menu(tester, 'Légende', 'Bas');
    expect(chart()['legend'], {'pos': 'b', 'overlay': false});
    await menu(tester, 'Légende', 'Aucune');
    expect(chart().containsKey('legend'), isFalse);
    await menu(tester, 'Légende', 'Gauche');
    expect(chart()['legend'], {'pos': 'l'});

    await command(tester, 'Titres des axes');
    expect(find.widgetWithText(TextField, 'Axe des valeurs'), findsOneWidget);
    expect(tester.widget<TextField>(find.widgetWithText(TextField, 'Axe des valeurs')).controller!.text, 'Euros');
    await tester.enterText(find.widgetWithText(TextField, 'Axe des catégories'), 'Mois');
    await tester.enterText(find.widgetWithText(TextField, 'Axe des valeurs'), '');
    await confirm(tester);
    expect(axes()[0], {
      'id': 1,
      'kind': 'cat',
      'pos': 'b',
      'tick': 'out',
      'title': {'text': 'Mois'},
    });
    expect(axes()[1], {'id': 2, 'kind': 'val', 'pos': 'l'});
    expect(charts().single['id'], _key);
    await finish(tester);
  });

  testWidgets('changes the ranges of the series, adds one and removes one', (tester) async {
    await open(tester);
    await select(tester);
    await tab(tester, 'Création de graphique');

    await command(tester, 'Sélectionner les données');
    await tester.enterText(find.widgetWithText(TextField, 'Valeurs de la série'), r'=Feuil1!$B$2:$B$4');
    await tester.enterText(find.widgetWithText(TextField, 'Étiquettes des catégories'), r'Feuil1!$A$2:$A$4');
    await tester.tap(find.text('Ajouter une série'));
    await tester.pumpAndSettle();
    await tester.enterText(find.widgetWithText(TextField, 'Valeurs de la série').last, r'Feuil1!$C$2:$C$4');
    await confirm(tester);

    final series = (plot()['series']! as List).cast<Map<String, Object?>>();
    expect(series, hasLength(2));
    expect(series[0], {
      'index': 0,
      'order': 0,
      'name': {
        'ref': r'Feuil1!$B$1',
        'count': 1,
        'str': ['Prix'],
      },
      'cat': {
        'ref': r'Feuil1!$A$2:$A$4',
        'count': 2,
        'str': ['a', 'b'],
      },
      'val': {
        'ref': r'Feuil1!$B$2:$B$4',
        'count': 2,
        'num': [2, 3],
      },
      'smooth': true,
    });
    expect(series[1], {
      'index': 1,
      'val': {'ref': r'Feuil1!$C$2:$C$4'},
      'cat': {'ref': r'Feuil1!$A$2:$A$4'},
    });

    await command(tester, 'Sélectionner les données');
    await tester.tap(find.byTooltip('Supprimer la série').first);
    await tester.pumpAndSettle();
    await confirm(tester);
    expect((plot()['series']! as List).single, series[1]);
    expect(charts().single['id'], _key);
    await finish(tester);
  });

  testWidgets('moves and resizes a chart with the mouse, one edit each', (tester) async {
    await open(tester);
    await select(tester);
    final before = chartRect();
    final edits = hub.history.length;

    await drag(tester, onScreen(tester).center, const Offset(64, 40));
    expect(chartRect(), before.shift(const Offset(64, 40)));
    expect(hub.history.length, edits + 1);
    expect(charts().single['id'], _key);
    expect(chart()['space'], {'fill': 'FFFFFF'});

    await drag(tester, onScreen(tester).bottomRight, const Offset(30, 20));
    expect(chartRect(), Rect.fromLTRB(before.left + 64, before.top + 40, before.right + 94, before.bottom + 60));
    expect(hub.history.length, edits + 2);

    await drag(tester, onScreen(tester).topLeft, const Offset(-1000, -1000));
    expect(chartRect().topLeft, Offset.zero);
    await finish(tester);
  });

  testWidgets('a click on a chart changes nothing; a cell lets it go', (tester) async {
    await open(tester);
    final edits = hub.history.length;
    await select(tester);
    expect(find.text('Création de graphique'), findsOneWidget);
    await tester.tapAt(onScreen(tester).center);
    await tester.pump();
    expect(hub.history.length, edits);

    await tester.tapAt(tester.getTopLeft(find.byType(SheetView)) + const Offset(100, 100));
    await tester.pump();
    expect(find.text('Création de graphique'), findsNothing);

    await select(tester);
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pump();
    expect(find.text('Création de graphique'), findsNothing);
    await finish(tester);
  });

  testWidgets('resizes in centimeters and deletes the chart', (tester) async {
    await open(tester);
    await select(tester);
    await tab(tester, 'Format');
    final before = chartRect();
    final height = find.byTooltip('Hauteur').last;
    await tester.enterText(find.descendant(of: height, matching: find.byType(TextField)), '5');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await settle(tester);
    expect(chartRect().height, closeTo(5 * 96 / 2.54, 1));
    expect(chartRect().topLeft, before.topLeft);
    expect(chartRect().width, before.width);

    await command(tester, 'Supprimer le graphique');
    await settle(tester);
    expect(hub.doc['S1']!.attributes['charts'], isNull);
    expect(find.text('Création de graphique'), findsNothing);
    await finish(tester);
  });

  testWidgets('Delete removes the chart selected', (tester) async {
    await open(tester);
    await select(tester);
    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(hub.doc['S1']!.attributes['charts'], isNull);
    await finish(tester);
  });

  test('a rectangle becomes an anchor and back', () {
    final book = Workbook(Tree.fromEdit(workbook([placed()]))!);
    final sheet = book.sheets.single;
    final l = book.layout(sheet);
    expect(chartAnchor(l, Offset(l.x(3) + 10, l.y(4) + 2.5)), {'col': 2, 'row': 3, 'dx': 10 * 9525, 'dy': (2.5 * 9525).round()});
    expect(chartAnchor(Workbook(Tree.fromEdit(workbook([]))!).layout(sheet), Offset.zero), {'col': 0, 'row': 0, 'dx': 0, 'dy': 0});

    final rect = Rect.fromLTRB(l.x(2) + 7, l.y(3) + 5, l.x(9) + 3, l.y(20));
    final placedAgain = ChartEdits(sheet, l).place(0, rect).single;
    expect(placedAgain['from'], {'col': 1, 'row': 2, 'dx': 7 * 9525, 'dy': 5 * 9525});
    expect(placedAgain['to'], {'col': 8, 'row': 19, 'dx': 3 * 9525, 'dy': 0});
    expect(placedAgain['id'], _key);

    final sized = {'x': 0, 'y': 0, 'w': 9525, 'h': 9525, 'chart': <String, Object?>{}};
    final withSize = Workbook(Tree.fromEdit(workbook([sized]))!);
    final moved = ChartEdits(withSize.sheets.single, l).place(0, const Rect.fromLTWH(10, 20, 300, 200)).single;
    expect([moved['x'], moved['y'], moved['w'], moved['h']], [10 * 9525, 20 * 9525, 300 * 9525, 200 * 9525]);
    expect(moved.containsKey('from'), isFalse);
  });

  test('a series added takes an index the plot has not', () {
    final book = Workbook(Tree.fromEdit(workbook([placed()]))!);
    final sheet = book.sheets.single;
    final edited = ChartEdits(sheet, book.layout(sheet)).setSeries(0, const [SeriesInput(null, '', r'Feuil1!$C$2:$C$3'), SeriesInput(0, r'Feuil1!$B$1', r'Feuil1!$B$2:$B$3')], r'Feuil1!$A$2:$A$3');
    final series = (((edited.single['chart']! as Map)['plots'] as List).single as Map)['series'] as List;
    expect([for (final s in series) (s as Map)['index']], [1, 0]);
    expect(((series.first as Map)['val'] as Map)['ref'], r'Feuil1!$C$2:$C$3');
  });
}
