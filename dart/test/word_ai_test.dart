import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/ai.dart';
import 'package:loffice/src/word/comments.dart';
import 'package:loffice/src/word/edits.dart';
import 'package:loffice/src/word/revisions.dart';
import 'package:loffice/src/word/revisions_pane.dart';
import 'package:trame/testing.dart';

const _first = '2026-11-03T08:00:00Z';
const _second = '2026-11-03T08:05:00Z';
const _ai = AiParticipant(id: 'ai', name: 'Héraut');

Map<String, String> _by(String author, String date) => {'ins': author, 'insd': date};

/// A document whose first flow starts with an insertion of Héraut and one
/// of Bob, both of the first batch, then one more of Héraut's second batch;
/// the first batch has a comment.
Tree _revised() {
  final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!)!;
  final flow = flowsOf(tree).first;
  final typed = Edit([
    Change.text(
      flow.id,
      Delta()
        ..insert('Alpha ', _by('Héraut', _first))
        ..insert('Bravo ', _by('Bob', _first)),
    ),
  ]);
  expect(tree.apply(typed), isNotNull);
  expect(
    tree.apply(
      Edit([
        Change.text(
          flow.id,
          Delta()
            ..retain(12)
            ..insert('Charlie ', _by('Héraut', _second)),
        ),
      ]),
    ),
    isNotNull,
  );
  final comment = addComment(tree, (flow.id, 0), (flow.id, 6), 'Reformulé pour plus de clarté.', (name: 'Héraut', date: DateTime.utc(2026, 11, 3, 8), style: null));
  expect(tree.apply(comment), isNotNull);
  return tree;
}

Map<String, String?> _marks(Tree tree, String text) {
  final flow = flowsOf(tree).first;
  var at = 0;
  for (final op in flow.text!.ops) {
    final i = flow.text!.text.indexOf(text);
    if (at <= i && i < at + op.insert!.length) return {...?op.attributes};
    at += op.insert!.length;
  }
  return const {};
}

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> sync(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  final named = ValueNotifier<AiParticipant?>(null);

  Future<void> open(WidgetTester tester, {AiParticipant? ai}) async {
    named.value = ai;
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(_revised().toEdit());
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ValueListenableBuilder(
            valueListenable: named,
            builder: (_, ai, _) => WordEditor(session: session, media: (_) async => Uint8List(0), title: 'doc.docx', ai: ai),
          ),
        ),
      ),
    );
    await sync(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  Future<void> openPane(WidgetTester tester) async {
    await tester.tap(find.text('Révision'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Volet des révisions'));
    await tester.pumpAndSettle();
  }

  /// Whether a rect of [color] is drawn somewhere.
  bool paintsColor(Color color) {
    var found = false;
    for (final e in find.byType(CustomPaint).evaluate()) {
      final pattern = paints..something((name, args) => found |= name == #drawRect && (args[1] as Paint).color.toARGB32() == color.toARGB32());
      (pattern as Matcher).matches(e.renderObject!, {});
    }
    return found;
  }

  test('settles only the changes of one author, and leaves the others marked', () {
    final tree = _revised();
    final all = [for (final f in flowsOf(tree)) (f, 0, f.text!.length)];
    final accepted = Tree.fromEdit(tree.toEdit())!;
    expect(accepted.apply(settle(all, accept: true, owns: (a, _) => a == 'Héraut')), isNotNull);
    expect(_marks(accepted, 'Alpha'), isEmpty);
    expect(_marks(accepted, 'Charlie'), isEmpty);
    expect(_marks(accepted, 'Bravo'), _by('Bob', _first));

    final rejected = Tree.fromEdit(tree.toEdit())!;
    expect(rejected.apply(settle(all, accept: false, owns: (a, _) => a == 'Héraut')), isNotNull);
    expect(flowsOf(rejected).first.text!.text, contains('Bravo '));
    expect(flowsOf(rejected).first.text!.text, isNot(contains('Alpha')));
    expect(flowsOf(rejected).first.text!.text, isNot(contains('Charlie')));
    expect(_marks(rejected, 'Bravo'), _by('Bob', _first));
  });

  test('keeps the other author of a change both inserted and struck', () {
    final flow = Node(
      id: 'f',
      type: 'text',
      parent: 'body',
      key: 'V',
      text: Delta()
        ..insert('ab', {'ins': 'Bob', 'insd': _first, 'del': 'Héraut', 'deld': _second})
        ..insert('\n'),
    );
    final accepted = settle([(flow, 0, 3)], accept: false, owns: (a, _) => a == 'Héraut');
    expect(accepted.changes.single.text, Delta()..retain(2, {'del': '', 'deld': ''}));
  });

  test('groups the changes of the AI by batch with their comments, the others one by one', () {
    final tree = _revised();
    final revisions = revisionsOf(flowsOf(tree));
    final comments = [for (final t in commentThreads(tree)) t.root];
    final groups = groupRevisions(revisions, ai: _ai, comments: comments);
    expect([for (final g in groups) (g.author, g.batch, g.revisions.length)], [('Héraut', true, 1), ('Bob', false, 1), ('Héraut', true, 1)]);
    expect(groups[0].comments.map((c) => c.text), ['Reformulé pour plus de clarté.']);
    expect(groups[2].comments, isEmpty);
    expect(groupRevisions(revisions, comments: comments).every((g) => !g.batch && g.comments.isEmpty), isTrue);
  });

  testWidgets('accepts all the changes of an author from the review tab and leaves the others', (tester) async {
    await open(tester, ai: _ai);
    await tester.tap(find.text('Révision'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Accepter').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Accepter toutes les modifications de ✦ Héraut'));
    await sync(tester);
    expect(_marks(hub.doc, 'Alpha'), isEmpty);
    expect(_marks(hub.doc, 'Charlie'), isEmpty);
    expect(_marks(hub.doc, 'Bravo'), _by('Bob', _first));
    await finish(tester);
  });

  testWidgets('lists the batches of the AI with their comment in the revisions pane and settles one', (tester) async {
    await open(tester, ai: _ai);
    await openPane(tester);
    final pane = find.byType(RevisionsPane);
    expect(find.descendant(of: pane, matching: find.textContaining('Lot de ✦ Héraut')), findsNWidgets(2));
    expect(find.descendant(of: pane, matching: find.text('Reformulé pour plus de clarté.')), findsOneWidget);
    expect(find.descendant(of: pane, matching: find.textContaining('Inséré : Bob')), findsOneWidget);

    await tester.tap(find.descendant(of: pane, matching: find.text('Refuser le lot')).first);
    await sync(tester);
    expect(flowsOf(hub.doc).first.text!.text, isNot(contains('Alpha')));
    expect(_marks(hub.doc, 'Charlie'), _by('Héraut', _second));
    expect(_marks(hub.doc, 'Bravo'), _by('Bob', _first));
    await finish(tester);
  });

  testWidgets('shows nothing of an AI when the host does not enable it', (tester) async {
    await open(tester);
    final other = DocSession(hub.connect, clientId: 'ai')..start();
    addTearDown(other.dispose);
    await sync(tester);
    other.select(DocSelection(flowsOf(hub.doc).first.id, 12, 20));
    await tester.pump(const Duration(milliseconds: 100));
    await sync(tester);
    await openPane(tester);
    await tester.tap(find.byTooltip('Accepter').first);
    await tester.pumpAndSettle();
    expect(find.text('Accepter toutes les modifications de Héraut'), findsOneWidget);
    expect(find.textContaining('✦'), findsNothing);
    expect(find.byIcon(Icons.auto_awesome), findsNothing);
    expect(find.textContaining('Lot de'), findsNothing);
    expect(paintsColor(aiColor), isFalse);
    await finish(tester);
  });

  testWidgets('draws the presence and the cursor of the AI with its own marker', (tester) async {
    await open(tester, ai: _ai);
    final other = DocSession(hub.connect, clientId: 'ai')..start();
    addTearDown(other.dispose);
    await sync(tester);
    expect(find.byIcon(Icons.auto_awesome), findsOneWidget);
    expect(paintsColor(aiColor), isFalse);
    other.select(DocSelection(flowsOf(hub.doc).first.id, 12, 20));
    await tester.pump(const Duration(milliseconds: 100));
    await sync(tester);
    await tester.pump(const Duration(milliseconds: 200));
    expect(paintsColor(aiColor), isTrue);
    await finish(tester);
  });

  testWidgets('follows an AI the host names after the editor is open', (tester) async {
    await open(tester);
    final other = DocSession(hub.connect, clientId: 'ai')..start();
    addTearDown(other.dispose);
    await sync(tester);
    other.select(DocSelection(flowsOf(hub.doc).first.id, 12, 20));
    await tester.pump(const Duration(milliseconds: 100));
    await sync(tester);
    expect(paintsColor(aiColor), isFalse);

    debugTags = [];
    addTearDown(() => debugTags = null);
    named.value = _ai;
    await tester.pump(const Duration(milliseconds: 200));
    expect(paintsColor(aiColor), isTrue);
    expect(debugTags, contains('✦ Héraut'));
    await finish(tester);
  });
}
