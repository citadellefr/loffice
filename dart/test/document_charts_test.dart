import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chart/chart_edits.dart';
import 'package:loffice/src/chrome/ribbon.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_canvas.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/word/blocks.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/layout.dart';
import 'package:loffice/src/word/page_painter.dart';
import 'package:loffice/src/word/pages_view.dart';
import 'package:loffice/src/word/paragraph.dart';
import 'package:trame/testing.dart';

/// What the Go package reads of the chart of a slide: its chart and the
/// table of its data.
Map<String, Object?> _frame() {
  final changes = jsonDecode(File('../testdata/pptx/cht-chart-type.json').readAsStringSync()) as List<Object?>;
  return changes.cast<Map<String, Object?>>().firstWhere((c) => (c['a'] as Map<String, Object?>?)?['frame'] == 'chart')['a']! as Map<String, Object?>;
}

List<Object?> _series(Object? chart) => ((chart! as Map<String, Object?>)['plots']! as List<Object?>).cast<Map<String, Object?>>().first['series']! as List<Object?>;

Edit _deck({required bool data}) {
  final changes = jsonDecode(File('../testdata/pptx/cht-chart-type.json').readAsStringSync()) as List<Object?>;
  if (!data) {
    for (final c in changes.cast<Map<String, Object?>>()) {
      (c['a'] as Map<String, Object?>?)?.remove('chartData');
    }
  }
  return Edit.fromJson(changes)!;
}

/// The paragraph "no specified style" with a chart before it, as the Go
/// package reads one.
Edit _document({required bool data}) {
  final changes = jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()) as List<Object?>;
  final frame = _frame();
  final text = changes.cast<Map<String, Object?>>().firstWhere((c) => c['id'] == '1');
  (text['x']! as List<Object?>).insert(0, {
    'i': '￼',
    'a': {
      'img': jsonEncode({'w': 4572000, 'h': 2743200, 'chart': frame['chart'], 'chartNode': 'chart-1'}),
    },
  });
  changes.add({
    'o': 'new',
    'id': 'chart-1',
    't': 'chart',
    'p': 'doc',
    'k': 'g',
    'a': {'chart': frame['chart'], if (data) 'chartData': frame['chartData']},
  });
  return Edit.fromJson(changes)!;
}

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> start(WidgetTester tester, Edit edit, Widget Function(DocSession session) editor) async {
    tester.view.physicalSize = const Size(1800, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(edit);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: editor(session))));
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
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

  Future<void> typeCell(WidgetTester tester, String key, String text) async {
    await tester.enterText(find.byKey(ValueKey(key)), text);
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await settle(tester);
  }

  final original = _frame();

  /// The chart of a document and its tabs: how the editor of each shows it.
  final editors = {
    'PowerPoint': (
      open: (WidgetTester tester, {required bool data}) => start(tester, _deck(data: data), (s) => PresentationEditor(session: s, media: (_) async => Uint8List(0))),
      node: () => hub.doc.children(hub.doc.children('deck').firstWhere((n) => n.type == 'slide').id).firstWhere((n) => n.attributes['frame'] == 'chart'),
      select: (WidgetTester tester) async {
        final frame = hub.doc.children(hub.doc.children('deck').firstWhere((n) => n.type == 'slide').id).firstWhere((n) => n.attributes['frame'] == 'chart');
        final deck = Deck(hub.doc);
        final rect = tester.getRect(find.byType(SlideCanvas));
        final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
        final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
        await tester.tapAt(origin + SlidePainter(deck).transformOf(frame).apply(const Offset(40, 40)) * scale);
        await tester.pump();
      },
      leave: (WidgetTester tester) async {
        await tester.tapAt(tester.getRect(find.byType(SlideCanvas)).bottomRight - const Offset(2, 2));
        await tester.pump();
      },
    ),
    'Word': (
      open: (WidgetTester tester, {required bool data}) => start(tester, _document(data: data), (s) => WordEditor(session: s, media: (_) async => Uint8List(0))),
      node: () => hub.doc['chart-1']!,
      select: (WidgetTester tester) async {
        await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
        await tester.sendKeyEvent(LogicalKeyboardKey.home);
        await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
        await tester.pump();
      },
      leave: (WidgetTester tester) async {
        await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
        await tester.sendKeyEvent(LogicalKeyboardKey.end);
        await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
        await tester.pump();
      },
    ),
  };

  for (final MapEntry(key: name, value: editor) in editors.entries) {
    group(name, () {
      Future<void> select(WidgetTester tester, {bool data = true}) async {
        await editor.open(tester, data: data);
        if (name == 'Word') await tester.tap(find.byType(WordPagesView));
        await editor.select(tester);
      }

      Map<String, Object?> chart() => editor.node().attributes['chart']! as Map<String, Object?>;

      Map<String, Object?> plot() => (chart()['plots']! as List<Object?>).first! as Map<String, Object?>;

      Map<String, Object?> table() => editor.node().attributes['chartData']! as Map<String, Object?>;

      testWidgets('shows the tabs of a chart selected, and takes them away when it is not', (tester) async {
        await select(tester);
        expect(find.text('Création de graphique'), findsOneWidget);
        expect(find.text('Format'), findsOneWidget);
        await editor.leave(tester);
        expect(find.text('Création de graphique'), findsNothing);
        await finish(tester);
      });

      testWidgets('changes the type, the title and the legend, the data of the series staying as they are', (tester) async {
        await select(tester);
        await tab(tester, 'Création de graphique');
        await menu(tester, 'Modifier le type de graphique', 'Barres groupées');
        expect(plot()['kind'], 'bar');
        expect(plot()['dir'], 'bar');
        expect(plot()['grouping'], 'clustered');
        expect(plot()['axes'], ((original['chart']! as Map<String, Object?>)['plots']! as List<Object?>).cast<Map<String, Object?>>().first['axes']);

        await tab(tester, 'Format');
        await command(tester, 'Titre du graphique');
        await tester.tap(find.byType(Checkbox));
        await tester.pump();
        await tester.enterText(find.widgetWithText(TextField, 'Texte (vide : titre automatique)'), 'Chiffre d’affaires');
        await tester.tap(find.text('OK'));
        await settle(tester);
        await tester.pumpAndSettle();
        expect(chart()['title'], {'text': 'Chiffre d’affaires'});

        await menu(tester, 'Légende', 'Bas');
        expect(chart()['legend'], {'pos': 'b'});
        await menu(tester, 'Légende', 'Aucune');
        expect(chart().containsKey('legend'), isFalse);

        expect(_series(chart()), _series(original['chart']));
        expect(editor.node().attributes['chartData'], original['chartData']);
        await finish(tester);
      });

      testWidgets('edits the data in a pane: a cell, a row, a series with an index the chart has not', (tester) async {
        await select(tester);
        await tab(tester, 'Création de graphique');
        await command(tester, 'Sélectionner les données');

        await typeCell(tester, 'cell0.1', '40,5');
        await typeCell(tester, 'cell1.2', 'n/a');
        await typeCell(tester, 'cell1.3', '');
        await typeCell(tester, 'name0', 'Ventes');
        var series = (table()['series']! as List<Object?>).cast<Map<String, Object?>>();
        expect(series[0]['values'], [32, 40.5, 28, 12, 15]);
        expect(series[1]['values'], [12, 12, 'n/a', null, 28]);
        expect(series[0]['name'], 'Ventes');
        expect(table()['categories'], (original['chartData']! as Map<String, Object?>)['categories']);

        await tester.tap(find.text('Ajouter une série'));
        await settle(tester);
        series = (table()['series']! as List<Object?>).cast<Map<String, Object?>>();
        expect(series.map((s) => s['index']), [0, 1, 2]);
        expect(series[2]['values'], List.filled(5, null));
        expect(series[2].containsKey('name'), isFalse);

        await tester.ensureVisible(find.byTooltip('Supprimer la série').first);
        await tester.tap(find.byTooltip('Supprimer la série').first);
        await settle(tester);
        await tester.ensureVisible(find.byTooltip('Supprimer la ligne').first);
        await tester.tap(find.byTooltip('Supprimer la ligne').first);
        await settle(tester);
        await tester.tap(find.text('Ajouter une ligne'));
        await settle(tester);
        series = (table()['series']! as List<Object?>).cast<Map<String, Object?>>();
        expect(series.map((s) => s['index']), [1, 2]);
        expect(series[0]['values'], [12, 'n/a', null, 28, null]);
        expect((table()['categories']! as List<Object?>).length, 5);
        expect((table()['categories']! as List<Object?>).last, isNull);
        expect(chart(), original['chart']);
        await finish(tester);
      });

      testWidgets('leaves the data of a chart without an embedded workbook read-only', (tester) async {
        await select(tester, data: false);
        await tab(tester, 'Création de graphique');
        await command(tester, 'Sélectionner les données');
        expect(find.text('Les données de ce graphique sont en lecture seule : il n’a pas de classeur incorporé.'), findsOneWidget);
        expect(find.byType(TextField), findsNothing);
        await finish(tester);
      });
    });
  }

  testWidgets('Word selects a chart by a tap on its drawing and shows its tabs', (tester) async {
    await start(tester, _document(data: true), (s) => WordEditor(session: s, media: (_) async => Uint8List(0)));
    await tester.tap(find.byType(WordPagesView));
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.end);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await tester.pump();
    expect(find.text('Création de graphique'), findsNothing);
    final view = tester.widget<WordPagesView>(find.byType(WordPagesView));
    final page = find.byType(CustomPaint).evaluate().map((e) => e.renderObject! as RenderBox).firstWhere((b) => b.size.width < 1000 && b.size.height > 1000);
    final (_, rect, _) = view.layout.pictureAt('1', 0)!;
    await tester.tapAt(page.localToGlobal(rect.center * view.scale));
    await tester.pump();
    expect((view.selection.start, view.selection.end), (0, 1));
    expect(find.text('Création de graphique'), findsOneWidget);
    await finish(tester);
  });

  test('Word draws the chart of its node, not the one the drawing read', () async {
    final tree = Tree.fromEdit(_document(data: true))!;
    final doc = WordDocument(tree);
    final context = WordContext(doc);
    final layout = WordLayout(doc, context, ParaCache());
    final painter = PagePainter(context: context);
    Future<List<int>> draw() async {
      final recorder = ui.PictureRecorder();
      painter.paint(ui.Canvas(recorder), layout.pages.first);
      final image = await recorder.endRecording().toImage(612, 792);
      return (await image.toByteData())!.buffer.asUint8List();
    }

    final before = await draw();
    expect(await draw(), before);
    expect(tree.apply(ChartChanges.edit(tree['chart-1']!, (chart) => ChartChanges.retype(chart, ChartType.pie))), isNotNull);
    expect(await draw(), isNot(before));
  });
}
