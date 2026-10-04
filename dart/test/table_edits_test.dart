import 'dart:convert';
import 'dart:io';
import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
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
