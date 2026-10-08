import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:trame/testing.dart';

void main() {
  late FakeHub hub;
  late DocSession mine;
  late DocSession theirs;

  Future<void> settle(WidgetTester tester) async {
    await tester.runAsync(hub.settle);
    await tester.pump();
  }

  Future<void> pumpEditor(WidgetTester tester, {List<Command> commands = const []}) async {
    hub = FakeHub('one\ntwo');
    mine = DocSession(hub.connect)..start();
    theirs = DocSession(hub.connect)..start();
    addTearDown(mine.dispose);
    addTearDown(theirs.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: PlainTextEditor(session: mine, commands: commands))));
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

  testWidgets('keywords after an @: a mention is written by its name, an answer in place of its question', (tester) async {
    final answers = StreamController<Answer>();
    addTearDown(answers.close);
    final asked = <(String, String, String)>[];
    await pumpEditor(
      tester,
      commands: [
        Command('taches', search: (query) async => [Mention('@Relire le devis', Uri.parse('urn:x:todo?id=1'))]),
        Command(
          'assistant',
          answer: (question, {required before, required after}) {
            asked.add((question, before, after));
            return answers.stream;
          },
        ),
      ],
    );
    Future<void> type(String text) async {
      final value = tester.widget<TextField>(find.byType(TextField)).controller!.value;
      final at = value.selection.extentOffset;
      tester.testTextInput.updateEditingValue(TextEditingValue(
        text: value.text.replaceRange(at, at, text),
        selection: TextSelection.collapsed(offset: at + text.length),
      ));
      await settle(tester);
    }

    await tester.showKeyboard(find.byType(TextField));
    tester.testTextInput.updateEditingValue(const TextEditingValue(text: 'one\ntwo', selection: TextSelection.collapsed(offset: 3)));
    await tester.pump();
    await type(' @');
    expect(find.text('@taches'), findsOneWidget);
    expect(find.text('@assistant'), findsOneWidget);
    await type('t');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(hub.text, 'one @taches \ntwo');
    await tester.pump(const Duration(milliseconds: 10));
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await settle(tester);
    expect(hub.text, 'one @Relire le devis \ntwo');

    await type('@');
    await type('assistant et après ?');
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();
    expect(asked, [('et après ?', 'one @Relire le devis ', '\ntwo')]);
    expect(tester.widget<TextField>(find.byType(TextField)).readOnly, isTrue);
    theirs.replace(0, 0, '# ');
    await settle(tester);
    answers.add(const Answer.done('Puis trois.'));
    await tester.pump();
    await settle(tester);
    expect(hub.text, '# one @Relire le devis Puis trois.\ntwo');
    expect(tester.widget<TextField>(find.byType(TextField)).readOnly, isFalse);
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
