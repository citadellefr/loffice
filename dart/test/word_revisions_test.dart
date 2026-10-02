import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/edits.dart';
import 'package:loffice/src/word/revisions.dart';

void main() {
  const date = '2026-09-29T08:30:00Z';
  const struck = {'del': 'Moi', 'deld': date};
  const typed = {'ins': 'Moi', 'insd': date};

  test('dates a revision to the minute, in UTC', () {
    expect(revisionDate(DateTime.utc(2026, 9, 29, 8, 30, 42, 123)), date);
  });

  test('strikes what is typed over and inserts after it, as Word shows it', () {
    final flow = Delta()..insert('abcdef\n');
    final t = track(flow, wordEditing(Node(id: 't', type: 'text', parent: 'body', key: 'V', text: flow)).replace(2, 4, 'XY', const {}), 'Moi', date);
    expect(t.delta, Delta()
      ..retain(2)
      ..retain(2, struck)
      ..insert('XY', typed));
    expect(flow.compose(t.delta).text, 'abcdXYef\n');
    expect(t.offset(4), 6);
    expect(t.offset(2, after: false), 2);
  });

  test('deletes for good only what the author inserted', () {
    final flow = Delta()
      ..insert('ab')
      ..insert('XY', typed)
      ..insert('cd\n');
    final t = track(flow, Delta()
      ..retain(3)
      ..delete(2), 'Moi', date);
    expect(t.delta, Delta()
      ..retain(3)
      ..delete(1)
      ..retain(1, struck));
    expect(t.offset(3, after: false), 3);
    expect(t.offset(3), 4);
  });

  test('leaves what is struck already, and joins no paragraphs', () {
    final flow = Delta()
      ..insert('a')
      ..insert('b', {'del': 'Bob', 'deld': '2026-09-01T10:00:00Z'})
      ..insert('c\n', {'jc': 'center'})
      ..insert('d\n');
    final node = Node(id: 't', type: 'text', parent: 'body', key: 'V', text: flow);
    final t = track(flow, wordEditing(node).delete(0, 5), 'Moi', date);
    expect(t.delta, Delta()
      ..retain(1, struck)
      ..retain(1)
      ..retain(3, struck));
  });

  group('settles', () {
    final flow = Delta()
      ..insert('Un ')
      ..insert('ajout', {'ins': 'Alice', 'insd': '2026-09-01T10:00:00Z'})
      ..insert(' retiré', {'del': 'Bob', 'deld': '2026-09-02T10:00:00Z'})
      ..insert('\n', {'ins': 'Alice', 'insd': '2026-09-01T10:00:00Z'})
      ..insert('Fin\n');
    final node = Node(id: 't', type: 'text', parent: 'body', key: 'V', text: flow);

    test('lists the changes in order', () {
      final all = revisionsOf([node]);
      expect([for (final r in all) (r.start, r.end, r.deleted, r.author)], [(3, 8, false, 'Alice'), (8, 15, true, 'Bob'), (15, 16, false, 'Alice')]);
      expect(all.first.date, DateTime.utc(2026, 9, 1, 10));
    });

    test('accepting keeps the insertions and drops the deletions', () {
      final after = flow.compose(settle([(node, 0, flow.length)], accept: true).changes.single.text!);
      expect(after.text, 'Un ajout\nFin\n');
      expect(after.ops.any((op) => op.attributes?.keys.any(revisionKeys.contains) ?? false), isFalse);
    });

    test('rejecting drops the insertions and keeps the deletions', () {
      final after = flow.compose(settle([(node, 0, flow.length)], accept: false).changes.single.text!);
      expect(after.text, 'Un  retiréFin\n');
      expect(after.ops.any((op) => op.attributes?.keys.any(revisionKeys.contains) ?? false), isFalse);
    });

    test('only unmarks the last mark of a flow', () {
      final last = Delta()
        ..insert('x')
        ..insert('\n', {'ins': 'Alice'});
      final n = Node(id: 'l', type: 'text', parent: 'body', key: 'V', text: last);
      expect(last.compose(settle([(n, 0, 2)], accept: false).changes.single.text!), Delta()..insert('x\n'));
    });
  });
}
