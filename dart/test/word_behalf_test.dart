import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/comments.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/edits.dart';
import 'package:trame/testing.dart';

/// The nodes under "doc" that tell whom the AI made its revisions for: Go
/// writes them, and they are none of the editor's business.
const _behalf = [
  ('behalf-2026-11-03T08:00:00Z-0000000a', 'Héraut', '2026-11-03T08:00:00Z', 'utilisateur-secret-1'),
  ('behalf-2026-11-03T08:05:00Z-0000000a', 'Héraut', '2026-11-03T08:05:00Z', 'utilisateur-secret-2'),
];

Edit withBehalf(String fixture) {
  final base = Edit.fromJson(jsonDecode(File('../testdata/docx/$fixture.json').readAsStringSync()))!;
  var last = Tree.fromEdit(base)!.children('doc').last.key;
  final nodes = <Change>[];
  for (final (id, author, date, user) in _behalf) {
    last = keyBetween(last, '');
    nodes.add(Change.create(Node(id: id, type: 'behalf', parent: 'doc', key: last, attributes: {'author': author, 'date': date, 'user': user})));
  }
  return Edit([...base.changes, ...nodes]);
}

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  List<(String, Object?)> behalves() => [
    for (final n in hub.doc.children('doc'))
      if (n.type == 'behalf') (n.id, n.attributes['user']),
  ];

  final kept = [for (final (id, _, _, user) in _behalf) (id, user)];

  testWidgets('shows none of the behalf nodes and keeps them through edits', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(withBehalf('par-known-styles'));
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: WordEditor(session: session, media: (_) async => Uint8List(0), title: 'par-known-styles.docx'),
        ),
      ),
    );
    await settle(tester);
    expect(tester.takeException(), isNull);
    expect(find.textContaining('secret'), findsNothing);
    expect(find.text('Page 1 sur 1'), findsOneWidget);

    final page = find.byType(CustomPaint).evaluate().map((e) => e.renderObject! as RenderBox).firstWhere((b) => b.size.width < 1000 && b.size.height > 1000);
    await tester.tapAt(page.localToGlobal(const Offset(150, 110)));
    await settle(tester);
    final body = flowsOf(hub.doc).first;
    expect(body.text!.text, isNot(contains('secret')));
    final value = TextEditingValue.fromJSON(tester.testTextInput.editingState!);
    tester.testTextInput.updateEditingValue(TextEditingValue(text: 'Bref ${value.text}', selection: const TextSelection.collapsed(offset: 5)));
    await settle(tester);
    expect(flowsOf(hub.doc).first.text!.text, startsWith('Bref '));
    expect(tester.takeException(), isNull);
    expect(find.textContaining('secret'), findsNothing);
    expect(behalves(), kept);
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  });

  test('reads, edits and comments a document with behalf nodes, which stay', () {
    final tree = Tree.fromEdit(withBehalf('comments-rich-para'))!;
    final before = [for (final n in tree.children('doc')) n.id];
    expect(WordDocument(tree).lastSection, isNotNull);
    final threads = commentThreads(tree);
    expect(threads.map((t) => t.id), ['cm0', 'cm1', 'cm2', 'cm4']);
    expect(flowsOf(tree).where((f) => f.type != 'text'), isEmpty);

    final doc = WordDocument(tree);
    expect(tree.apply(WordEdits(doc).insertTable(flowsOf(tree).first, 0, 1, 1, 400)), isNotNull);
    final flow = flowsOf(tree).first;
    final add = addComment(tree, (flow.id, 0), (flow.id, 1), 'Une remarque', (name: 'Alice', date: DateTime.utc(2026, 11, 3), style: null));
    expect(tree.apply(add), isNotNull);
    expect(tree.apply(deleteComments(tree, {add.changes.first.id})), isNotNull);
    expect(
      tree.apply(
        Edit([
          Change.set('doc', attributes: {'track': true}),
        ]),
      ),
      isNotNull,
    );

    final nodes = tree.children('doc').where((n) => n.type == 'behalf').toList();
    expect([for (final n in nodes) (n.id, n.attributes['user'])], kept);
    expect(before.where((id) => tree[id] == null), isEmpty);
  });
}
