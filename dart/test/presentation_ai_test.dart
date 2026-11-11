import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/chrome/ai.dart';
import 'package:trame/testing.dart';

void main() {
  const shape = 's256-5';
  const ai = AiParticipant(id: 'ai', name: 'Héraut');
  late FakeHub hub;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> open(WidgetTester tester, {AiParticipant? ai}) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!);
    final session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: PresentationEditor(session: session, media: (_) async => Uint8List(0), ai: ai),
        ),
      ),
    );
    await settle(tester);
  }

  Future<DocSession> join(WidgetTester tester, String id) async {
    final other = DocSession(hub.connect, clientId: id)..start();
    addTearDown(other.dispose);
    await settle(tester);
    return other;
  }

  Future<void> publish(WidgetTester tester) async {
    await tester.pump(const Duration(milliseconds: 60));
    await settle(tester);
  }

  final canvas = find.byWidgetPredicate((w) => w is CustomPaint && w.painter.runtimeType.toString() == '_CanvasPainter');
  final select = const DocSelection(shape, 0, 6);
  final fill = aiColor.withValues(alpha: 0.25);

  PaintPattern drawn(Color color) => paints..something((method, arguments) => method == #drawRect && (arguments[1] as Paint).color.toARGB32() == color.toARGB32());

  testWidgets('draws the cursor of the AI in its own color, not that of a person', (tester) async {
    await open(tester, ai: ai);
    final human = await join(tester, 'human');
    human.select(select);
    await publish(tester);
    expect(canvas, isNot(drawn(aiColor)));

    final bot = await join(tester, 'ai');
    bot.select(select);
    await publish(tester);
    expect(canvas, allOf(drawn(fill), drawn(aiColor)));
  });

  testWidgets('shows who is here, the AI by its mark', (tester) async {
    await open(tester, ai: ai);
    await join(tester, 'human');
    await join(tester, 'ai');
    expect(find.byIcon(Icons.auto_awesome), findsOneWidget);
    await tester.tap(find.text('Fichier'));
    await tester.pump();
    expect(find.byIcon(Icons.auto_awesome), findsNWidgets(2));
    expect(find.text('✦ Héraut'), findsOneWidget);
    expect(find.text('Peer 2'), findsOneWidget);
  });

  testWidgets('lights what the AI edited, then lets it go', (tester) async {
    await open(tester, ai: ai);
    final bot = await join(tester, 'ai');
    expect(canvas, isNot(drawn(fill)));

    bot.share({
      'recent': [
        {'n': shape, 'at': 1},
      ],
    });
    await publish(tester);
    expect(canvas, allOf(drawn(fill), drawn(aiColor)));

    await tester.pump(const Duration(seconds: 5));
    expect(canvas, isNot(drawn(fill)));
    expect(canvas, isNot(drawn(aiColor)));
  });

  testWidgets('shows nothing of an AI the host did not name', (tester) async {
    await open(tester);
    final bot = await join(tester, 'ai');
    bot
      ..select(select)
      ..share({
        'recent': [
          {'n': shape, 'at': 1},
        ],
      });
    await publish(tester);
    expect(canvas, isNot(drawn(aiColor)));
    expect(canvas, isNot(drawn(fill)));
    expect(find.byIcon(Icons.auto_awesome), findsNothing);
    expect(find.byType(CircleAvatar), findsOneWidget);
  });
}
