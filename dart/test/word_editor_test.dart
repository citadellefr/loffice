import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/edits.dart';
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
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/$fixture.json').readAsStringSync()))!);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: WordEditor(session: session, media: (_) async => Uint8List(0), title: '$fixture.docx')),
    ));
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  Node body() => flowsOf(hub.doc).first;

  Future<void> ctrl(WidgetTester tester, LogicalKeyboardKey key) async {
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyEvent(key);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
  }

  /// Clicks the first page near its top-left corner, in the text.
  Future<void> clickText(WidgetTester tester) async {
    final page = find.byType(CustomPaint).evaluate().map((e) => e.renderObject! as RenderBox).firstWhere((b) => b.size.width < 1000 && b.size.height > 1000);
    await tester.tapAt(page.localToGlobal(const Offset(150, 110)));
    await settle(tester);
  }

  testWidgets('shows the ribbon, the pages and the status', (tester) async {
    await open(tester, 'par-known-styles');
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Mise en page'), findsWidgets);
    expect(find.text('Page 1 sur 1'), findsOneWidget);
    expect(find.textContaining('mots'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('fits a phone', (tester) async {
    await open(tester, 'par-known-styles', size: const Size(390, 844));
    expect(find.text('Accueil'), findsOneWidget);
    expect(find.text('Page 1 sur 1'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('types, bolds and splits paragraphs', (tester) async {
    await open(tester, 'par-known-styles');
    await clickText(tester);
    final before = body().text!.text;
    await ctrl(tester, LogicalKeyboardKey.home);
    tester.testTextInput.updateEditingValue(TextEditingValue(
      text: 'Bref ${before.substring(0, before.length - 1)}',
      selection: const TextSelection.collapsed(offset: 5),
    ));
    await settle(tester);
    expect(body().text!.text, startsWith('Bref '));

    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    for (var i = 0; i < 4; i++) {
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
    }
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    await ctrl(tester, LogicalKeyboardKey.keyG);
    expect(wordEditing(body()).attributesAt(1)?['b'], '1');

    final paragraphs = body().text!.text.split('\n').length;
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowRight);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(body().text!.text.split('\n').length, paragraphs + 1);
    await finish(tester);
  });

  testWidgets('applies a style and centers', (tester) async {
    await open(tester, 'par-known-styles');
    await clickText(tester);
    await ctrl(tester, LogicalKeyboardKey.keyE);
    expect(wordEditing(body()).markAt(0)['jc'], 'center');
    await finish(tester);
  });

  testWidgets('selects all, across the tables, and bolds it', (tester) async {
    await open(tester, 'tbl-having-applied-style');
    await clickText(tester);
    await ctrl(tester, LogicalKeyboardKey.keyA);
    await ctrl(tester, LogicalKeyboardKey.keyG);
    for (final flow in flowsOf(hub.doc)) {
      if (flow.text!.length > 1) expect(wordEditing(flow).attributesAt(0)?['b'], '1', reason: flow.id);
    }
    await finish(tester);
  });

  test('deletes across flows: the blocks between gone, the ends joined', () {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/tbl-having-applied-style.json').readAsStringSync()))!)!;
    final body = tree.children('body');
    final first = body.firstWhere((n) => n.type == 'text');
    final last = body.lastWhere((n) => n.type == 'text');
    final text = last.text!.text;
    final selection = WordSelection()
      ..set(first.id, 0)
      ..extendTo(last.id, 0);
    final ranges = selection.ranges(tree, flowsOf(tree));
    expect(ranges.first.$1.id, first.id);
    expect(ranges.last, (last, 0, 0));
    expect(tree.apply(deleteRanges(tree, ranges)), isNotNull);
    // the table before the first flow stays, those between go
    expect(tree.children('body').map((n) => n.type), ['tbl', 'text']);
    expect(tree[first.id]!.text!.text, text);
    expect(tree[last.id], isNull);
  });

  test('joins the paragraphs a deletion across flows meets in, the first keeping its formatting', () {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!)!;
    expect(tree.apply(WordEdits(WordDocument(tree)).insertTable(flowsOf(tree).first, 0, 1, 1, 400)), isNotNull);
    final kids = tree.children('body');
    final (first, last) = (kids[0], kids[2]);
    final (a, b) = (wordEditing(first), wordEditing(last));
    final selection = WordSelection()
      ..set(first.id, 2)
      ..extendTo(last.id, 3);
    expect(tree.apply(deleteRanges(tree, selection.ranges(tree, flowsOf(tree)))), isNotNull);
    expect(tree.children('body').map((n) => n.type), ['text']);
    final joined = wordEditing(tree[first.id]!);
    expect(joined.text, a.text.substring(0, 2) + b.text.substring(3));
    expect(WordEdits.paragraphLike(joined.markAt(0)), WordEdits.paragraphLike(a.markAt(0)));
    expect(joined.attributesAt(joined.text.length - 1), b.attributesAt(b.text.length - 1));
  });

  testWidgets('inserts a picture the host picked, no wider than the text', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: WordEditor(
          session: session,
          media: (_) async => Uint8List(0),
          onPicture: () async => (media: 'pic', width: 4000, height: 2000),
        ),
      ),
    ));
    await settle(tester);
    await clickText(tester);
    await tester.tap(find.text('Insertion'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Images'));
    await settle(tester);
    final img = body().text!.ops.map((o) => o.attributes?['img']).nonNulls.single;
    final p = jsonDecode(img) as Map<String, Object?>;
    expect(p['media'], 'pic');
    expect((p['w']! as int) / 12700, lessThan(500));
    expect((p['w']! as int) / (p['h']! as int), closeTo(2, 0.01));
    await finish(tester);
  });

  test('finds and replaces in every flow, with the formatting found', () {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/tbl-having-applied-style.json').readAsStringSync()))!)!;
    final matches = findIn(flowsOf(tree), 'FOO');
    expect(matches.length, greaterThan(3));
    expect(findIn(flowsOf(tree), 'FOO', matchCase: true), isEmpty);
    final before = matches.first.$1;
    final attributes = wordEditing(before).attributesAt(matches.first.$2);
    expect(tree.apply(replaceAll(matches, 'truc')), isNotNull);
    expect(findIn(flowsOf(tree), 'foo'), isEmpty);
    expect(findIn(flowsOf(tree), 'truc').length, matches.length);
    expect(wordEditing(tree[before.id]!).attributesAt(matches.first.$2), attributes);
  });

  testWidgets('opens the search with Ctrl+H and replaces everything', (tester) async {
    await open(tester, 'tbl-having-applied-style');
    await clickText(tester);
    await ctrl(tester, LogicalKeyboardKey.keyH);
    await tester.enterText(find.byType(TextField).first, 'bar');
    await tester.enterText(find.byType(TextField).last, 'baz');
    await tester.pump();
    expect(find.textContaining('résultats'), findsOneWidget);
    await tester.tap(find.text('Remplacer tout'));
    await settle(tester);
    expect(findIn(flowsOf(hub.doc), 'bar'), isEmpty);
    expect(findIn(flowsOf(hub.doc), 'baz'), isNotEmpty);
    await finish(tester);
  });

  testWidgets('tracks what is typed and deleted, then accepts it all', (tester) async {
    await open(tester, 'par-known-styles');
    await clickText(tester);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    await ctrl(tester, LogicalKeyboardKey.keyE);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    expect(hub.doc['doc']!.attributes['track'], isTrue);
    expect(find.text('Suivi des modifications : activé'), findsOneWidget);

    await ctrl(tester, LogicalKeyboardKey.home);
    final before = body().text!.text;
    tester.testTextInput.updateEditingValue(TextEditingValue(
      text: 'Bref ${before.substring(0, before.length - 1)}',
      selection: const TextSelection.collapsed(offset: 5),
    ));
    await settle(tester);
    expect(wordEditing(body()).attributesAt(0)?['ins'], 'Peer 1');

    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(body().text!.text, 'Bref $before');
    expect(wordEditing(body()).attributesAt(5)?['del'], 'Peer 1');
    expect(find.textContaining('Supprimé : Peer 1'), findsOneWidget);

    // back over the text struck, then over the space typed, which goes
    await tester.sendKeyEvent(LogicalKeyboardKey.backspace);
    await tester.sendKeyEvent(LogicalKeyboardKey.backspace);
    await settle(tester);
    expect(body().text!.text, 'Bref$before');

    await tester.tap(find.text('Révision'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Accepter').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Accepter toutes les modifications'));
    await settle(tester);
    expect(body().text!.text, 'Bref${before.substring(1)}');
    expect(body().text!.ops.any((op) => op.attributes?.keys.any(const {'ins', 'del'}.contains) ?? false), isFalse);
    await finish(tester);
  });

  testWidgets('lays out again what is typed', (tester) async {
    await open(tester, 'par-known-styles');
    expect(find.text('Page 1 sur 1'), findsOneWidget);
    final flow = flowsOf(session.document).first;
    session.edit(Edit([Change.text(flow.id, Delta()..insert('x\n' * 200))]));
    await settle(tester);
    expect(find.text('Page 1 sur 1'), findsNothing);
    await finish(tester);
  });

  testWidgets('comments a word, answers, resolves and deletes the thread', (tester) async {
    await open(tester, 'par-known-styles');
    await clickText(tester);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.controlLeft);
    await tester.sendKeyDownEvent(LogicalKeyboardKey.altLeft);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyM);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.altLeft);
    await tester.sendKeyUpEvent(LogicalKeyboardKey.controlLeft);
    await settle(tester);
    await tester.enterText(find.widgetWithText(TextField, 'Commencer une conversation'), 'Pourquoi ?');
    await tester.pump();
    await tester.tap(find.text('Publier'));
    await settle(tester);
    List<Node> comments() => hub.doc.children('doc').where((n) => n.type == 'comment').toList();
    expect(comments().single.attributes['author'], 'Peer 1');
    final id = comments().single.id;
    final anchors = [for (final op in body().text!.ops) if (op.attributes != null) ...op.attributes!.entries.where((e) => e.value == id).map((e) => e.key)];
    expect(anchors, ['cs', 'ce', 'comment']);
    expect(find.text('Pourquoi ?'), findsOneWidget);

    await tester.enterText(find.widgetWithText(TextField, 'Répondre'), 'Parce que.');
    await tester.pump();
    await tester.tap(find.byTooltip('Publier'));
    await settle(tester);
    expect(comments().map((c) => c.attributes['parent']), [null, id]);

    await tester.tap(find.byTooltip('Autres actions de thread').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Résoudre le thread'));
    await settle(tester);
    expect(comments().every((c) => c.attributes['done'] == true), isTrue);
    expect(find.text('Résolu'), findsOneWidget);

    await tester.tap(find.byTooltip('Autres actions de thread').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Supprimer le thread'));
    await settle(tester);
    expect(comments(), isEmpty);
    expect(body().text!.ops.where((op) => op.attributes?.containsKey('cs') ?? false), isEmpty);
    expect(find.text('Aucun commentaire'), findsOneWidget);
    await finish(tester);
  });

  testWidgets('shows the comments of a document beside its pages', (tester) async {
    await open(tester, 'comments-rich-para');
    expect(find.text('Commentaires'), findsWidgets);
    expect(find.text('Steve Canny'), findsOneWidget);
    expect(find.text('Text with character style.'), findsOneWidget);
    await tester.tap(find.text('Text with character style.'));
    await settle(tester);
    expect(find.widgetWithText(TextField, 'Répondre'), findsOneWidget);
    await finish(tester);
  });

  test('inserts a table after the paragraph, the flow cut there', () {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!)!;
    final doc = WordDocument(tree);
    final flow = flowsOf(tree).first;
    final text = flow.text!.text;
    final edit = WordEdits(doc).insertTable(flow, 0, 2, 3, 400);
    expect(tree.apply(edit), isNotNull);
    final kids = tree.children('body');
    expect(kids.map((n) => n.type).take(3), ['text', 'tbl', 'text']);
    final table = kids[1];
    expect(tree.children(table.id), hasLength(2));
    expect(tree.children(tree.children(table.id).first.id), hasLength(3));
    expect(table.attributes['grid'], [2666, 2666, 2666]);
    expect('${kids[0].text!.text}${kids[2].text!.text}', text);

    final cell = tree.children(tree.children(table.id).first.id).first;
    expect(tree.apply(WordEdits(WordDocument(tree)).insertColumn(cell, right: true)), isNotNull);
    expect(tree.children(tree.children(table.id).first.id), hasLength(4));
    expect(tree[table.id]!.attributes['grid'], hasLength(4));
    expect(tree.apply(WordEdits(WordDocument(tree)).deleteRow(cell)), isNotNull);
    expect(tree.children(table.id), hasLength(1));
  });
}
