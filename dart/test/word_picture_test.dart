import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/edits.dart';
import 'package:loffice/src/word/pages_view.dart';
import 'package:trame/testing.dart';

const _emu = 12700;

void main() {
  late FakeHub hub;
  late DocSession session;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Node body() => flowsOf(hub.doc).first;

  /// Opens a document with a picture at the start of its body.
  Future<void> open(WidgetTester tester, Map<String, Object?> picture) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    hub = FakeHub.tree(Edit.fromJson(jsonDecode(File('../testdata/docx/par-known-styles.json').readAsStringSync()))!);
    session = DocSession(hub.connect)..start();
    addTearDown(session.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: WordEditor(session: session, media: (_) async => Uint8List(0)),
        ),
      ),
    );
    await settle(tester);
    session.edit(
      Edit([
        Change.text(body().id, Delta()..insert('￼', {'img': jsonEncode(picture)})),
      ]),
    );
    await settle(tester);
  }

  Map<String, Object?> picture() => jsonDecode(body().text!.ops.map((o) => o.attributes?['img']).nonNulls.single) as Map<String, Object?>;

  WordPagesView view(WidgetTester tester) => tester.widget<WordPagesView>(find.byType(WordPagesView));

  /// Where the picture is on the screen.
  Rect onScreen(WidgetTester tester) {
    final page = find.byType(CustomPaint).evaluate().map((e) => e.renderObject! as RenderBox).firstWhere((b) => b.size.width < 1000 && b.size.height > 1000);
    final (_, rect, _) = view(tester).layout.pictureAt(body().id, 0)!;
    final scale = view(tester).scale;
    return Rect.fromPoints(page.localToGlobal(rect.topLeft * scale), page.localToGlobal(rect.bottomRight * scale));
  }

  Future<void> drag(WidgetTester tester, Offset from, Offset by) async {
    final gesture = await tester.startGesture(from);
    await gesture.moveBy(by);
    await gesture.up();
    await settle(tester);
  }

  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  testWidgets('selects a picture in the line, resizes it by a handle with its proportions, and deletes it', (tester) async {
    await open(tester, {'media': 'pic', 'w': 200 * _emu, 'h': 100 * _emu});
    await tester.tapAt(onScreen(tester).center);
    await settle(tester);
    expect((view(tester).selection.start, view(tester).selection.end), (0, 1));

    final scale = view(tester).scale;
    await drag(tester, onScreen(tester).bottomRight, Offset(60 * scale, 10 * scale));
    expect(picture()['w'], closeTo(260 * _emu, _emu));
    expect(picture()['h'], closeTo(130 * _emu, _emu));
    expect((view(tester).selection.start, view(tester).selection.end), (0, 1));

    await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
    await drag(tester, onScreen(tester).bottomRight, Offset(-60 * scale, -30 * scale));
    await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
    expect(picture()['w'], closeTo(200 * _emu, _emu));
    expect(picture()['h'], closeTo(100 * _emu, _emu));

    await tester.sendKeyEvent(LogicalKeyboardKey.delete);
    await settle(tester);
    expect(body().text!.ops.map((o) => o.attributes?['img']).nonNulls, isEmpty);
    await finish(tester);
  });

  testWidgets('drags a floating picture to another position', (tester) async {
    await open(tester, {
      'media': 'pic',
      'w': 100 * _emu,
      'h': 100 * _emu,
      'float': {'relX': 'column', 'x': 0, 'relY': 'paragraph', 'y': 0, 'wrap': 'square', 'wrapText': 'bothSides'},
    });
    final scale = view(tester).scale;
    final from = onScreen(tester).center;
    await tester.tapAt(from);
    await settle(tester);
    await drag(tester, from, Offset(80 * scale, 50 * scale));
    final float = picture()['float']! as Map<String, Object?>;
    expect(float['x'], closeTo(80 * _emu, 2 * _emu));
    expect(float['y'], closeTo(50 * _emu, 2 * _emu));
    expect(float['wrap'], 'square');
    await finish(tester);
  });
}
