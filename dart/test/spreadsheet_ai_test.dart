import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/ai.dart';
import 'package:trame/testing.dart';

Edit workbook() => Edit([
  Change.create(Node(id: 'book', type: 'book', key: 'V')),
  Change.create(
    Node(
      id: 'S1',
      type: 'sheet',
      parent: 'book',
      key: 'K',
      attributes: const {'name': 'Feuil1'},
      grid: Grid([
        const Cell(1, 1, {'v': 2}),
      ]),
    ),
  ),
]);

void main() {
  late FakeHub hub;
  final ai = const AiParticipant(id: 'ai', name: 'Héraut');
  final named = ValueNotifier<AiParticipant?>(null);
  final glow = aiColor.withValues(alpha: 0.25);

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<DocSession> join(WidgetTester tester, String id) async {
    final session = DocSession(hub.connect, clientId: id)..start();
    addTearDown(session.dispose);
    await settle(tester);
    return session;
  }

  Future<void> open(WidgetTester tester, {AiParticipant? participant}) async {
    named.value = participant;
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(workbook());
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ValueListenableBuilder(
            valueListenable: named,
            builder: (_, ai, _) => SpreadsheetEditor(session: session, title: 'classeur.xlsx', ai: ai),
          ),
        ),
      ),
    );
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  Future<void> act(WidgetTester tester, DocSession who, {bool mark = false}) async {
    who.select(DocSelection.cells('S1', 1, 1, 2, 2));
    if (mark) {
      who.share({
        'recent': [
          {
            'n': 'S1',
            'c': [1, 1, 2, 2],
            'at': 1,
          },
        ],
      });
    }
    await tester.pump(const Duration(seconds: 1));
    await settle(tester);
    await tester.pump(const Duration(milliseconds: 200));
  }

  Finder grid() => find.byWidgetPredicate((w) => w is CustomPaint && w.painter.runtimeType.toString() == '_SheetPainter');
  PaintPattern drawn(Color color) => paints..something((method, args) => method == #drawRect && (args[1] as Paint).color.toARGB32() == color.toARGB32());

  testWidgets("the AI's selection is drawn in its color and a person's is not", (tester) async {
    await open(tester, participant: ai);
    await act(tester, await join(tester, 'ai'));
    expect(tester.renderObject(grid()), drawn(aiColor));
    expect(find.byIcon(Icons.auto_awesome), findsOneWidget);
    await finish(tester);

    await open(tester, participant: ai);
    await act(tester, await join(tester, 'bob'));
    expect(tester.renderObject(grid()), isNot(drawn(aiColor)));
    expect(find.byIcon(Icons.auto_awesome), findsNothing);
    await finish(tester);
  });

  testWidgets('what the AI changed lately is lit a few seconds', (tester) async {
    debugTags = [];
    addTearDown(() => debugTags = null);
    await open(tester, participant: ai);
    await act(tester, await join(tester, 'ai'), mark: true);
    expect(tester.renderObject(grid()), drawn(glow));
    expect(debugTags, contains('✦ Héraut'));
    await tester.pump(const Duration(seconds: 5));
    expect(tester.renderObject(grid()), isNot(drawn(glow)));
    await finish(tester);
  });

  testWidgets('without an AI nothing of it is drawn', (tester) async {
    await open(tester);
    await act(tester, await join(tester, 'ai'), mark: true);
    expect(tester.renderObject(grid()), isNot(drawn(aiColor)));
    expect(tester.renderObject(grid()), isNot(drawn(glow)));
    expect(find.byIcon(Icons.auto_awesome), findsNothing);
    await finish(tester);
  });

  testWidgets('follows an AI the host names after the editor is open', (tester) async {
    await open(tester);
    final bot = await join(tester, 'ai');
    await act(tester, bot, mark: true);
    expect(tester.renderObject(grid()), isNot(drawn(aiColor)));

    debugTags = [];
    addTearDown(() => debugTags = null);
    named.value = ai;
    await tester.pump();
    bot.share({
      'recent': [
        {
          'n': 'S1',
          'c': [1, 1, 2, 2],
          'at': 2,
        },
      ],
    });
    await act(tester, bot);
    expect(tester.renderObject(grid()), allOf(drawn(aiColor), drawn(glow)));
    expect(debugTags, contains('✦ Héraut'));
    await finish(tester);
  });
}
