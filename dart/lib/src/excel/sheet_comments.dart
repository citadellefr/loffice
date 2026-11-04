import 'dart:math' as math;

import 'package:trame/trame.dart';

import '../chrome/comments.dart';

/// The fields a cell keeps its comments in: a note, or a thread.
const commentFields = ['note', 'thread'];

/// A note or a thread of comments on a cell, as the pane of comments
/// lists them. A note is a thread of one comment, which cannot be
/// answered or resolved, whoever edits its text.
class CellThread extends CommentThread {
  CellThread(this.row, this.col, super.root, {this.note = false});

  final int row, col;
  final bool note;

  @override
  bool get resolvable => !note;

  @override
  bool get repliable => !note;

  @override
  bool editableBy(DocComment comment, String me) => note || super.editableBy(comment, me);
}

/// The name of the comment [i] of a thread, for those the file gave none.
String _commentId(int row, int col, int i, Map<String, Object?> item) => item['id'] as String? ?? 'c$row-$col-$i';

DocComment _comment(String id, Map<String, Object?> item) => DocComment(Node(id: id, type: 'comment', key: '', attributes: item), '${item['text'] ?? ''}');

/// The note of a cell, as the file gives it.
Map<String, Object?>? noteOf(Map<String, Object?>? fields) => fields?['note'] is Map<String, Object?> ? fields!['note']! as Map<String, Object?> : null;

/// The thread of a cell, as the file gives it: its comments, the first
/// one answered by the others.
List<Map<String, Object?>> threadOf(Map<String, Object?>? fields) {
  final t = fields?['thread'];
  return [
    if (t is List<Object?>)
      for (final i in t)
        if (i is Map<String, Object?>) i,
  ];
}

/// Whether a cell has a note or a thread.
bool hasComments(Map<String, Object?>? fields) => noteOf(fields) != null || threadOf(fields).isNotEmpty;

/// The note or the thread of a cell, if any.
CellThread? threadAt(Map<String, Object?>? fields, int row, int col) {
  final items = threadOf(fields);
  if (items.isNotEmpty) {
    final thread = CellThread(row, col, _comment(_commentId(row, col, 0, items.first), items.first));
    for (var i = 1; i < items.length; i++) {
      thread.replies.add(_comment(_commentId(row, col, i, items[i]), items[i]));
    }
    return thread;
  }
  final note = noteOf(fields);
  return note == null ? null : CellThread(row, col, _comment('note$row-$col', note), note: true);
}

/// The notes and threads of a sheet, row by row.
List<CellThread> sheetThreads(Node sheet) {
  final cells = [
    for (final c in sheet.grid?.cells ?? const <Cell>[])
      if (c.row > 0 && c.col > 0 && hasComments(c.fields)) c,
  ]..sort((a, b) => a.row != b.row ? a.row.compareTo(b.row) : a.col.compareTo(b.col));
  return [for (final c in cells) threadAt(c.fields, c.row, c.col)!];
}

String _stamp(DateTime d) => '${d.toUtc().toIso8601String().substring(0, 19)}Z';

final _random = math.Random.secure();

/// A GUID in braces and capitals, as Excel names the comments of a thread.
String _guid() {
  final id = [for (var i = 0; i < 32; i++) _random.nextInt(16).toRadixString(16)].join().toUpperCase();
  return '{${id.substring(0, 8)}-${id.substring(8, 12)}-${id.substring(12, 16)}-${id.substring(16, 20)}-${id.substring(20)}}';
}

/// The edits of the notes and threads of a sheet: each sends the whole
/// field of a cell, as the hub takes it.
class CommentEdits {
  const CommentEdits(this.sheet, this.author);

  final Node sheet;

  /// The name of the user, who signs what is added.
  final String author;

  Edit _set(int row, int col, Map<String, Object?> fields) => Edit([
    Change.cells(sheet.id, [Cell(row, col, fields)]),
  ]);

  Map<String, Object?> _fields(int row, int col) => sheet.grid?.cell(row, col) ?? const {};

  /// A thread on a cell, made of a comment saying [text].
  Edit startThread(int row, int col, String text, DateTime now) => _set(row, col, {
    'thread': [
      {'id': _guid(), 'author': author, 'date': _stamp(now), 'text': text},
    ],
  });

  /// A note on a cell saying [text].
  Edit startNote(int row, int col, String text) => _set(row, col, {
    'note': {'author': author, 'text': text},
  });

  /// An answer to the thread of a cell.
  Edit reply(CellThread t, String text, DateTime now) => _set(t.row, t.col, {
    'thread': [
      ...threadOf(_fields(t.row, t.col)),
      {'id': _guid(), 'author': author, 'date': _stamp(now), 'text': text},
    ],
  });

  /// A comment of a thread, or a note, saying [text] instead.
  Edit say(CellThread t, DocComment comment, String text) {
    final fields = _fields(t.row, t.col);
    if (t.note) {
      return _set(t.row, t.col, {
        'note': {...?noteOf(fields), 'text': text},
      });
    }
    return _set(t.row, t.col, {
      'thread': [
        for (final (i, item) in threadOf(fields).indexed)
          if (_commentId(t.row, t.col, i, item) == comment.id) {...item, 'text': text} else item,
      ],
    });
  }

  /// A thread resolved, or opened again.
  Edit resolve(CellThread t, bool done) {
    final items = threadOf(_fields(t.row, t.col));
    final first = {...items.first}..remove('done');
    if (done) first['done'] = true;
    return _set(t.row, t.col, {
      'thread': [first, ...items.skip(1)],
    });
  }

  /// A comment deleted, the whole thread when it is the first, or a note.
  Edit delete(CellThread t, DocComment comment) {
    if (t.note || comment.id == t.id) return discard(t);
    return _set(t.row, t.col, {
      'thread': [
        for (final (i, item) in threadOf(_fields(t.row, t.col)).indexed)
          if (_commentId(t.row, t.col, i, item) != comment.id) item,
      ],
    });
  }

  /// A note or a thread deleted.
  Edit discard(CellThread t) => _set(t.row, t.col, {'note': null, 'thread': null});

  /// The notes and threads of the cells of an area deleted.
  Edit clear(int top, int left, int bottom, int right) {
    final cells = [
      for (final c in sheet.grid?.rows(top, bottom) ?? const <Cell>[])
        if (c.row > 0 && c.col >= left && c.col <= right && hasComments(c.fields)) Cell(c.row, c.col, const {'note': null, 'thread': null}),
    ];
    return cells.isEmpty ? Edit() : Edit([Change.cells(sheet.id, cells)]);
  }
}
