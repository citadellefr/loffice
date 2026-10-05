import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/ribbon.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_canvas.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:trame/testing.dart';

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> open(WidgetTester tester, String fixture, {Size size = const Size(1400, 900)}) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/pptx/$fixture.json').readAsStringSync()))!);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: PresentationEditor(session: session, media: (_) async => Uint8List(0), title: '$fixture.pptx')),
    ));
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  List<Node> slides() => [for (final n in hub.doc.children('deck')) if (n.type == 'slide') n];

  testWidgets('shows the ribbon, the slides and the status', (tester) async {
    await open(tester, 'shp-shapes');
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Insertion'), findsOneWidget);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('fits a phone', (tester) async {
    await open(tester, 'shp-shapes', size: const Size(390, 844));
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('adds, duplicates and deletes slides', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.byTooltip('Dupliquer la diapositive'));
    await settle(tester);
    expect(slides(), hasLength(3));
    expect(find.text('Diapositive 2 sur 3'), findsOneWidget);
    expect(hub.doc.children(slides()[1].id).length, hub.doc.children(slides()[0].id).where((n) => n.type != 'notes').length);

    await tester.tap(find.byTooltip('Supprimer la diapositive'));
    await settle(tester);
    expect(slides(), hasLength(2));

    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyM);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(slides(), hasLength(3));
    expect(slides()[1].attributes['layout'], 'L1');
    await finish(tester);
  });

  testWidgets('a new slide has the placeholders of its layout', (tester) async {
    await open(tester, 'ph-populated-placeholders');
    await tester.tap(find.byTooltip('Nouvelle diapositive').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('3_Custom Layout').last);
    await settle(tester);
    final fresh = slides()[1];
    expect(fresh.attributes['layout'], 'L5');
    final placeholders = [for (final n in hub.doc.children(fresh.id)) n.attributes['ph']];
    expect(placeholders, [{'idx': '10', 'sz': 'quarter', 'type': 'tbl'}]);
    await finish(tester);
  });

  testWidgets('moves a picture with the mouse, then resizes it by a corner', (tester) async {
    await open(tester, 'shp-shapes');
    final deck = Deck(hub.doc);
    final painter = SlidePainter(deck);
    final rect = tester.getRect(find.byType(SlideCanvas));
    final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
    final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
    final pic = hub.doc['s256-6']!;
    final at = origin + painter.transformOf(pic).apply(painter.sizeOf(pic)!.center(Offset.zero)) * scale;
    Map<String, Object?> xfrm() => hub.doc['s256-6']!.attributes['xfrm']! as Map<String, Object?>;

    var drag = await tester.startGesture(at);
    await drag.moveTo(at - const Offset(30, 20) * scale);
    await drag.up();
    await settle(tester);
    expect(xfrm()['x'], 7164288 - 30 * emuPerPoint);
    expect(xfrm()['y'], 5949280 - 20 * emuPerPoint);

    final corner = origin + painter.transformOf(hub.doc['s256-6']!).apply(painter.sizeOf(pic)!.bottomRight(Offset.zero)) * scale;
    drag = await tester.startGesture(corner);
    await drag.moveTo(corner + const Offset(10, 10) * scale);
    await drag.up();
    await settle(tester);
    expect(xfrm()['w'], closeTo(1778000 + 10 * emuPerPoint, 1));
    expect(xfrm()['h'], closeTo(711200 + 10 * emuPerPoint, 1));
    await finish(tester);
  });

  testWidgets('a second click goes into a group, whose shape then moves in it, the group around it', (tester) async {
    await open(tester, 'shp-shapes');
    final rect = tester.getRect(find.byType(SlideCanvas));
    await tester.tapAt(rect.topLeft + const Offset(4, 4));
    await tester.sendKeyEvent(LogicalKeyboardKey.pageDown);
    await tester.pump();
    expect(find.text('Diapositive 2 sur 2'), findsOneWidget);
    final deck = Deck(hub.doc);
    final painter = SlidePainter(deck);
    final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
    final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
    final child = hub.doc['s257-3']!;
    final group = Map.of(hub.doc['s257-5']!.attributes['xfrm']! as Map<String, Object?>);
    final at = origin + painter.transformOf(child).apply(painter.sizeOf(child)!.center(Offset.zero)) * scale;
    Future<void> pause() => tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 400)));

    await tester.tapAt(at);
    await tester.pump();
    await pause();
    await tester.tapAt(at);
    await tester.pump();
    await pause();
    final drag = await tester.startGesture(at);
    await drag.moveTo(at + const Offset(20, 0) * scale);
    await drag.up();
    await settle(tester);
    final x = hub.doc['s257-3']!.attributes['xfrm']! as Map<String, Object?>;
    expect(x['x'], 2051720 + 20 * emuPerPoint);
    // it was on the right edge of the group, which widens with it
    expect(hub.doc['s257-5']!.attributes['xfrm'], {...group, 'w': 1105272 + 254000, 'cw': 1105272 + 254000});
    await finish(tester);
  });

  testWidgets('types into the cells of a table, from one to the next', (tester) async {
    await open(tester, 'tbl-cell');
    final slide = slides()[0];
    final frame = hub.doc.children(slide.id).firstWhere((n) => n.attributes['frame'] == 'table');
    final deck = Deck(hub.doc);
    final painter = SlidePainter(deck);
    final table = painter.tableOf(frame)!;
    final row = hub.doc.children(frame.id)[1];
    final cells = hub.doc.children(row.id);
    final rect = tester.getRect(find.byType(SlideCanvas));
    final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
    final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
    final center = painter.transformOf(frame).apply(table.cells[cells[1].id]!.rect.center);

    await tester.tapAt(origin + center * scale);
    await tester.pump();
    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'Bonjour', selection: TextSelection.collapsed(offset: 7)));
    await settle(tester);
    expect(hub.doc[cells[1].id]!.text!.text, 'Bonjour\n');

    await tester.sendKeyEvent(LogicalKeyboardKey.tab);
    await tester.pump();
    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'Salut', selection: TextSelection.collapsed(offset: 5)));
    await settle(tester);
    expect(hub.doc[cells[2].id]!.text!.text, 'Salut\n');

    // Escape takes the table, which Delete deletes
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(hub.doc[frame.id], isNull);
    await finish(tester);
  });

  testWidgets('merges the cells dragged over, splits them, drags an edge', (tester) async {
    await open(tester, 'tbl-cell');
    final frame = hub.doc.children(slides()[0].id).firstWhere((n) => n.attributes['frame'] == 'table');
    final deck = Deck(hub.doc);
    final painter = SlidePainter(deck);
    final table = painter.tableOf(frame)!;
    final rect = tester.getRect(find.byType(SlideCanvas));
    final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
    final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
    Offset at(Offset local) => origin + painter.transformOf(frame).apply(local) * scale;
    Node cell(int r, int c) => hub.doc.children(hub.doc.children(frame.id)[r].id)[c];
    final from = cell(1, 1), to = cell(2, 2);

    final drag = await tester.startGesture(at(table.cells[from.id]!.rect.center));
    await drag.moveTo(at(table.cells[to.id]!.rect.center));
    await drag.up();
    await tester.pump();
    await tester.tap(find.text('Disposition'));
    await tester.pump();
    await tester.tap(find.byTooltip('Fusionner les cellules'));
    await settle(tester);
    expect(cell(1, 1).attributes['gridSpan'], 2);
    expect(cell(1, 1).attributes['rowSpan'], 2);
    expect(cell(2, 2).attributes['vMerge'], isTrue);
    expect(cell(2, 2).attributes['hMerge'], isTrue);

    await tester.tap(find.byTooltip('Fractionner les cellules'));
    await settle(tester);
    expect(cell(1, 1).attributes['gridSpan'], isNull);
    expect(cell(2, 2).attributes['vMerge'], isNull);

    final edge = Offset(table.columns[1], table.rows[1] / 2);
    final pull = await tester.startGesture(at(edge));
    await pull.moveTo(at(edge + const Offset(20, 0)));
    await pull.up();
    await settle(tester);
    final grid = hub.doc[frame.id]!.attributes['grid']! as List<Object?>;
    expect(grid[0], closeTo(1524000 + 20 * emuPerPoint, 1));
    expect(grid[1], closeTo(1524000 - 20 * emuPerPoint, 1));

    // a cell split in three columns, then typed 3 cm high
    await tester.tapAt(at(painter.tableOf(hub.doc[frame.id]!)!.cells[cell(3, 0).id]!.rect.center));
    await tester.pump();
    await tester.tap(find.byTooltip('Fractionner les cellules'));
    await tester.pumpAndSettle();
    await tester.enterText(find.widgetWithText(TextField, 'Nombre de colonnes'), '3');
    await tester.tap(find.text('OK'));
    await settle(tester);
    expect(hub.doc[frame.id]!.attributes['grid'], hasLength(6));
    expect(cell(0, 0).attributes['gridSpan'], 3);
    await tester.enterText(find.descendant(of: find.byTooltip('Hauteur'), matching: find.byType(TextField)), '3');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await settle(tester);
    expect(hub.doc.children(frame.id)[3].attributes['h'], closeTo(1080000, 2));
    await finish(tester);
  });

  testWidgets('a table selected is shaded and bordered through its cells', (tester) async {
    await open(tester, 'tbl-cell');
    final frame = hub.doc.children(slides()[0].id).firstWhere((n) => n.attributes['frame'] == 'table');
    final deck = Deck(hub.doc);
    final painter = SlidePainter(deck);
    final rect = tester.getRect(find.byType(SlideCanvas));
    final scale = math.min((rect.width - 48) / deck.size.width, (rect.height - 48) / deck.size.height);
    final origin = rect.topLeft + Offset((rect.width - deck.size.width * scale) / 2, (rect.height - deck.size.height * scale) / 2);
    // the edge of the table takes the table
    await tester.tapAt(origin + painter.transformOf(frame).apply(const Offset(1, 1)) * scale);
    await tester.pump();
    await tester.tap(find.text('Création de tableau'));
    await tester.pump();
    await tester.tap(find.byTooltip('Trame de fond').first);
    await tester.pumpAndSettle();
    await tester.tap(find.descendant(of: find.byType(ColorPalette), matching: find.byType(InkWell)).first);
    await settle(tester);
    final cells = [for (final r in hub.doc.children(frame.id)) ...hub.doc.children(r.id)];
    expect(cells.every((c) => (c.attributes['fill']! as Map<String, Object?>)['solid'] is Map), isTrue);

    await tester.tap(find.byTooltip('Bordures').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Bordures extérieures'));
    await settle(tester);
    final first = hub.doc.children(hub.doc.children(frame.id).first.id).first;
    expect((first.attributes['lnT']! as Map<String, Object?>)['w'], 12700);
    expect(first.attributes['lnR'], isNull);

    await tester.tap(find.text('Disposition'));
    await tester.pump();
    await tester.tap(find.byTooltip('Orientation du texte').first);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(CheckedPopupMenuItem<String>, 'Rotation de 270° de tout le texte'));
    await settle(tester);
    await tester.tap(find.byTooltip('Aligner en bas').first);
    await settle(tester);
    final all = [for (final r in hub.doc.children(frame.id)) ...hub.doc.children(r.id)];
    expect(all.where((c) => c.attributes['hMerge'] != true && c.attributes['vMerge'] != true).every((c) => c.attributes['vert'] == 'vert270' && c.attributes['anchor'] == 'b'), isTrue);
    await finish(tester);
  });

  testWidgets('inserts a table, then rows from the ribbon and with Tab', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.text('Insertion'));
    await tester.pump();
    await tester.tap(find.byTooltip('Tableau').first);
    await tester.pumpAndSettle();
    // three columns, two rows
    await tester.tap(find.descendant(of: find.byType(TableSizeGrid), matching: find.byType(GestureDetector)).at(12));
    await settle(tester);
    final frame = hub.doc.children(slides()[0].id).last;
    expect(frame.attributes['frame'], 'table');
    List<Node> rows() => hub.doc.children(frame.id);
    expect(rows(), hasLength(2));
    expect(hub.doc.children(rows()[0].id), hasLength(3));

    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'Nom', selection: TextSelection.collapsed(offset: 3)));
    await settle(tester);
    expect(hub.doc.children(rows()[0].id).first.text!.text, 'Nom\n');

    await tester.tap(find.text('Disposition'));
    await tester.pump();
    await tester.tap(find.byTooltip('Insérer en dessous'));
    await settle(tester);
    expect(rows(), hasLength(3));
    await tester.tap(find.text('Création de tableau'));
    await tester.pump();
    await tester.tap(find.text('Ligne d’en-tête'));
    await settle(tester);
    expect((hub.doc[frame.id]!.attributes['tbl']! as Map<String, Object?>)['firstRow'], isNull);
    await tester.tap(find.byTooltip('Styles').first);
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Style clair 1 - Accentuation 2'));
    await settle(tester);
    expect(hub.doc[frame.id]!.attributes['tbl'], {'style': '{0E3FDE45-AF77-4B5C-9715-49D594BDF05E}', 'bandRow': true});

    // Tab from the first cell to the last, and once more: a fourth row
    for (var i = 0; i < 9; i++) {
      await tester.sendKeyEvent(LogicalKeyboardKey.tab);
      await tester.pump();
    }
    await settle(tester);
    expect(rows(), hasLength(4));
    await finish(tester);
  });

  testWidgets('a transition picked, its options and duration, then for all slides', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.text('Transitions'));
    await tester.pump();
    await tester.tap(find.byTooltip('Pousser'));
    await settle(tester);
    Object? transition(int i) => slides()[i].attributes['transition'];
    expect(transition(0), {'dur': 1000, 'effect': 'push', 'dir': 'u'});

    await tester.tap(find.byTooltip('Options d’effet').first);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(CheckedPopupMenuItem<Map<String, Object?>>, 'De la droite'));
    await settle(tester);
    await tester.enterText(find.descendant(of: find.byTooltip('Durée'), matching: find.byType(TextField)), '2');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await settle(tester);
    expect(transition(0), {'dur': 2000, 'effect': 'push', 'dir': 'l'});

    await tester.tap(find.byTooltip('Appliquer partout'));
    await settle(tester);
    expect(transition(1), transition(0));
    await tester.tap(find.byTooltip('Aucune'));
    await settle(tester);
    expect(transition(0), isNull);
    await finish(tester);
  });

  testWidgets('inserts a shape and types into it', (tester) async {
    await open(tester, 'shp-shapes');
    await tester.tap(find.text('Insertion'));
    await tester.pump();
    await tester.tap(find.byTooltip('Zone de texte'));
    await settle(tester);
    final box = hub.doc.children(slides()[0].id).last;
    expect(box.attributes['name'], startsWith('ZoneTexte'));

    // typing goes into the new text box through the text input
    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'Bonjour', selection: TextSelection.collapsed(offset: 7)));
    await settle(tester);
    expect(hub.doc[box.id]!.text!.text, 'Bonjour\n');

    // bold on the whole word, then Enter and more text
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyA);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyG);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    expect(hub.doc[box.id]!.text!.ops.first.attributes?['b'], '1');
    await tester.sendKeyEvent(LogicalKeyboardKey.end);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(hub.doc[box.id]!.text!.text, 'Bonjour\n\n');

    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(hub.doc[box.id], isNull);
    await finish(tester);
  });
}
