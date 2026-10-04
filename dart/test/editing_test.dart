import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/text/editing.dart';
import 'package:trame/trame.dart';

/// "Un deux\n" in bold then "trois\n", centered and bulleted.
final _flow = Delta([
  const Op.insert('Un deux', {'b': '1'}),
  const Op.insert('\n', {'algn': 'l', 'sz': '2000'}),
  const Op.insert('trois 😀'),
  const Op.insert('\n', {'algn': 'ctr', 'bu': 'char:•'}),
]);

Delta _apply(Delta? d) => _flow.compose(d!);

void main() {
  final e = FlowEditing(_flow);

  test('types with the formatting around', () {
    expect(e.typingAttributes(3), {'b': '1'});
    expect(e.typingAttributes(0), {'b': '1'});
    expect(e.typingAttributes(8), <String, String>{});
    expect(_apply(e.replace(2, 2, '!', e.typingAttributes(2))).text, 'Un! deux\ntrois 😀\n');
    final empty = FlowEditing(Delta([const Op.insert('\n', {'algn': 'r', 'sz': '4000', 'p': '<a:pPr/>'})]));
    expect(empty.typingAttributes(0), {'sz': '4000'});
  });

  test('Enter makes a paragraph like the one it breaks', () {
    final after = _apply(e.newParagraph(8 + 5, 8 + 5));
    expect(after.text, 'Un deux\ntrois\n 😀\n');
    expect(after.ops.where((o) => o.insert == '\n').map((o) => o.attributes?['algn']), ['l', 'ctr', 'ctr']);
  });

  test('Backspace deletes a character, or joins paragraphs keeping the first', () {
    expect(_apply(e.deleteBackward(2, 2)).text, 'U deux\ntrois 😀\n');
    // an emoji is two units, deleted whole
    expect(_apply(e.deleteBackward(16, 16)).text, 'Un deux\ntrois \n');
    final joined = _apply(e.deleteBackward(8, 8));
    expect(joined.text, 'Un deuxtrois 😀\n');
    expect(joined.ops.last.attributes, {'algn': 'l'});
    expect(e.deleteBackward(0, 0), isNull);
    expect(_apply(e.deleteBackward(3, 11)).text, 'Un is 😀\n');
  });

  test('Delete never takes the last mark', () {
    expect(e.deleteForward(16, 16), isNull);
    expect(_apply(e.deleteForward(14, 14)).text, 'Un deux\ntrois \n');
    expect(_apply(e.deleteForward(10, 17)).text, 'Un deux\ntr\n');
  });

  test('formats runs and paragraphs', () {
    final bold = _apply(e.formatRuns(8, 13, {'b': '1'}));
    expect(bold.ops[2], const Op.insert('trois', {'b': '1'}));
    final unbold = _apply(e.formatRuns(0, 2, {'b': ''}));
    expect(unbold.ops.first, const Op.insert('Un'));
    expect(e.formatRuns(3, 3, {'b': '1'}), isNull);

    final centered = _apply(e.formatParagraphs(2, 10, {'algn': 'r'}));
    expect(centered.ops.where((o) => o.insert == '\n').map((o) => o.attributes?['algn']), ['r', 'r']);
    expect(e.paragraphsIn(0, 3).single['algn'], 'l');
    expect(e.paragraphsIn(0, 12).map((a) => a['algn']), ['l', 'ctr']);
  });

  test('moves by character and by word', () {
    expect(e.next(14), 16);
    expect(e.previous(16), 14);
    expect(e.next(0, word: true), 2);
    expect(e.next(2, word: true), 7);
    expect(e.previous(7, word: true), 3);
    expect(e.wordAt(4), (3, 7));
    expect(e.next(16), 16);
  });
}
