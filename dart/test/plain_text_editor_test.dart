import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';

import 'fakes.dart';

void main() {
  late FakeHub hub;
  late DocSession mine;
  late DocSession theirs;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> pumpEditor(WidgetTester tester) async {
    hub = FakeHub('one\ntwo');
    mine = DocSession(hub.connect)..start();
    theirs = DocSession(hub.connect)..start();
    addTearDown(mine.dispose);
    addTearDown(theirs.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: PlainTextEditor(session: mine))));
    await settle(tester);
  }

  /// Lets the last selection go out, as the test ends.
  Future<void> finish(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    await tester.pump(const Duration(milliseconds: 100));
  }

  String field(WidgetTester tester) => tester.widget<TextField>(find.byType(TextField)).controller!.text;

  testWidgets('shows the document and sends what is typed', (tester) async {
    await pumpEditor(tester);
    expect(field(tester), 'one\ntwo');
    await tester.enterText(find.byType(TextField), 'one\nthree\ntwo');
    await settle(tester);
    expect(hub.text, 'one\nthree\ntwo');
    expect(theirs.text, 'one\nthree\ntwo');
    await finish(tester);
  });

  testWidgets('follows the edits of others and keeps the caret in place', (tester) async {
    await pumpEditor(tester);
    final controller = tester.widget<TextField>(find.byType(TextField)).controller!;
    controller.selection = const TextSelection.collapsed(offset: 7);
    theirs.replace(0, 0, '>> ');
    await settle(tester);
    expect(field(tester), '>> one\ntwo');
    expect(controller.selection, const TextSelection.collapsed(offset: 10));
    await finish(tester);
  });

  testWidgets('undo reverts only this person\'s typing', (tester) async {
    await pumpEditor(tester);
    await tester.enterText(find.byType(TextField), 'one!\ntwo');
    await settle(tester);
    theirs.replace(0, 0, 'A');
    await settle(tester);
    mine.undo();
    await settle(tester);
    expect(field(tester), 'Aone\ntwo');
    expect(hub.text, 'Aone\ntwo');
    await finish(tester);
  });
}
