import 'dart:ui';

import 'package:trame/trame.dart';

import '../chrome/comments.dart';

/// Where comments sit, in 1/576 inch, and so in points.
const _commentUnit = 72 / 576;

/// The comments a slide holds: the node of each, its text one a line.
DocComment _comment(Node n) => DocComment(n, _text(n));

String _text(Node n) {
  final text = n.text?.text ?? '';
  return text.endsWith('\n') ? text.substring(0, text.length - 1) : text;
}

/// The threads of the comments of a slide, in the order they were made.
List<CommentThread> slideThreads(Tree tree, String slide) {
  final threads = <String, CommentThread>{};
  final replies = <DocComment>[];
  for (final n in tree.children(slide)) {
    if (n.type != 'comment') continue;
    final c = _comment(n);
    if (c.parent != null && tree[c.parent!]?.type == 'comment') {
      replies.add(c);
    } else {
      threads[c.id] = CommentThread(c);
    }
  }
  for (final c in replies) {
    var root = c.parent;
    while (root != null && threads[root] == null) {
      root = tree[root]?.attributes['parent'] as String?;
    }
    (root == null ? threads.putIfAbsent(c.id, () => CommentThread(c)) : threads[root]!).replies.add(c);
  }
  return threads.values.toList();
}

/// Where a thread's balloon sits on the slide, in points.
Offset threadPlace(CommentThread t) {
  final a = t.root.node.attributes;
  final x = a['x'], y = a['y'];
  return Offset((x is num ? x : 0) * _commentUnit, (y is num ? y : 0) * _commentUnit);
}

/// Who writes a comment and when.
typedef CommentAuthor = ({String name, DateTime date});

String _date(DateTime d) => '${d.toUtc().toIso8601String().substring(0, 19)}Z';

/// A comment on a slide, saying [text]; an answer when [parent] is given,
/// and then where the comment it answers sits.
Edit addComment(Tree tree, String slide, String text, CommentAuthor by, {String? parent}) {
  final kids = tree.children(slide).where((n) => n.type == 'comment').toList();
  final from = parent == null ? null : tree[parent];
  final started = kids.where((n) => n.attributes['parent'] == null).length;
  final step = 12.0 * (started % 12 + 1);
  int unit(Object? known, double points) => known is num ? known.round() : (points / _commentUnit).round();
  final node = Node(
    id: randomId(),
    type: 'comment',
    parent: slide,
    key: keyBetween(kids.isEmpty ? 'zzz' : kids.last.key, ''),
    attributes: {
      'author': by.name,
      'initials': initialsOf(by.name),
      'date': _date(by.date),
      'x': unit(from?.attributes['x'], step),
      'y': unit(from?.attributes['y'], step),
      'parent': ?parent,
    },
    text: Delta()..insert('${text.trim()}\n'),
  );
  return Edit([Change.create(node)]);
}

/// Makes a comment say [text] instead.
Edit editComment(DocComment comment, String text) {
  final delta = Delta()
    ..insert(text.trim())
    ..delete(comment.text.length);
  return Edit([Change.text(comment.id, delta)]);
}

/// Deletes a comment, or the whole thread when [comment] starts it.
Edit deleteComment(CommentThread thread, DocComment comment) =>
    Edit([for (final c in comment.id == thread.id ? thread.all : [comment]) Change.delete(c.id)]);
