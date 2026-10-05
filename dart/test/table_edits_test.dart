import 'dart:convert';
import 'dart:io';
import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/builtin_styles.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/powerpoint/table.dart';
import 'package:loffice/src/powerpoint/table_edits.dart';
import 'package:trame/trame.dart';

void main() {
  late Tree tree;

  setUp(() {
    tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/tbl-cell.json').readAsStringSync()))!)!;
  });

  // s256-2 is 3 × 3, its first cell merged over two columns and two rows
  TableLayout layout() => SlidePainter(Deck(tree)).tableOf(tree['s256-2']!)!;

  void edit(Edit Function(TableEdits e) make) {
    final e = make(TableEdits(tree, tree['s256-2']!, layout()));
    expect(tree.apply(e), isNotNull, reason: '$e');
  }

  CellLayout merged(TableLayout t) => t.cells.values.firstWhere((c) => c.node.text!.text == 'merged cell\n');

  test('a built-in style the presentation lacks is drawn as Office draws it', () {
    final ids = [for (final (_, ids) in builtinTableStyleFamilies) ...ids.where((id) => id.isNotEmpty)];
    expect(ids, hasLength(74));
    expect(ids.every(builtinTableStyles.containsKey), isTrue);

    // Medium Style 2 - Accent 2
    const id = '{21E4AEA4-8DFA-4A89-87EB-49C32662AFE0}';
    expect(tree['deck']!.attributes['tblStyles'], isNot(contains(id)));
    edit((e) => e.restyle(id));
    final tbl = tree['s256-2']!.attributes['tbl']! as Map<String, Object?>;
    expect(tbl['style'], id);
    final fill = layout().cells.values.last.fill!.$1;
    expect(jsonEncode(fill), contains('accent2'));
  });

  test('a row inserted across a merged cell lengthens it', () {
    edit((e) => e.insertRow(1));
    final t = layout();
    expect(t.rows, hasLength(5));
    expect(merged(t).rowSpan, 3);
    expect(merged(t).colSpan, 2);
    expect(t.cells, hasLength(7));
    expect(t.slots.every((s) => s.length == 3 && s.every((n) => n != null)), isTrue);
  });

  test('a row inserted at the end is like the last one', () {
    edit((e) => e.insertRow(3));
    final t = layout();
    expect(t.rows, hasLength(5));
    expect(t.cells, hasLength(9));
    expect(merged(t).rowSpan, 2);
  });

  test('a column inserted across a merged cell widens it, the table too', () {
    final before = (tree['s256-2']!.attributes['xfrm']! as Map<String, Object?>)['w']! as num;
    edit((e) => e.insertColumn(1));
    final t = layout();
    expect(t.columns, hasLength(5));
    expect(merged(t).colSpan, 3);
    expect(merged(t).rowSpan, 2);
    expect(t.cells, hasLength(7));
    expect((tree['s256-2']!.attributes['xfrm']! as Map<String, Object?>)['w'], before + 2032000);
  });

  test('a column inserted first is like the first one', () {
    edit((e) => e.insertColumn(0));
    final t = layout();
    expect(t.columns, hasLength(5));
    expect(merged(t).col, 1);
    expect(t.cells, hasLength(9));
  });

  test('deleting the first row of a merged cell hands its text down', () {
    edit((e) => e.deleteRow(0));
    final t = layout();
    expect(t.rows, hasLength(3));
    expect(merged(t).rowSpan, 1);
    expect(merged(t).colSpan, 2);
    expect(t.cells, hasLength(5));
  });

  test('deleting a column of a merged cell narrows it', () {
    edit((e) => e.deleteColumn(0));
    final t = layout();
    expect(t.columns, hasLength(3));
    expect(merged(t).colSpan, 1);
    expect(merged(t).rowSpan, 2);
    expect(t.cells, hasLength(5));
  });

  test('deleting the last row or column deletes the table', () {
    for (var i = 0; i < 2; i++) {
      edit((e) => e.deleteRow(0));
    }
    edit((e) => e.deleteRow(0));
    expect(tree['s256-2'], isNull);
  });

  test('a range grows to hold the merged cells it cuts', () {
    final t = layout();
    final corner = t.cellOver(2, 2)!, below = t.cellOver(2, 0)!;
    expect(t.range(below, corner), hasLength(3));
    final right = t.cellOver(0, 2)!;
    expect(t.range(below, right).map((c) => c.node.id).toSet(), t.cells.keys.toSet());
  });

  test('cells merged take the text of the others, split back they are empty', () {
    var t = layout();
    final row = t.range(t.cellOver(2, 0)!, t.cellOver(2, 2)!);
    final texts = [for (final c in row) c.node.text!.text];
    edit((e) => e.merge(row));
    t = layout();
    final all = t.cellOver(2, 1)!;
    expect(all.colSpan, 3);
    expect(all.node.id, row.first.node.id);
    expect(all.node.text!.text, '${texts.map((x) => x.substring(0, x.length - 1)).where((x) => x.isNotEmpty).join('\n')}\n');
    expect(t.cells, hasLength(4));

    edit((e) => e.split(all));
    t = layout();
    expect(t.cells, hasLength(6));
    expect(t.cellOver(2, 0)!.node.text!.text, all.node.text!.text);
    expect(t.cellOver(2, 2)!.node.text!.text, '\n');
  });

  test('a cell split into columns and rows shares them, the others merged across', () {
    final cell = layout().cellOver(0, 2)!;
    final grid = [...tree['s256-2']!.attributes['grid']! as List<Object?>];
    edit((e) => e.splitInto(cell, 2, 2));
    final t = layout();
    expect(t.slots, hasLength(4));
    expect(t.slots.every((s) => s.length == 4), isTrue);
    expect(t.cells, hasLength(9));
    final m = merged(t);
    expect((m.rowSpan, m.colSpan), (3, 2));
    for (final (r, c) in [(0, 2), (0, 3), (1, 2), (1, 3)]) {
      expect((t.cellOver(r, c)!.rowSpan, t.cellOver(r, c)!.colSpan), (1, 1));
    }
    expect(t.cellOver(0, 2)!.node.id, cell.node.id);
    expect(t.cellOver(2, 2)!.colSpan, 2);
    expect(t.cellOver(3, 2)!.colSpan, 2);
    final widths = tree['s256-2']!.attributes['grid']! as List<Object?>;
    expect((widths[2]! as num) + (widths[3]! as num), grid[2]);
    final trs = tree.children('s256-2');
    expect(trs[1].attributes['h'], trs[0].attributes['h']);
  });

  test('a merged cell split creates the cells its span covered without one', () {
    final cell = merged(layout());
    final row = tree[cell.node.parent]!;
    for (final n in tree.children(row.id).where((n) => n.attributes['hMerge'] == true).toList()) {
      expect(tree.apply(Edit([Change.delete(n.id)])), isNotNull);
    }
    expect(layout().slots.first.where((n) => n == null), hasLength(1));
    edit((e) => e.split(merged(layout())));
    final t = layout();
    expect(t.cells, hasLength(9));
    expect(t.slots.every((s) => s.every((n) => n != null)), isTrue);
  });

  test('an inner edge shares the width of its columns, the last widens the table', () {
    final grid = [...tree['s256-2']!.attributes['grid']! as List<Object?>];
    final width = (tree['s256-2']!.attributes['xfrm']! as Map<String, Object?>)['w']! as num;
    edit((e) => e.resizeColumn(1, 10));
    var now = tree['s256-2']!.attributes['grid']! as List<Object?>;
    expect(now[0], (grid[0]! as num) + 127000);
    expect(now[1], (grid[1]! as num) - 127000);
    edit((e) => e.resizeColumn(3, -1000));
    now = tree['s256-2']!.attributes['grid']! as List<Object?>;
    expect(now[2], 182880);
    expect((tree['s256-2']!.attributes['xfrm']! as Map<String, Object?>)['w'], width - ((grid[2]! as num) - 182880));
  });

  test('a row no lower than its text', () {
    final before = layout();
    edit((e) => e.resizeRow(3, 20));
    var t = layout();
    expect(t.rows[3] - t.rows[2], closeTo(before.rows[3] - before.rows[2] + 20, 0.01));
    expect((tree['s256-2']!.attributes['xfrm']! as Map<String, Object?>)['h'], ((before.rows.last + 20) * emuPerPoint).round());
    edit((e) => e.resizeRow(3, -1000));
    t = layout();
    expect(t.rows[3] - t.rows[2], closeTo(t.fits[2], 0.01));
  });

  test('borders on the edges named, and across the outer ones', () {
    const pen = {'w': 25400};
    var t = layout();
    final corner = t.cellOver(2, 2)!;
    edit((e) => e.borders([corner], 'outside', pen));
    t = layout();
    Map<String, Object?> at(int r, int c) => t.cellOver(r, c)!.node.attributes;
    expect([for (final k in ['lnT', 'lnB', 'lnL', 'lnR']) at(2, 2)[k]], everyElement(pen));
    expect(at(1, 2)['lnB'], pen);
    expect(at(2, 1)['lnR'], pen);

    final block = t.range(t.cellOver(1, 2)!, t.cellOver(2, 2)!);
    edit((e) => e.borders(block, 'insideH', pen));
    expect(layout().cellOver(1, 2)!.node.attributes['lnL'], isNull);

    edit((e) => e.borders(block, 'none', pen));
    t = layout();
    expect(at(2, 2)['lnT'], {'fill': {'none': true}});
    expect(at(2, 1)['lnR'], {'fill': {'none': true}});
  });

  test('cells cleared keep their marks', () {
    final t = layout();
    edit((e) => e.clear(t.cells.values.toList()));
    expect(layout().cells.values.every((c) => c.node.text!.text == '\n'), isTrue);
  });

  test('a new table in the default style, the options toggled', () {
    final deck = Deck(tree);
    final slide = deck.slides.first;
    final e = newTable(deck, slide, 3, 4, const Rect.fromLTWH(60, 100, 600, 90), name: 'Tableau 9', key: 'zz');
    expect(tree.apply(e), isNotNull);
    final frame = tree[e.changes.first.id]!;
    expect((frame.attributes['tbl']! as Map<String, Object?>)['style'], defaultTableStyle);
    var t = SlidePainter(Deck(tree)).tableOf(frame)!;
    expect(t.cells, hasLength(12));
    expect(t.size.width, closeTo(600, 0.01));

    expect(tree.apply(TableEdits(tree, frame, t).toggle('firstRow')), isNotNull);
    expect((tree[frame.id]!.attributes['tbl']! as Map<String, Object?>)['firstRow'], isNull);
    t = SlidePainter(Deck(tree)).tableOf(tree[frame.id]!)!;
    expect(tree.apply(TableEdits(tree, tree[frame.id]!, t).toggle('bandCol')), isNotNull);
    expect((tree[frame.id]!.attributes['tbl']! as Map<String, Object?>)['bandCol'], isTrue);
  });
}
