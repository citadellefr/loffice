import 'dart:ui' as ui;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/ribbon.dart';
import 'package:loffice/src/excel/cell_painter.dart';
import 'package:loffice/src/excel/edits.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/sheet_comments.dart';
import 'package:loffice/src/excel/sheet_view.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/testing.dart';

const _note = {'author': 'Alice', 'text': 'Vérifier\nce total', 'shape': 3};
const _first = {'id': '{00000000-0000-0000-0000-000000000001}', 'author': 'Alice', 'date': '2026-11-03T10:00:00Z', 'text': 'Pourquoi ?'};
const _answer = {'id': '{00000000-0000-0000-0000-000000000002}', 'author': 'Bob', 'date': '2026-11-03T11:30:00Z', 'text': 'Parce que'};

Edit workbook() => Edit([
  Change.create(const Node(id: 'book', type: 'book', key: 'V')),
  Change.create(
    Node(
      id: 'S1',
      type: 'sheet',
      parent: 'book',
      key: 'K',
      attributes: const {'name': 'Feuil1'},
      grid: Grid([
        const Cell(1, 1, {'v': 2}),
        const Cell(2, 2, {'v': 7, 'note': _note}),
        const Cell(3, 3, {
          'thread': [_first, _answer],
        }),
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

  Future<void> open(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(workbook());
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

  Offset at(WidgetTester tester, int row, int col) {
    final view = tester.state<SheetViewState>(find.byType(SheetView));
    return tester.getTopLeft(find.byType(SheetView)) + Offset(view.screenX(col) + 20, view.screenY(row) + 8);
  }

  Future<void> select(WidgetTester tester, int row, int col) async {
    await tester.tapAt(at(tester, row, col));
    await tester.pump();
  }

  Future<void> review(WidgetTester tester, String command) async {
    await tester.tap(find.text('Révision'));
    await tester.pump();
    await tester.tap(find.text(command));
    await tester.pump();
  }

  VoidCallback? pressable(WidgetTester tester, String label) => tester.widget<RibbonButton>(find.widgetWithText(RibbonButton, label)).onPressed;

  Future<void> choose(WidgetTester tester, Finder menu, String item) async {
    await tester.tap(menu);
    await tester.pumpAndSettle();
    await tester.tap(find.text(item).last);
    await settle(tester);
  }

  List<Object?> thread() => cell(3, 3)!['thread']! as List<Object?>;

  group('model', () {
    final sheet = Tree.fromEdit(workbook())!['S1']!;

    test('reads notes and threads row by row', () {
      final threads = sheetThreads(sheet);
      expect([for (final t in threads) (t.row, t.col, t.note)], [(2, 2, true), (3, 3, false)]);
      expect(threads[0].root.author, 'Alice');
      expect(threads[0].root.text, 'Vérifier\nce total');
      expect(threads[0].replies, isEmpty);
      expect(threads[1].id, _first['id']);
      expect(threads[1].replies.single.author, 'Bob');
      expect(threads[1].replies.single.date?.toUtc(), DateTime.utc(2026, 11, 3, 11, 30));
      expect(threads[1].done, false);
      expect(threadAt(sheet.grid!.cell(1, 1), 1, 1), isNull);
    });

    test('a resolved thread is done', () {
      final t = threadAt(
        {
          'thread': [
            {..._first, 'done': true},
          ],
        },
        1,
        1,
      )!;
      expect(t.done, true);
    });

    test('edits send the whole field of the cell and keep what is not theirs', () {
      final edits = CommentEdits(sheet, 'Moi');
      final t = sheetThreads(sheet)[1];
      Map<String, Object?> sent(Edit e) => e.changes.single.cells!.single.fields;
      final reply = sent(edits.reply(t, 'Merci', DateTime.utc(2026, 11, 4, 8, 5, 9)))['thread']! as List<Object?>;
      expect(reply.sublist(0, 2), [_first, _answer]);
      expect(reply[2], {'id': matches(RegExp(r'^\{[0-9A-F]{8}(-[0-9A-F]{4}){3}-[0-9A-F]{12}\}$')), 'author': 'Moi', 'date': '2026-11-04T08:05:09Z', 'text': 'Merci'});
      expect(sent(edits.resolve(t, true)), {
        'thread': [
          {..._first, 'done': true},
          _answer,
        ],
      });
      expect(sent(edits.say(t, t.replies.single, 'Autre')), {
        'thread': [
          _first,
          {..._answer, 'text': 'Autre'},
        ],
      });
      expect(sent(edits.delete(t, t.replies.single)), {
        'thread': [_first],
      });
      expect(sent(edits.delete(t, t.root)), {'note': null, 'thread': null});
      final n = sheetThreads(sheet)[0];
      expect(sent(edits.say(n, n.root, 'Neuf')), {
        'note': {..._note, 'text': 'Neuf'},
      });
      expect(sent(edits.startNote(5, 5, 'Hop')), {
        'note': {'author': 'Moi', 'text': 'Hop'},
      });
    });

    test('a note is edited by anyone, a thread comment by its author', () {
      final threads = sheetThreads(sheet);
      expect(threads[0].editableBy(threads[0].root, 'Moi'), true);
      expect(threads[1].editableBy(threads[1].root, 'Moi'), false);
      expect(threads[1].editableBy(threads[1].root, 'Alice'), true);
      expect(threads[0].resolvable || threads[0].repliable, false);
    });

    test('copies, sorts and clears leave notes and threads where they are', () {
      final tree = Tree.fromEdit(workbook())!;
      final edits = SheetEdits(Workbook(tree));
      final s = tree['S1']!;
      final clip = edits.copy(s, const CellArea(1, 1, 3, 3));
      expect(clip.cells.where((c) => hasComments(c.fields)), isEmpty);
      final pasted = edits.paste(s, clip, 10, 10).changes.single.cells!;
      expect(pasted.where((c) => hasComments(c.fields)), isEmpty);
      expect(pasted.where((c) => c.fields.containsKey('note') || c.fields.containsKey('thread')), isEmpty);
      final sorted = edits.sort(s, const CellArea(1, 2, 3, 2), 2).changes.single.cells!;
      expect(sorted.where((c) => c.fields.containsKey('note')), isEmpty);
      expect(edits.clear(s, const CellArea(1, 1, 3, 3)).changes.single.cells!.every((c) => !c.fields.containsKey('note') && !c.fields.containsKey('thread')), true);
      final all = CommentEdits(s, 'Moi').clear(1, 1, 3, 3).changes.single.cells!;
      expect([for (final c in all) (c.row, c.col)], [(2, 2), (3, 3)]);
      expect(all.every((c) => c.fields['note'] == null && c.fields['thread'] == null), true);
    });
  });

  group('indicator', () {
    Future<List<int>> pixel(WidgetTester tester, bool markers, int x, int y) async {
      final tree = Tree.fromEdit(workbook())!;
      final book = Workbook(tree);
      final sheet = tree['S1']!;
      final l = book.layout(sheet);
      final recorder = ui.PictureRecorder();
      CellPainter(book: book, sheet: sheet, locale: NumberLocale.fr, x: l.x, y: l.y, markers: markers).paint(Canvas(recorder), const CellArea(1, 1, 4, 4));
      return tester
          .runAsync(() async {
            final image = await recorder.endRecording().toImage(300, 120);
            final data = (await image.toByteData())!;
            final i = (y * 300 + x) * 4;
            return [data.getUint8(i), data.getUint8(i + 1), data.getUint8(i + 2)];
          })
          .then((v) => v!);
    }

    testWidgets('a note is marked in red, a thread in purple, a cell without in neither', (tester) async {
      // B2 spans x 64..128, y 20..40; C3 x 128..192, y 40..60
      expect(await pixel(tester, true, 126, 22), [0xFF, 0x00, 0x00]);
      expect(await pixel(tester, true, 190, 42), [0x70, 0x30, 0xA0]);
      expect(await pixel(tester, true, 62, 2), isNot([0xFF, 0x00, 0x00]));
      expect(await pixel(tester, true, 66, 38), isNot([0xFF, 0x00, 0x00]));
      expect(await pixel(tester, false, 126, 22), isNot([0xFF, 0x00, 0x00]));
    });
  });

  group('editor', () {
    testWidgets('shows a bubble while the pointer rests on a cell with a note or a thread', (tester) async {
      await open(tester);
      final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse);
      await mouse.addPointer(location: at(tester, 1, 1));
      await tester.pump();
      expect(find.text('Alice'), findsNothing);

      await mouse.moveTo(at(tester, 2, 2));
      await tester.pump();
      expect(find.text('Alice'), findsOneWidget);
      expect(find.text('Vérifier\nce total'), findsOneWidget);
      expect(
        tester.widget<DecoratedBox>(find.ancestor(of: find.text('Alice'), matching: find.byType(DecoratedBox)).first).decoration,
        isA<BoxDecoration>().having((d) => d.color, 'color', const Color(0xFFFFFFE1)),
      );

      await mouse.moveTo(at(tester, 3, 3));
      await tester.pump();
      expect(find.text('Alice'), findsOneWidget);
      expect(find.text('Bob'), findsOneWidget);
      expect(find.text('Pourquoi ?'), findsOneWidget);
      expect(find.text('Parce que'), findsOneWidget);
      expect(find.text('Vérifier\nce total'), findsNothing);

      await mouse.moveTo(at(tester, 1, 1));
      await tester.pump();
      expect(find.text('Bob'), findsNothing);

      await mouse.moveTo(at(tester, 2, 2));
      await tester.pump();
      await mouse.moveTo(const Offset(-20, -20));
      await tester.pump();
      expect(find.text('Alice'), findsNothing);
      await finish(tester);
    });

    testWidgets('a new comment starts a thread signed by the user', (tester) async {
      await open(tester);
      await select(tester, 5, 4);
      await review(tester, 'Nouveau commentaire');
      await tester.enterText(find.widgetWithText(TextField, 'Commencer une conversation'), 'À voir\nvite');
      await tester.pump();
      await tester.tap(find.text('Publier'));
      await settle(tester);
      final items = cell(5, 4)!['thread']! as List<Object?>;
      final first = items.single! as Map<String, Object?>;
      expect(first['author'], session.name);
      expect(first['text'], 'À voir\nvite');
      expect(first['id'], matches(RegExp(r'^\{[0-9A-F]{8}(-[0-9A-F]{4}){3}-[0-9A-F]{12}\}$')));
      expect(first['date'], matches(RegExp(r'^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$')));
      expect(cell(5, 4)!.containsKey('note'), false);
      expect(find.text('À voir\nvite'), findsOneWidget);
      await finish(tester);
    });

    testWidgets('a new note is signed by the user and has no shape', (tester) async {
      await open(tester);
      await select(tester, 6, 5);
      await review(tester, 'Nouvelle note');
      await tester.enterText(find.widgetWithText(TextField, 'Commencer une conversation'), 'Un rappel');
      await tester.pump();
      await tester.tap(find.text('Publier'));
      await settle(tester);
      expect(cell(6, 5)!['note'], {'author': session.name, 'text': 'Un rappel'});
      expect(cell(6, 5)!.containsKey('thread'), false);
      await finish(tester);
    });

    testWidgets('a cell with a comment or a note cannot receive another', (tester) async {
      await open(tester);
      await select(tester, 1, 1);
      await tester.tap(find.text('Révision'));
      await tester.pump();
      expect(pressable(tester, 'Nouveau commentaire'), isNotNull);
      expect(pressable(tester, 'Nouvelle note'), isNotNull);
      expect(pressable(tester, 'Modifier la note'), isNull);
      for (final (r, c) in [(2, 2), (3, 3)]) {
        await select(tester, r, c);
        expect(pressable(tester, 'Nouveau commentaire'), isNull);
        expect(pressable(tester, 'Nouvelle note'), isNull);
        expect(pressable(tester, 'Modifier la note'), isNotNull);
      }
      await finish(tester);
    });

    testWidgets('lists the notes and threads of the sheet, a click selecting their cell', (tester) async {
      await open(tester);
      await review(tester, 'Afficher les commentaires');
      expect(find.text('Pourquoi ?'), findsOneWidget);
      expect(find.text('Parce que'), findsOneWidget);
      expect(tester.getTopLeft(find.text('Vérifier\nce total')).dy, lessThan(tester.getTopLeft(find.text('Pourquoi ?')).dy));
      await tester.tap(find.text('Pourquoi ?'));
      await tester.pump();
      expect(find.text('C3'), findsOneWidget);
      expect(find.widgetWithText(TextField, 'Répondre'), findsOneWidget);
      await tester.tap(find.text('Vérifier\nce total'));
      await tester.pump();
      expect(find.text('B2'), findsOneWidget);
      expect(find.widgetWithText(TextField, 'Répondre'), findsNothing);
      await finish(tester);
    });

    testWidgets('answers, edits and resolves a thread', (tester) async {
      await open(tester);
      await review(tester, 'Afficher les commentaires');
      await tester.tap(find.text('Pourquoi ?'));
      await tester.pump();
      await tester.enterText(find.widgetWithText(TextField, 'Répondre'), 'Merci');
      await tester.pump();
      await tester.tap(find.byIcon(Icons.send));
      await settle(tester);
      expect(thread().length, 3);
      expect(thread().sublist(0, 2), [_first, _answer]);
      final mine = thread()[2]! as Map<String, Object?>;
      expect(mine['author'], session.name);
      expect(mine['text'], 'Merci');
      expect(mine.containsKey('done'), false);

      await choose(tester, find.byTooltip('Autres actions de thread').at(3), 'Modifier le commentaire');
      await tester.enterText(find.widgetWithText(TextField, 'Merci'), 'Merci bien');
      await tester.tap(find.text('Enregistrer'));
      await settle(tester);
      expect((thread()[2]! as Map)['text'], 'Merci bien');
      expect((thread()[2]! as Map)['id'], mine['id']);

      await choose(tester, find.byTooltip('Autres actions de thread').at(1), 'Résoudre le thread');
      expect(thread()[0], {..._first, 'done': true});
      expect(thread()[1], _answer);

      await tester.tap(find.text('Rouvrir'));
      await settle(tester);
      expect(thread()[0], _first);
      await finish(tester);
    });

    testWidgets('deletes a reply, then a thread, then a note', (tester) async {
      await open(tester);
      await review(tester, 'Afficher les commentaires');
      await tester.tap(find.text('Pourquoi ?'));
      await tester.pump();
      await choose(tester, find.byTooltip('Autres actions de thread').at(2), 'Supprimer le commentaire');
      expect(thread(), [_first]);
      await choose(tester, find.byTooltip('Autres actions de thread').at(1), 'Supprimer le thread');
      expect(cell(3, 3), isNull);
      await choose(tester, find.byTooltip('Autres actions de thread').first, 'Supprimer le thread');
      expect(cell(2, 2), {'v': 7});
      expect(find.text('Aucun commentaire'), findsOneWidget);
      await finish(tester);
    });

    testWidgets('a note has no answer and cannot be resolved, but anyone edits its text', (tester) async {
      await open(tester);
      await review(tester, 'Afficher les commentaires');
      await tester.tap(find.text('Vérifier\nce total'));
      await tester.pump();
      await tester.tap(find.byTooltip('Autres actions de thread').first);
      await tester.pumpAndSettle();
      expect(find.text('Résoudre le thread'), findsNothing);
      expect(find.text('Modifier le commentaire'), findsOneWidget);
      await tester.tap(find.text('Modifier le commentaire'));
      await tester.pumpAndSettle();
      await tester.enterText(find.widgetWithText(TextField, 'Vérifier\nce total'), 'Revu');
      await tester.tap(find.text('Enregistrer'));
      await settle(tester);
      expect(cell(2, 2)!['note'], {..._note, 'text': 'Revu'});
      await finish(tester);
    });

    testWidgets('Supprimer in the ribbon deletes the note of the active cell', (tester) async {
      await open(tester);
      await select(tester, 2, 2);
      await review(tester, 'Supprimer');
      await settle(tester);
      expect(cell(2, 2), {'v': 7});
      await finish(tester);
    });

    testWidgets('typing a value keeps the note of the cell', (tester) async {
      await open(tester);
      await select(tester, 2, 2);
      await tester.sendKeyEvent(LogicalKeyboardKey.f2);
      await tester.pump();
      await tester.enterText(find.byType(EditableText).last, '9');
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await settle(tester);
      expect(cell(2, 2), {'v': 9, 'note': _note});
      await finish(tester);
    });
  });
}
