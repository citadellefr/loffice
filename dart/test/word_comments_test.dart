import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/comments.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/edits.dart';

void main() {
  Tree fixture() => Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/docx/comments-rich-para.json').readAsStringSync()))!)!;
  final by = (name: 'Alice Martin', date: DateTime.utc(2026, 9, 29, 8), style: 'CommentText');

  test('reads the threads in the order of the text, with their ranges', () {
    final tree = fixture();
    final threads = commentThreads(tree);
    expect(threads.map((t) => t.id), ['cm0', 'cm1', 'cm2', 'cm4']);
    expect(threads.first.root.author, 'Steve Canny');
    expect(threads.first.root.initials, 'SJC');
    expect(threads[2].root.text, 'Text with character style.');
    final flows = flowsOf(tree);
    for (final t in threads) {
      expect(t.anchored, isTrue);
      final ranges = threadRanges(t, flows);
      expect(ranges, isNotEmpty);
      final (flow, a, b) = ranges.first;
      expect(tree[flow]!.text!.text.substring(a, b), isNot(contains('￼')));
    }
  });

  test('adds a comment across flows, answers it, resolves it and deletes it', () {
    final tree = fixture();
    expect(tree.apply(WordEdits(WordDocument(tree)).insertTable(flowsOf(tree).first, 0, 1, 1, 400)), isNotNull);
    final flows = flowsOf(tree);
    final (first, last) = (flows.first, flows.last);
    final add = addComment(tree, (first.id, 0), (last.id, 2), 'Pourquoi ?\nEt comment ?', by);
    expect(tree.apply(add), isNotNull);
    final id = add.changes.first.id;
    var thread = commentThreads(tree).firstWhere((t) => t.id == id);
    expect(thread.root.author, 'Alice Martin');
    expect(thread.root.initials, 'AM');
    expect(thread.root.text, 'Pourquoi ?\nEt comment ?');
    expect(tree[id]!.attributes['date'], '2026-09-29T08:00:00Z');
    expect(thread.start, (first.id, 1));
    expect(thread.end!.$1, last.id);
    expect(flowsOf(tree, id).first.text!.ops.last.attributes, {'pstyle': 'CommentText'});

    expect(tree.apply(replyTo(tree, thread, 'Parce que.', (name: 'Bob', date: DateTime.utc(2026, 9, 29, 9), style: null))), isNotNull);
    thread = commentThreads(tree).firstWhere((t) => t.id == id);
    final reply = thread.replies.single;
    expect(reply.parent, id);
    final anchors = commentAnchors(flowsOf(tree));
    expect(anchors['cs']![reply.id], (first.id, 1));
    expect(anchors['ce']![reply.id], (last.id, thread.reference!.$2 + 1));

    expect(tree.apply(resolveThread(thread, true)), isNotNull);
    expect(commentThreads(tree).firstWhere((t) => t.id == id).all.every((c) => c.done), isTrue);

    expect(tree.apply(deleteComments(tree, {id, reply.id})), isNotNull);
    expect(tree[id], isNull);
    final left = commentAnchors(flowsOf(tree));
    expect(left.values.expand((m) => m.keys).toSet(), {'cm0', 'cm1', 'cm2', 'cm4'});
    expect(flowsOf(tree).first.text!.text, first.text!.text);
  });

  test('edits what a comment says, its objects and formatting kept', () {
    final tree = fixture();
    final thread = commentThreads(tree).firstWhere((t) => t.id == 'cm2');
    expect(tree.apply(editComment(tree, thread.root, 'Text with a character style!')), isNotNull);
    final flow = flowsOf(tree, 'cm2').first;
    expect(flow.text!.text.replaceAll('￼', ''), 'Text with a character style!\n');
    expect(flow.text!.text, startsWith('￼'));
    final styled = flow.text!.ops.firstWhere((op) => op.insert!.contains('character'));
    expect(styled.attributes?['rstyle'], 'BookTitle');
  });

  test('gives initials as Word does', () {
    expect(initialsOf('Élodie de la Tour'), 'ÉD');
    expect(initialsOf(''), '');
  });
}
