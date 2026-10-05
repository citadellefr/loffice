import 'package:trame/trame.dart';

import '../chrome/comments.dart';
import 'edits.dart';

/// A comment and the answers to it, with the place of its range in the
/// flows of the body.
class WordThread extends CommentThread {
  WordThread(super.root);

  /// Where the range starts and ends, the Objects excluded, and where the
  /// reference is: a flow and an offset.
  (String, int)? start, end, reference;

  /// Whether the text the comment was about is still there.
  @override
  bool get anchored => start != null || end != null || reference != null;
}

/// The anchors of the comments in the flows of the body, by the key that
/// holds them, "cs", "ce" or "comment": comment id to flow and offset.
Map<String, Map<String, (String, int)>> commentAnchors(List<Node> flows) {
  final out = {'cs': <String, (String, int)>{}, 'ce': <String, (String, int)>{}, 'comment': <String, (String, int)>{}};
  for (final flow in flows) {
    var at = 0;
    for (final op in flow.text!.ops) {
      final a = op.attributes;
      if (a != null) {
        for (final e in out.entries) {
          final id = a[e.key];
          if (id != null) e.value.putIfAbsent(id, () => (flow.id, at));
        }
      }
      at += op.insert!.length;
    }
  }
  return out;
}

/// The threads of the comments of a document, in the order of their ranges
/// in the body; those whose text is gone last.
List<WordThread> commentThreads(Tree tree) {
  final nodes = tree.children('doc').where((n) => n.type == 'comment').toList();
  if (nodes.isEmpty) return const [];
  final flows = flowsOf(tree);
  final anchors = commentAnchors(flows);
  final order = {for (final (i, f) in flows.indexed) f.id: i};
  final threads = <String, WordThread>{};
  final replies = <DocComment>[];
  for (final n in nodes) {
    final c = DocComment(n, _plain(tree, n.id));
    if (c.parent != null && tree[c.parent!]?.type == 'comment') {
      replies.add(c);
      continue;
    }
    final t = WordThread(c);
    final start = anchors['cs']![n.id];
    t
      ..start = start == null ? null : (start.$1, start.$2 + 1)
      ..end = anchors['ce']![n.id]
      ..reference = anchors['comment']![n.id];
    threads[n.id] = t;
  }
  for (final c in replies) {
    var root = c.parent;
    while (root != null && threads[root] == null) {
      root = tree[root]?.attributes['parent'] as String?;
    }
    (root == null ? threads.putIfAbsent(c.id, () => WordThread(c)) : threads[root]!).replies.add(c);
  }
  (int, int) place(WordThread t) {
    final p = t.start ?? t.end ?? t.reference;
    return p == null ? (1 << 30, 0) : (order[p.$1] ?? 0, p.$2);
  }

  return threads.values.toList()
    ..sort((a, b) {
      final (x, y) = (place(a), place(b));
      return x.$1 != y.$1 ? x.$1.compareTo(y.$1) : x.$2.compareTo(y.$2);
    });
}

String _plain(Tree tree, String id) => [
  for (final f in flowsOf(tree, id)) f.text!.text.replaceAll('￼', '').replaceAll('\v', '\n'),
].join().trimRight();

/// The ranges of the flows a thread's comment is about, in reading order.
List<(String, int, int)> threadRanges(WordThread t, List<Node> flows) {
  final start = t.start, end = t.end;
  if (start == null || end == null) return const [];
  final i = flows.indexWhere((f) => f.id == start.$1), j = flows.indexWhere((f) => f.id == end.$1);
  if (i < 0 || j < i) return const [];
  if (i == j) return end.$2 > start.$2 ? [(start.$1, start.$2, end.$2)] : const [];
  return [
    (start.$1, start.$2, flows[i].text!.length - 1),
    for (var k = i + 1; k < j; k++) (flows[k].id, 0, flows[k].text!.length - 1),
    (end.$1, 0, end.$2),
  ];
}

/// Who writes a comment, when, and the style of its paragraphs.
typedef CommentAuthor = ({String name, DateTime date, String? style});

/// A comment on the range from [start] to [end], saying [text]: the node,
/// its text and its anchors, the end's after the range.
Edit addComment(Tree tree, (String, int) start, (String, int) end, String text, CommentAuthor by) {
  final (node, changes) = _comment(tree, text, by);
  final id = node.id;
  final inserts = <String, List<(int, Attributes)>>{};
  inserts.putIfAbsent(start.$1, () => []).add((start.$2, {'cs': id}));
  inserts.putIfAbsent(end.$1, () => [])
    ..add((end.$2, {'ce': id}))
    ..add((end.$2, {'comment': id}));
  return Edit([...changes, ..._anchorChanges(tree, inserts)]);
}

/// An answer to a thread, its anchors beside those of the thread's comment.
Edit replyTo(Tree tree, WordThread thread, String text, CommentAuthor by) {
  final (node, changes) = _comment(tree, text, by, parent: thread.id);
  final id = node.id;
  final inserts = <String, List<(int, Attributes)>>{};
  final start = thread.start, after = thread.reference ?? thread.end;
  if (start != null) inserts.putIfAbsent(start.$1, () => []).add((start.$2, {'cs': id}));
  if (after != null) {
    final at = after.$2 + 1;
    inserts.putIfAbsent(after.$1, () => [])
      ..add((at, {'ce': id}))
      ..add((at, {'comment': id}));
  }
  return Edit([...changes, ..._anchorChanges(tree, inserts)]);
}

(Node, List<Change>) _comment(Tree tree, String text, CommentAuthor by, {String? parent}) {
  final kids = tree.children('doc');
  final node = Node(
    id: randomId(),
    type: 'comment',
    parent: 'doc',
    key: keyBetween(kids.isEmpty ? '' : kids.last.key, ''),
    attributes: {
      'author': by.name,
      'initials': initialsOf(by.name),
      'date': '${by.date.toUtc().toIso8601String().substring(0, 19)}Z',
      'parent': ?parent,
    },
  );
  final mark = by.style == null ? null : {'pstyle': by.style!};
  final flow = Delta();
  for (final line in text.trim().split('\n')) {
    if (line.isNotEmpty) flow.insert(line);
    flow.insert('\n', mark);
  }
  return (node, [Change.create(node), Change.create(Node(id: randomId(), type: 'text', parent: node.id, key: 'V', text: flow))]);
}

/// The changes inserting Objects at offsets of flows, in the order given
/// for the same offset.
List<Change> _anchorChanges(Tree tree, Map<String, List<(int, Attributes)>> inserts) => [
  for (final e in inserts.entries)
    if (tree[e.key] != null) Change.text(e.key, _insertAll(e.value)),
];

Delta _insertAll(List<(int, Attributes)> inserts) {
  final sorted = [...inserts.indexed]..sort((a, b) => a.$2.$1 != b.$2.$1 ? a.$2.$1.compareTo(b.$2.$1) : a.$1.compareTo(b.$1));
  final d = Delta();
  var at = 0;
  for (final (_, (offset, attrs)) in sorted) {
    d
      ..retain(offset - at)
      ..insert('￼', attrs);
    at = offset;
  }
  return d;
}

/// Resolves a thread, or opens it again: its comment and the answers.
Edit resolveThread(WordThread thread, bool done) =>
    Edit([for (final c in thread.all) Change.set(c.id, attributes: {'done': done})]);

/// Deletes comments, and their anchors in the flows of the body.
Edit deleteComments(Tree tree, Set<String> ids) {
  final changes = <Change>[for (final id in ids) Change.delete(id)];
  for (final flow in flowsOf(tree)) {
    final d = Delta();
    var kept = 0, changed = false;
    for (final op in flow.text!.ops) {
      final a = op.attributes;
      final anchor = a != null && (ids.contains(a['cs']) || ids.contains(a['ce']) || ids.contains(a['comment']));
      if (anchor) {
        d
          ..retain(kept)
          ..delete(op.insert!.length);
        kept = 0;
        changed = true;
      } else {
        kept += op.insert!.length;
      }
    }
    if (changed) changes.add(Change.text(flow.id, d.chop()));
  }
  return Edit(changes);
}

/// Makes a comment say [text] instead: the text changed as little as it
/// can, its objects and formatting kept.
Edit editComment(Tree tree, DocComment comment, String text) {
  final flow = flowsOf(tree, comment.id).firstOrNull;
  if (flow == null) return Edit();
  final whole = flow.text!.text;
  // the offsets in the flow of the characters shown, objects left out
  final shown = <int>[];
  for (var i = 0; i < whole.length - 1; i++) {
    if (whole[i] != '￼') shown.add(i);
  }
  final old = [for (final i in shown) whole[i] == '\v' ? '\n' : whole[i]].join();
  final d = diff(old, text.trimRight());
  final mark = WordEdits.paragraphLike(wordEditing(flow).markAt(0));
  final out = Delta();
  var at = 0, flowAt = 0;
  void moveTo(int shownIndex) {
    final target = shownIndex < shown.length ? shown[shownIndex] : whole.length - 1;
    out.retain(target - flowAt);
    flowAt = target;
  }

  for (final op in d.ops) {
    if (op.isRetain) {
      at += op.retain;
    } else if (op.isDelete) {
      for (var k = 0; k < op.delete; k++) {
        moveTo(at + k);
        out.delete(1);
        flowAt++;
      }
      at += op.delete;
    } else {
      moveTo(at);
      final attrs = wordEditing(flow).typingAttributes(flowAt);
      for (final (i, part) in op.insert!.split('\n').indexed) {
        if (i > 0) out.insert('\n', mark.isEmpty ? null : mark);
        if (part.isNotEmpty) out.insert(part, attrs.isEmpty ? null : attrs);
      }
    }
  }
  return Edit([Change.text(flow.id, out.chop())]);
}
