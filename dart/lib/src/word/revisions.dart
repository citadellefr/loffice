import 'package:trame/trame.dart';

/// The keys of tracked changes: who inserted or deleted text, and when.
const revisionKeys = {'ins', 'insd', 'del', 'deld'};

/// The date of a revision made at [now], as Word writes it: in UTC, to the
/// minute, so that what is typed in a minute is one revision.
String revisionDate(DateTime now) {
  final t = now.toUtc();
  return DateTime.utc(t.year, t.month, t.day, t.hour, t.minute).toIso8601String().replaceFirst('.000Z', 'Z');
}

/// A change of a flow as Word makes it while tracking changes, and where
/// the offsets of the flow the change would have made went.
class Tracked {
  Tracked._(this.delta, this._segments);

  final Delta delta;

  /// Runs of the flow made: their length had the change not been
  /// tracked, and their length as it is; 0 for text struck.
  final List<(int, int)> _segments;

  /// Where [at], an offset in the flow the change would have made, is in
  /// the flow it makes: before the text struck there, or [after] it.
  int offset(int at, {bool after = true}) {
    var u = 0, t = 0;
    for (final (du, dt) in _segments) {
      if (du == 0) {
        if (u == at && !after) return t;
        t += dt;
        continue;
      }
      if (at < u + du) return t + at - u;
      u += du;
      t += dt;
    }
    return t + at - u;
  }
}

/// [change] of [flow] tracked as [author]'s: what it inserts is an
/// insertion, what it deletes stays, struck, but for the author's own
/// insertions, which go. Text deleted and typed over is struck before
/// the text typed, as Word shows it. Nothing joins: the paragraph marks
/// a deletion would have rewritten keep their formatting.
Tracked track(Delta flow, Delta change, String author, String date) {
  final out = Delta();
  final segments = <(int, int)>[];
  var at = 0;
  var deleted = false;
  void strike(int n) {
    for (final op in slice(flow, at, at + n).ops) {
      final a = op.attributes ?? const <String, String>{};
      final length = op.insert!.length;
      if (a['del'] != null) {
        out.retain(length);
        segments.add((0, length));
      } else if (a['ins'] == author) {
        out.delete(length);
      } else {
        out.retain(length, {'del': author, 'deld': date});
        segments.add((0, length));
      }
    }
    at += n;
    deleted = true;
  }

  final ops = change.ops;
  for (var i = 0; i < ops.length; i++) {
    final op = ops[i];
    if (op.isInsert) {
      if (i + 1 < ops.length && ops[i + 1].isDelete) strike(ops[++i].delete);
      out.insert(op.insert!, {
        for (final e in (op.attributes ?? const <String, String>{}).entries)
          if (e.key != 'del' && e.key != 'deld') e.key: e.value,
        'ins': author,
        'insd': date,
      });
      segments.add((op.length, op.length));
    } else if (op.isDelete) {
      strike(op.delete);
    } else {
      out.retain(op.retain, deleted ? null : op.attributes);
      segments.add((op.retain, op.retain));
      at += op.retain;
    }
  }
  return Tracked._(out.chop(), segments);
}

/// The changes of the flows [edit] makes tracked as [author]'s.
Edit trackEdit(Tree tree, Edit edit, String author, String date) => Edit([
  for (final c in edit.changes)
    if (c.kind == ChangeKind.text && tree[c.id]?.text != null) Change.text(c.id, track(tree[c.id]!.text!, c.text!, author, date).delta) else c,
]);

/// What a selection across flows covers struck as [author]'s deletion:
/// nothing joins, nothing is removed but the author's own insertions.
Edit trackDeletion(List<(Node, int, int)> ranges, String author, String date) => Edit([
  for (final (flow, start, end) in ranges)
    if (end > start) Change.text(flow.id, track(flow.text!, Delta()..retain(start)..delete(end - start), author, date).delta),
]);

/// A tracked change: text of a flow an author inserted or deleted.
class WordRevision {
  const WordRevision(this.flow, this.start, this.end, {required this.deleted, required this.author, this.date});

  final String flow;
  final int start, end;
  final bool deleted;
  final String author;
  final DateTime? date;
}

/// The tracked changes of [flows] in reading order, neighbours of the same
/// kind, author and date made one.
List<WordRevision> revisionsOf(List<Node> flows) {
  final out = <WordRevision>[];
  for (final flow in flows) {
    var at = 0;
    for (final op in flow.text!.ops) {
      final a = op.attributes ?? const <String, String>{};
      final length = op.insert!.length;
      final deleted = a['del'] != null;
      final author = a['del'] ?? a['ins'];
      if (author != null) {
        final date = a[deleted ? 'deld' : 'insd'];
        final last = out.lastOrNull;
        if (last != null && last.flow == flow.id && last.end == at && last.deleted == deleted && last.author == author &&
            last.date == DateTime.tryParse(date ?? '')) {
          out.last = WordRevision(flow.id, last.start, at + length, deleted: deleted, author: author, date: last.date);
        } else {
          out.add(WordRevision(flow.id, at, at + length, deleted: deleted, author: author, date: DateTime.tryParse(date ?? '')));
        }
      }
      at += length;
    }
  }
  return out;
}

/// Accepts or rejects the tracked changes over ranges of flows: an
/// insertion accepted or a deletion rejected stays, unmarked; an insertion
/// rejected or a deletion accepted goes, and so does text both inserted
/// and deleted. The last mark of a flow cannot go: it is only unmarked.
Edit settle(List<(Node, int, int)> ranges, {required bool accept}) {
  const unmarked = {'ins': '', 'insd': '', 'del': '', 'deld': ''};
  final changes = <Change>[];
  for (final (flow, start, end) in ranges) {
    final last = flow.text!.length - 1;
    final d = Delta()..retain(start);
    var at = start;
    for (final op in slice(flow.text!, start, end).ops) {
      final a = op.attributes ?? const <String, String>{};
      final length = op.insert!.length;
      final inserted = a['ins'] != null, deleted = a['del'] != null;
      final goes = deleted && (accept || inserted) || inserted && !accept;
      if (!inserted && !deleted) {
        d.retain(length);
      } else if (!goes) {
        d.retain(length, unmarked);
      } else if (at + length > last) {
        d
          ..delete(last - at)
          ..retain(1, unmarked);
      } else {
        d.delete(length);
      }
      at += length;
    }
    final delta = d.chop();
    if (!delta.isEmpty) changes.add(Change.text(flow.id, delta));
  }
  return Edit(changes);
}

/// The flow from [start] to [end], as inserts.
Delta slice(Delta flow, int start, int end) {
  final out = Delta();
  var at = 0;
  for (final op in flow.ops) {
    final s = op.insert!;
    final a = (start - at).clamp(0, s.length), b = (end - at).clamp(0, s.length);
    if (b > a) out.insert(s.substring(a, b), op.attributes);
    at += s.length;
  }
  return out;
}
