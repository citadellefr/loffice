import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:trame/trame.dart';

import '../text/editing.dart';
import 'document.dart';
import 'layout.dart';
import 'revisions.dart';

/// The keys of the flows of Word documents: text typed takes no revision
/// from the text around it.
const wordKeys = FlowKeys(paragraph: wordParagraphKeys, own: wordOwnKeys, objects: {...wordObjectKeys, ...revisionKeys});

/// Edits of the flows of a Word document.
FlowEditing wordEditing(Node text) => FlowEditing(text.text!, keys: wordKeys);

/// Where the caret and selection are: a range of one flow, or from a
/// place of a flow to one of a later or earlier flow; and the page it was
/// put on, for a flow drawn on several, a header's.
class WordSelection extends ChangeNotifier {
  /// The flow of the caret, the extent's.
  String? flow;

  /// The flow the selection started in: [flow] but when it spans several.
  String? baseFlow;
  var base = 0;
  var extent = 0;

  /// The page the selection was made on, and in which part of it.
  int? page;
  var area = PageArea.body;

  /// The run attributes text typed next takes, set at a caret by the
  /// buttons of the ribbon.
  Attributes? typing;

  /// Whether the selection goes from a flow to another.
  bool get spans => baseFlow != null && baseFlow != flow;

  bool get collapsed => !spans && base == extent;

  /// The start and end in the flow of the caret, when it is the only one.
  int get start => base < extent ? base : extent;
  int get end => base < extent ? extent : base;

  void set(String flow, int base, [int? extent, int? page, PageArea? area]) {
    final same = flow == this.flow && !spans && base == this.base && (extent ?? base) == this.extent;
    this.flow = flow;
    baseFlow = flow;
    this.base = base;
    this.extent = extent ?? base;
    if (page != null) this.page = page;
    if (area != null) this.area = area;
    if (!same) typing = null;
    notifyListeners();
  }

  /// Moves the extent, the start staying where it is: to another flow
  /// when [flow] differs.
  void extendTo(String flow, int extent, [int? page]) {
    this.flow = flow;
    this.extent = extent;
    if (page != null) this.page = page;
    typing = null;
    notifyListeners();
  }

  void clear() {
    flow = baseFlow = null;
    base = extent = 0;
    typing = null;
    notifyListeners();
  }

  /// The ranges of the flows the selection covers, in reading order: the
  /// flows of [order] between its two ends, whole in between.
  List<(Node, int, int)> ranges(Tree tree, List<Node> order) {
    final a = baseFlow == null ? null : tree[baseFlow!];
    final b = flow == null ? null : tree[flow!];
    if (a == null || b == null) return const [];
    if (!spans) return [(b, start, end)];
    var ia = order.indexWhere((n) => n.id == a.id), ib = order.indexWhere((n) => n.id == b.id);
    if (ia < 0 || ib < 0) return [(b, extent, extent)];
    var (from, to) = (base, extent);
    if (ia > ib) (ia, ib, from, to) = (ib, ia, extent, base);
    return [
      for (var i = ia; i <= ib; i++)
        (order[i], i == ia ? from : 0, i == ib ? to : order[i].text!.length - 1),
    ];
  }
}

/// The text nodes of the body in reading order, those of tables and content
/// controls included; of a header or footer when [root] is one.
List<Node> flowsOf(Tree tree, [String root = 'body']) {
  final out = <Node>[];
  void walk(String parent) {
    for (final n in tree.children(parent)) {
      if (n.type == 'text') {
        out.add(n);
      } else {
        walk(n.id);
      }
    }
  }

  walk(root);
  return out;
}

/// The edits the Word editor makes on the tree of a document.
class WordEdits {
  WordEdits(this.doc);

  final WordDocument doc;

  Tree get tree => doc.tree;

  /// A key after [after] among the children of [parent], before the next.
  String keyAfter(String parent, Node? after) {
    final kids = tree.children(parent);
    if (after == null) return keyBetween(kids.isEmpty ? '' : kids.last.key, '');
    final i = kids.indexWhere((n) => n.id == after.id);
    final next = i + 1 < kids.length ? kids[i + 1].key : '';
    return keyBetween(after.key, next == after.key ? '' : next);
  }

  /// The paragraph keys a new paragraph takes from [mark]: its style and
  /// formatting, not its section or its id.
  static Attributes paragraphLike(Attributes mark) => {
    for (final e in mark.entries)
      if (wordParagraphKeys.contains(e.key) && !wordOwnKeys.contains(e.key)) e.key: e.value,
  };

  /// A table of [rows] × [columns] after the paragraph holding [offset] of
  /// [text], its columns sharing [width] points: the flow is cut there, the
  /// paragraphs after it going to a new flow after the table.
  Edit insertTable(Node text, int offset, int rows, int columns, double width) {
    final editing = wordEditing(text);
    final (_, mark) = editing.paragraphAt(offset);
    final flow = text.text!;
    final changes = <Change>[];
    final after = slice(flow, mark + 1, flow.length);
    if (after.length > 0) {
      // the last mark of a flow stays: it takes the place of the paragraph's
      final last = flow.length - 1;
      final own = editing.attributesAt(mark) ?? const <String, String>{};
      final old = editing.attributesAt(last) ?? const <String, String>{};
      changes.add(Change.text(
        text.id,
        (Delta()
              ..retain(mark)
              ..delete(last - mark)
              ..retain(1, {for (final k in {...own.keys, ...old.keys}) if (own[k] != old[k]) k: own[k] ?? ''}))
            .chop(),
      ));
    }
    final table = randomId();
    final col = (width * 20 / columns).floor();
    final style = doc.styles.containsKey('TableGrid') ? 'TableGrid' : null;
    const line = {'val': 'single', 'sz': 4, 'space': 0, 'color': 'auto'};
    final parent = text.parent;
    final tableKey = keyAfter(parent, text);
    changes.add(Change.create(Node(
      id: table,
      type: 'tbl',
      parent: parent,
      key: tableKey,
      attributes: {
        'style': ?style,
        'w': {'w': 0, 'type': 'auto'},
        'grid': List.filled(columns, col),
        'look': {'firstRow': true, 'lastRow': false, 'firstColumn': true, 'lastColumn': false, 'noHBand': false, 'noVBand': true},
        if (style == null) 'borders': {for (final side in ['top', 'left', 'bottom', 'right', 'insideH', 'insideV']) side: line},
      },
    )));
    final cellMark = {
      if (doc.styles.containsKey(doc.paragraphStyle)) 'pstyle': ?doc.paragraphStyle,
    };
    var rowKey = '';
    for (var r = 0; r < rows; r++) {
      final row = randomId();
      rowKey = keyBetween(rowKey, '');
      changes.add(Change.create(Node(id: row, type: 'tr', parent: table, key: rowKey)));
      var cellKey = '';
      for (var c = 0; c < columns; c++) {
        final cell = randomId();
        cellKey = keyBetween(cellKey, '');
        changes.add(Change.create(Node(id: cell, type: 'tc', parent: row, key: cellKey, attributes: {'w': {'w': col, 'type': 'dxa'}})));
        changes.add(Change.create(Node(id: randomId(), type: 'text', parent: cell, key: 'V', text: Delta()..insert('\n', cellMark.isEmpty ? null : cellMark))));
      }
    }
    // Word wants a paragraph after a table: the rest of the flow, or an
    // empty one like the paragraph the table follows
    final rest = after.length > 0 ? after : (Delta()..insert('\n', paragraphLike(editing.markAt(offset))));
    changes.add(Change.create(Node(id: randomId(), type: 'text', parent: parent, key: keyBetween(tableKey, _nextKey(parent, text)), text: rest)));
    return Edit(changes);
  }

  String _nextKey(String parent, Node after) {
    final kids = tree.children(parent);
    final i = kids.indexWhere((n) => n.id == after.id);
    return i + 1 < kids.length ? kids[i + 1].key : '';
  }

  /// A row like [row], above or below it, its cells empty.
  Edit insertRow(Node row, {required bool below}) {
    final table = row.parent;
    final rows = tree.children(table);
    final i = rows.indexWhere((n) => n.id == row.id);
    final key = below
        ? keyBetween(row.key, i + 1 < rows.length ? rows[i + 1].key : '')
        : keyBetween(i > 0 ? rows[i - 1].key : '', row.key);
    final id = randomId();
    final changes = <Change>[Change.create(Node(id: id, type: 'tr', parent: table, key: key, attributes: _without(row.attributes, const {'xml'})))];
    var cellKey = '';
    for (final cell in tree.children(row.id).where((c) => c.type == 'tc')) {
      cellKey = keyBetween(cellKey, '');
      changes.addAll(_emptyCell(cell, id, cellKey));
    }
    return Edit(changes);
  }

  /// A column like the one [cell] starts, left or right of it, in every
  /// row.
  Edit insertColumn(Node cell, {required bool right}) {
    final row = tree[cell.parent]!;
    final table = tree[row.parent]!;
    final at = _gridColumn(cell);
    final changes = <Change>[];
    for (final r in tree.children(table.id).where((n) => n.type == 'tr')) {
      final cells = tree.children(r.id).where((c) => c.type == 'tc').toList();
      Node? target;
      var col = 0;
      for (final c in cells) {
        final span = _span(c);
        if (at >= col && at < col + span) target = c;
        col += span;
      }
      target ??= cells.lastOrNull;
      if (target == null) continue;
      final kids = tree.children(r.id);
      final i = kids.indexWhere((n) => n.id == target!.id);
      final key = right
          ? keyBetween(target.key, i + 1 < kids.length ? kids[i + 1].key : '')
          : keyBetween(i > 0 ? kids[i - 1].key : '', target.key);
      changes.addAll(_emptyCell(target, r.id, key));
    }
    final grid = [...(table.attributes['grid'] as List<Object?>? ?? const [])];
    if (at < grid.length) {
      grid.insert(right ? at + 1 : at, grid[at]);
      changes.add(Change.set(table.id, attributes: {'grid': grid}));
    }
    return Edit(changes);
  }

  /// Deletes the row of a cell, or the table with its last row.
  Edit deleteRow(Node cell) {
    final row = tree[cell.parent]!;
    final rows = tree.children(row.parent).where((n) => n.type == 'tr');
    if (rows.length <= 1) return deleteTable(tree[row.parent]!);
    return Edit([Change.delete(row.id)]);
  }

  /// Deletes the column a cell starts in every row, or the table with its
  /// last column.
  Edit deleteColumn(Node cell) {
    final row = tree[cell.parent]!;
    final table = tree[row.parent]!;
    final at = _gridColumn(cell);
    final changes = <Change>[];
    var empty = true;
    for (final r in tree.children(table.id).where((n) => n.type == 'tr')) {
      var col = 0;
      final cells = tree.children(r.id).where((c) => c.type == 'tc').toList();
      for (final c in cells) {
        final span = _span(c);
        if (at >= col && at < col + span) {
          changes.add(Change.delete(c.id));
        } else {
          empty = false;
        }
        col += span;
      }
    }
    if (empty) return deleteTable(table);
    final grid = [...(table.attributes['grid'] as List<Object?>? ?? const [])];
    if (at < grid.length) {
      grid.removeAt(at);
      changes.add(Change.set(table.id, attributes: {'grid': grid}));
    }
    return Edit(changes);
  }

  /// Deletes a table; the flows around it stay apart.
  Edit deleteTable(Node table) => Edit([Change.delete(table.id)]);

  List<Change> _emptyCell(Node like, String row, String key) {
    final id = randomId();
    final first = tree.children(like.id).where((n) => n.type == 'text').firstOrNull;
    final mark = first == null ? const <String, String>{} : paragraphLike(wordEditing(first).markAt(0));
    return [
      Change.create(Node(id: id, type: 'tc', parent: row, key: key, attributes: _without(like.attributes, const {'xml', 'vMerge'}))),
      Change.create(Node(id: randomId(), type: 'text', parent: id, key: 'V', text: Delta()..insert('\n', mark.isEmpty ? null : mark))),
    ];
  }

  int _span(Node cell) => ((cell.attributes['span'] as num?) ?? 1).toInt();

  /// The grid column a cell starts at.
  int _gridColumn(Node cell) {
    final row = tree[cell.parent]!;
    var col = ((row.attributes['before'] as num?) ?? 0).toInt();
    for (final c in tree.children(row.id).where((c) => c.type == 'tc')) {
      if (c.id == cell.id) return col;
      col += _span(c);
    }
    return col;
  }

  /// Sets the page layout of every section: size, orientation, margins,
  /// columns; [change] maps a section's JSON to the new one.
  Edit pageSetup(Map<String, Object?> Function(Map<String, Object?> section) change) {
    final changes = <Change>[];
    final d = tree['doc'];
    if (d != null) changes.add(Change.set('doc', attributes: {'sect': change(doc.lastSection.json)}));
    for (final text in flowsOf(tree)) {
      final marks = <(int, String)>[];
      var pos = 0;
      for (final op in text.text!.ops) {
        final s = op.insert!;
        final sect = op.attributes?['sect'];
        if (sect != null) {
          for (var i = s.indexOf('\n'); i >= 0; i = s.indexOf('\n', i + 1)) {
            marks.add((pos + i, sect));
          }
        }
        pos += s.length;
      }
      if (marks.isEmpty) continue;
      final delta = Delta();
      var at = 0;
      for (final (k, sect) in marks) {
        delta
          ..retain(k - at)
          ..retain(1, {'sect': jsonEncode(change(jsonObject(sect) ?? const {}))});
        at = k + 1;
      }
      changes.add(Change.text(text.id, delta.chop()));
    }
    return Edit(changes);
  }
}

/// Deletes what a selection across flows covers: the end of the first,
/// the start of the last, the text of the flows between and the blocks
/// between them. When the two ends share a parent, the rest of the last
/// joins the first, as Word joins the paragraphs; they stay apart when one
/// is in a table and the other not.
Edit deleteRanges(Tree tree, List<(Node, int, int)> ranges) {
  if (ranges.isEmpty) return Edit();
  final changes = <Change>[];
  final (first, from, _) = ranges.first;
  final (last, _, to) = ranges.last;
  final join = first.id != last.id && first.parent == last.parent;
  final between = <String>{};
  if (join) {
    final kids = tree.children(first.parent);
    final a = kids.indexWhere((n) => n.id == first.id), b = kids.indexWhere((n) => n.id == last.id);
    for (var i = a + 1; i < b; i++) {
      between.add(kids[i].id);
      changes.add(Change.delete(kids[i].id));
    }
    changes
      ..add(Change.text(first.id, _joined(first, from, last, to)))
      ..add(Change.delete(last.id));
  }
  for (final (flow, start, end) in ranges) {
    if (join && (flow.id == first.id || flow.id == last.id)) continue;
    if (between.any((id) => tree.isUnder(flow.id, id))) continue;
    final d = wordEditing(flow).delete(start, end);
    if (!d.isEmpty) changes.add(Change.text(flow.id, d));
  }
  return Edit(changes);
}

/// [first] up to [from], then [last] from [to]: the paragraph where they
/// meet keeps the formatting of the first, and the last mark of [first]
/// becomes that of [last].
Delta _joined(Node first, int from, Node last, int to) {
  final a = wordEditing(first), b = wordEditing(last);
  final end = a.text.length - 1, lastEnd = b.text.length - 1;
  final (_, meet) = b.paragraphAt(to);
  final merged = {
    for (final e in (b.attributesAt(meet) ?? const <String, String>{}).entries)
      if (!wordParagraphKeys.contains(e.key) || wordOwnKeys.contains(e.key)) e.key: e.value,
    ...WordEdits.paragraphLike(a.markAt(from)),
  };
  final d = Delta()
    ..retain(from)
    ..delete(end - from);
  for (final op in slice(last.text!, to, meet).ops) {
    d.insert(op.insert!, op.attributes);
  }
  if (meet < lastEnd) {
    d.insert('\n', merged.isEmpty ? null : merged);
    for (final op in slice(last.text!, meet + 1, lastEnd).ops) {
      d.insert(op.insert!, op.attributes);
    }
  }
  final old = a.attributesAt(end) ?? const <String, String>{};
  final target = meet < lastEnd ? b.attributesAt(lastEnd) ?? const <String, String>{} : merged;
  d.retain(1, {
    for (final k in {...old.keys, ...target.keys})
      if (old[k] != target[k]) k: target[k] ?? '',
  });
  return d.chop();
}

/// Where [query] is in the flows, in reading order.
List<(Node, int, int)> findIn(List<Node> flows, String query, {bool matchCase = false}) {
  if (query.isEmpty) return const [];
  final needle = matchCase ? query : query.toLowerCase();
  final out = <(Node, int, int)>[];
  for (final flow in flows) {
    var text = flow.text!.text;
    if (!matchCase) text = text.toLowerCase();
    for (var i = text.indexOf(needle); i >= 0; i = text.indexOf(needle, i + needle.length)) {
      out.add((flow, i, i + needle.length));
    }
  }
  return out;
}

/// Replaces every match by [text], typed with the formatting of the first
/// character it replaces: one edit, undone at once.
Edit replaceAll(List<(Node, int, int)> matches, String text) {
  final byFlow = <String, List<(int, int)>>{};
  final nodes = <String, Node>{};
  for (final (node, a, b) in matches) {
    (byFlow[node.id] ??= []).add((a, b));
    nodes[node.id] = node;
  }
  final changes = <Change>[];
  for (final e in byFlow.entries) {
    final editing = wordEditing(nodes[e.key]!);
    final d = Delta();
    var at = 0;
    for (final (a, b) in e.value..sort((x, y) => x.$1.compareTo(y.$1))) {
      d
        ..retain(a - at)
        ..delete(b - a);
      if (text.isNotEmpty) {
        final attributes = editing.typingAttributes(a + 1);
        d.insert(text, attributes.isEmpty ? null : attributes);
      }
      at = b;
    }
    changes.add(Change.text(e.key, d.chop()));
  }
  return Edit(changes);
}

Map<String, Object?> _without(Map<String, Object?> attrs, Set<String> keys) => {
  for (final e in attrs.entries)
    if (!keys.contains(e.key)) e.key: e.value,
};
