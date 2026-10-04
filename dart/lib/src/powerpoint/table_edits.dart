import 'dart:math' as math;
import 'dart:ui';

import 'package:trame/trame.dart';

import '../text/editing.dart';
import 'deck.dart';
import 'table.dart';

/// Office's default table style, which every deck holds.
const defaultTableStyle = '{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}';

/// The height PowerPoint gives a new row, in EMU.
const _rowHeight = 370840;

/// How narrow a column can be dragged, in EMU: its cells' margins.
const _narrowest = 182880;

/// The keys of a cell that place it in the grid, not copied with its look.
const _placing = {'xml', 'gridSpan', 'rowSpan', 'hMerge', 'vMerge'};

/// A new table of [rows] × [columns] filling [bounds]'s width, in the
/// deck's default style, with a header row and banded rows.
Edit newTable(Deck deck, Node slide, int rows, int columns, Rect bounds, {required String name, required String key}) {
  final styles = deck.tree['deck']?.attributes['tblStyles'];
  final def = deck.tree['deck']?.attributes['tblStyleDef'];
  final style = def is String && styles is Map<String, Object?> && styles.containsKey(def) ? def : defaultTableStyle;
  final width = (bounds.width * emuPerPoint / columns).round();
  final id = randomId();
  final changes = <Change>[
    Change.create(Node(id: id, type: 'frame', parent: slide.id, key: key, attributes: {
      'frame': 'table',
      'name': name,
      'xfrm': {
        'x': (bounds.left * emuPerPoint).round(),
        'y': (bounds.top * emuPerPoint).round(),
        'w': width * columns,
        'h': _rowHeight * rows,
      },
      'tbl': {'style': style, 'firstRow': true, 'bandRow': true},
      'grid': List.filled(columns, width),
    })),
  ];
  var rowKey = '';
  for (var r = 0; r < rows; r++) {
    final row = randomId();
    rowKey = keyBetween(rowKey, '');
    changes.add(Change.create(Node(id: row, type: 'tr', parent: id, key: rowKey, attributes: const {'h': _rowHeight})));
    var cellKey = '';
    for (var c = 0; c < columns; c++) {
      cellKey = keyBetween(cellKey, '');
      changes.add(Change.create(Node(id: randomId(), type: 'tc', parent: row, key: cellKey, text: Delta([const Op.insert('\n')]))));
    }
  }
  return Edit(changes);
}

/// The edits of the rows and columns of a table, as its layout places its
/// cells: merged cells grow and shrink with what is inserted or deleted
/// across them.
class TableEdits {
  TableEdits(this.tree, this.frame, this.layout);

  final Tree tree;
  final Node frame;
  final TableLayout layout;

  List<Node> get _rows => [for (final n in tree.children(frame.id)) if (n.type == 'tr') n];

  int get _columns => layout.slots.isEmpty ? 0 : layout.slots.first.length;

  List<Object?> get _grid => [...?(frame.attributes['grid'] as List<Object?>?)];

  /// A row at [at], 0 putting it first, formatted as the row it follows,
  /// or precedes when it comes first.
  Edit insertRow(int at) {
    final rows = _rows;
    if (rows.isEmpty) return Edit();
    final like = at > 0 ? at - 1 : 0;
    final id = randomId();
    final changes = <Change>[
      Change.create(Node(
        id: id,
        type: 'tr',
        parent: frame.id,
        key: keyBetween(at > 0 ? rows[at - 1].key : '', at < rows.length ? rows[at].key : ''),
        attributes: {'h': rows[like].attributes['h'] ?? _rowHeight},
      )),
    ];
    final grown = <String>{};
    var key = '';
    for (var c = 0; c < _columns; c++) {
      key = keyBetween(key, '');
      final over = at > 0 && at < rows.length ? layout.cellOver(at - 1, c) : null;
      if (over != null && over.row + over.rowSpan > at) {
        if (grown.add(over.node.id)) changes.addAll(_rowSpan(over, over.rowSpan + 1));
        changes.add(Change.create(Node(id: randomId(), type: 'tc', parent: id, key: key, attributes: {
          'vMerge': true,
          if (c > over.col) 'hMerge': true,
          if (c == over.col && over.colSpan > 1) 'gridSpan': over.colSpan,
        }, text: Delta([const Op.insert('\n')]))));
      } else {
        changes.add(_emptyLike(layout.slots[like][c], id, key));
      }
    }
    return Edit(changes);
  }

  /// A column at [at], 0 putting it first, as wide and formatted as the
  /// column it follows, or precedes when it comes first.
  Edit insertColumn(int at) {
    final cols = _columns;
    if (cols == 0) return Edit();
    final like = at > 0 ? at - 1 : 0;
    final grid = _grid;
    final width = like < grid.length && grid[like] is num ? grid[like]! as num : 914400;
    grid.insert(at.clamp(0, grid.length), width);
    final changes = <Change>[Change.set(frame.id, attributes: {'grid': grid, 'xfrm': _wider(width)})];
    final rows = _rows;
    for (var r = 0; r < rows.length; r++) {
      final slot = layout.slots[r];
      final kids = tree.children(rows[r].id);
      final before = _last(slot, at);
      final i = before == null ? -1 : kids.indexOf(before);
      final key = keyBetween(i >= 0 ? kids[i].key : '', i + 1 < kids.length ? kids[i + 1].key : '');
      final origin = at > 0 && at < cols ? _origin(slot, at) : null;
      if (origin != null && _start(slot, origin) < at && at < _start(slot, origin) + _span(origin)) {
        final rowSpan = layout.cells[origin.id]?.rowSpan ?? 1;
        changes.add(Change.set(origin.id, attributes: {'gridSpan': _span(origin) + 1}));
        changes.add(Change.create(Node(id: randomId(), type: 'tc', parent: rows[r].id, key: key, attributes: {
          'hMerge': true,
          if (origin.attributes['vMerge'] == true) 'vMerge': true,
          if (rowSpan > 1) 'rowSpan': rowSpan,
        }, text: Delta([const Op.insert('\n')]))));
      } else {
        changes.add(_emptyLike(slot[like], rows[r].id, key));
      }
    }
    return Edit(changes);
  }

  /// Deletes a row, or the table with its last row. A cell merged down
  /// from it hands its text to the row below.
  Edit deleteRow(int at) {
    final rows = _rows;
    if (rows.length <= 1) return deleteTable();
    final changes = <Change>[Change.delete(rows[at].id)];
    for (final cell in layout.cells.values) {
      if (cell.rowSpan == 1 || at < cell.row || at >= cell.row + cell.rowSpan) continue;
      final span = cell.rowSpan - 1;
      if (cell.row < at) {
        changes.addAll(_rowSpan(cell, span));
        continue;
      }
      // the merged cell starts in the row deleted: its text moves down
      final below = layout.slots[at + 1];
      final heir = below[cell.col];
      if (heir == null) continue;
      changes
        ..add(Change.delete(heir.id))
        ..add(Change.create(Node(
          id: randomId(),
          type: 'tc',
          parent: heir.parent,
          key: heir.key,
          attributes: {...cell.node.attributes, 'rowSpan': span > 1 ? span : null}..removeWhere((_, v) => v == null),
          text: cell.node.text,
        )));
      for (var c = cell.col + 1; c < cell.col + cell.colSpan; c++) {
        final covered = below[c];
        if (covered != null) changes.add(Change.set(covered.id, attributes: {'vMerge': null, 'rowSpan': span > 1 ? span : null}));
      }
    }
    return Edit(changes);
  }

  /// Deletes a column, or the table with its last column. A cell merged
  /// across it loses a column of its span.
  Edit deleteColumn(int at) {
    if (_columns <= 1) return deleteTable();
    final grid = _grid;
    final width = at < grid.length && grid[at] is num ? grid[at]! as num : 0;
    if (at < grid.length) grid.removeAt(at);
    final changes = <Change>[Change.set(frame.id, attributes: {'grid': grid, 'xfrm': _wider(-width)})];
    for (final slot in layout.slots) {
      final origin = _origin(slot, at);
      if (origin == null) continue;
      final span = _span(origin);
      if (span == 1) {
        changes.add(Change.delete(origin.id));
        continue;
      }
      changes.add(Change.set(origin.id, attributes: {'gridSpan': span > 2 ? span - 1 : null}));
      final start = _start(slot, origin);
      for (var c = start + span - 1; c > start; c--) {
        if (slot[c] != null) {
          changes.add(Change.delete(slot[c]!.id));
          break;
        }
      }
    }
    return Edit(changes);
  }

  Edit deleteTable() => Edit([Change.delete(frame.id)]);

  /// Merges [cells], a block as [TableLayout.range] gives, into its first:
  /// it takes the text of the others, paragraph after paragraph, and the
  /// others are written as covered by it, as PowerPoint writes them.
  Edit merge(List<CellLayout> cells) {
    if (cells.length < 2) return Edit();
    final first = cells.first;
    final top = first.row, left = first.col;
    final rows = cells.map((c) => c.row + c.rowSpan).reduce(math.max) - top;
    final cols = cells.map((c) => c.col + c.colSpan).reduce(math.max) - left;
    final changes = <Change>[];
    final texts = [for (final c in cells.skip(1)) if (c.node.text case final t? when t.length > 1) t];
    if (texts.isNotEmpty) changes.add(Change.text(first.node.id, _appended(first.node.text ?? Delta([const Op.insert('\n')]), texts)));
    changes.add(Change.set(first.node.id, attributes: {
      'gridSpan': cols > 1 ? cols : null,
      'rowSpan': rows > 1 ? rows : null,
      'hMerge': null,
      'vMerge': null,
    }));
    for (var r = top; r < top + rows; r++) {
      for (var c = left; c < left + cols; c++) {
        final n = layout.slots[r][c];
        if (n == null || n.id == first.node.id) continue;
        final length = n.text?.length ?? 1;
        if (length > 1) changes.add(Change.text(n.id, Delta([Op.delete(length - 1)])));
        changes.add(Change.set(n.id, attributes: {
          'hMerge': c > left ? true : null,
          'vMerge': r > top ? true : null,
          'gridSpan': c == left && r > top && cols > 1 ? cols : null,
          'rowSpan': r == top && rows > 1 ? rows : null,
        }));
      }
    }
    return Edit(changes);
  }

  /// Splits a merged cell back into the cells it covers, its text left in
  /// the first; those its span covered without a node are created empty,
  /// formatted as it.
  Edit split(CellLayout cell) {
    if (cell.rowSpan == 1 && cell.colSpan == 1) return Edit();
    const loose = {'gridSpan': null, 'rowSpan': null, 'hMerge': null, 'vMerge': null};
    final changes = <Change>[Change.set(cell.node.id, attributes: loose)];
    final rows = _rows;
    for (var r = cell.row; r < cell.row + cell.rowSpan; r++) {
      final slot = layout.slots[r];
      final kids = tree.children(rows[r].id);
      var key = '';
      for (var c = 0; c < slot.length; c++) {
        final n = slot[c];
        if (n != null) {
          key = n.key;
          if (n.id != cell.node.id && c >= cell.col && c < cell.col + cell.colSpan) changes.add(Change.set(n.id, attributes: loose));
          continue;
        }
        if (c < cell.col || c >= cell.col + cell.colSpan) continue;
        final i = kids.indexWhere((k) => k.key.compareTo(key) > 0);
        key = keyBetween(key, i < 0 ? '' : kids[i].key);
        changes.add(_emptyLike(cell.node, rows[r].id, key));
      }
    }
    return Edit(changes);
  }

  /// Moves the edge after column [edge] - 1 by [by] points: the columns on
  /// each side of an inner edge share their width, the last edge widens
  /// the table.
  Edit resizeColumn(int edge, double by) {
    final grid = _grid;
    if (edge < 1 || edge > grid.length || grid.any((w) => w is! num)) return Edit();
    final widths = [for (final w in grid) (w! as num).round()];
    final last = edge == widths.length;
    var d = (by * emuPerPoint).round();
    d = math.max(d, math.min(0, _narrowest - widths[edge - 1]));
    if (!last) d = math.min(d, math.max(0, widths[edge] - _narrowest));
    if (d == 0) return Edit();
    widths[edge - 1] += d;
    if (!last) widths[edge] -= d;
    return Edit([Change.set(frame.id, attributes: {'grid': widths, if (last) 'xfrm': _wider(d)})]);
  }

  /// Moves the edge under row [edge] - 1 by [by] points, the row no lower
  /// than its text needs; the table grows or shrinks with it.
  Edit resizeRow(int edge, double by) {
    final rows = _rows;
    if (edge < 1 || edge > rows.length) return Edit();
    final now = layout.rows[edge] - layout.rows[edge - 1];
    final height = math.max(now + by, layout.fits[edge - 1]);
    if ((height - now).abs() < 0.01) return Edit();
    final x = {...?(frame.attributes['xfrm'] as Map<String, Object?>?)};
    x['h'] = ((layout.rows.last + height - now) * emuPerPoint).round();
    return Edit([
      Change.set(rows[edge - 1].id, attributes: {'h': (height * emuPerPoint).round()}),
      Change.set(frame.id, attributes: {'xfrm': x}),
    ]);
  }

  /// Empties the text of [cells], keeping the formatting of their marks.
  Edit clear(List<CellLayout> cells) => Edit([
    for (final c in cells)
      if (c.node.text case final t? when t.length > 1) Change.text(c.node.id, Delta([Op.delete(t.length - 1)])),
  ]);

  /// Turns an option of the table style on or off: "firstRow", "bandRow"…
  Edit toggle(String option) {
    final tbl = {...?(frame.attributes['tbl'] as Map<String, Object?>?)};
    if (tbl[option] == true) {
      tbl.remove(option);
    } else {
      tbl[option] = true;
    }
    return Edit([Change.set(frame.id, attributes: {'tbl': tbl})]);
  }

  /// Sets how many rows a merged cell spans, on it and on the cells its
  /// span covers in its first row, as PowerPoint writes them.
  List<Change> _rowSpan(CellLayout cell, int span) => [
    for (var c = cell.col; c < cell.col + cell.colSpan; c++)
      if (layout.slots[cell.row][c] case final n?) Change.set(n.id, attributes: {'rowSpan': span > 1 ? span : null}),
  ];

  /// The delta appending the paragraphs of [texts] to [into], each after
  /// a new mark formatted as the last of [into].
  static Delta _appended(Delta into, List<Delta> texts) {
    final out = Delta()..retain(into.length - 1);
    final mark = FlowEditing(into).markAt(into.length - 1);
    var empty = into.length == 1;
    for (final t in texts) {
      if (!empty) out.insert('\n', mark.isEmpty ? null : mark);
      empty = false;
      final ops = t.ops;
      for (var i = 0; i < ops.length; i++) {
        final text = i == ops.length - 1 ? ops[i].insert!.substring(0, ops[i].insert!.length - 1) : ops[i].insert!;
        if (text.isNotEmpty) out.insert(text, ops[i].attributes);
      }
    }
    return out;
  }

  /// The frame's place, [by] EMU wider.
  Map<String, Object?> _wider(num by) {
    final x = {...?(frame.attributes['xfrm'] as Map<String, Object?>?)};
    x['w'] = math.max(0, ((x['w'] as num? ?? 0) + by).round());
    return x;
  }

  /// An empty cell formatted as [like], without its merges.
  Change _emptyLike(Node? like, String row, String key) {
    final mark = like?.text?.ops.last.attributes;
    return Change.create(Node(
      id: randomId(),
      type: 'tc',
      parent: row,
      key: key,
      attributes: {
        for (final e in (like?.attributes ?? const <String, Object?>{}).entries)
          if (!_placing.contains(e.key)) e.key: e.value,
      },
      text: Delta([Op.insert('\n', mark)]),
    ));
  }

  /// The cell of a row each column belongs to: the one there, or the one
  /// whose span covers it.
  List<Node?> _owners(List<Node?> slot) {
    final out = <Node?>[];
    Node? owner;
    var left = 0;
    for (final n in slot) {
      if (left > 0 && (n == null || n.attributes['hMerge'] == true)) {
        left--;
        out.add(owner);
        continue;
      }
      owner = n;
      left = n == null ? 0 : _span(n) - 1;
      out.add(n);
    }
    return out;
  }

  Node? _origin(List<Node?> slot, int col) => _owners(slot)[col];

  int _start(List<Node?> slot, Node origin) => _owners(slot).indexOf(origin);

  int _span(Node n) => n.attributes['gridSpan'] is num ? (n.attributes['gridSpan']! as num).toInt() : 1;

  /// The last cell node of a row before a column.
  Node? _last(List<Node?> slot, int col) {
    for (var c = col - 1; c >= 0; c--) {
      if (slot[c] != null) return slot[c];
    }
    return null;
  }
}
