import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/drawing/color.dart';
import 'package:loffice/src/drawing/paint.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/text/text_frame.dart';
import 'package:trame/trame.dart';

Deck _fixture(String name) {
  final edit = Edit.fromJson(jsonDecode(File('../testdata/pptx/$name.json').readAsStringSync()))!;
  return Deck(Tree.fromEdit(edit)!);
}

void main() {
  const fixtures = ['shp-shapes', 'ph-populated-placeholders', 'txt-text', 'dml-fill', 'tbl-cell'];

  test('placeholders inherit where they stand and how their text looks', () {
    final deck = _fixture('ph-populated-placeholders');
    expect(deck.size, const Size(720, 540));
    final title = deck.tree['s257-2']!;
    final style = deck.styleOf(title);
    expect(style.placeholder, 'title');
    // the title has no position of its own: its layout's or master's
    expect(title.attributes['xfrm'], isNull);
    expect(style.bounds, isNotNull);
    expect(style.bounds!.width, greaterThan(100));
    // its text is the master's title style: 44 points, the major font
    final level = style.level(0);
    expect(level['sz'], '4400');
    expect(level['font'], '+mj-lt');

    final body = deck.styleOf(deck.tree['s258-2']!);
    expect(body.placeholder, 'body');
    expect(body.level(0)['bu'], startsWith('char:'));
    expect(body.level(1)['sz'], isNot(body.level(0)['sz']));

    final colors = deck.colorsOf(deck.slides.first);
    expect(colors.resolve({'scheme': 'tx1'}), const Color(0xFF000000));
    expect(deck.themeOf(deck.slides.first).typeface('+mj-lt'), 'Calibri');
  });

  test('lays out text: paragraphs, line breaks, offsets and carets', () {
    final deck = _fixture('txt-text');
    final painter = SlidePainter(deck);
    final box = deck.tree['s256-3']!;
    final frame = painter.textOf(box)!;
    // "abc\na\vb\vc\n": two paragraphs, the second on three lines
    expect(frame.paragraphs, hasLength(2));
    expect(frame.length, box.text!.length);
    final second = frame.paragraphs[1];
    expect(second.start, 4);
    expect(frame.caretAt(6).top, greaterThan(frame.caretAt(4).top));
    for (var i = 0; i < frame.length; i++) {
      final caret = frame.caretAt(i);
      expect(frame.offsetAt(caret.center + const Offset(0.1, 0)), i, reason: 'offset $i at $caret');
    }
    expect(frame.selection(0, 5), isNotEmpty);
  });

  test('lays out tables: rows grow with their text, cells take their style', () {
    final deck = _fixture('tbl-cell');
    final painter = SlidePainter(deck);
    final frame = deck.tree['s257-2']!;
    final table = painter.tableOf(frame)!;
    expect(table.cells, hasLength(16));
    expect(table.size.width, closeTo(4 * 1524000 / emuPerPoint, 1e-6));
    // "unladen swallows" takes two lines: more than the 29.2 points asked
    expect(table.rows[2] - table.rows[1], greaterThan(370840 / emuPerPoint + 10));
    expect(table.rows[3] - table.rows[2], closeTo(370840 / emuPerPoint, 0.5));

    Color? fill(String id) {
      final f = table.cells[id]!.fill;
      return f == null ? null : DrawingPainter(f.$2).color(f.$1);
    }

    // a header, then bands: the first darker than the second
    expect(fill('s257-2-r2-c1'), isNot(fill('s257-2-r3-c1')));
    expect(fill('s257-2-r1-c1'), fill('s257-2-r3-c1'));
    expect(table.cells['s257-2-r1-c1']!.lines.keys, containsAll(['l', 'r', 't', 'b']));

    final cell = deck.tree['s257-2-r2-c1']!;
    expect(painter.textOf(cell), same(table.cells[cell.id]!.text));
    final corner = painter.transformOf(frame).apply(Offset.zero);
    expect(painter.transformOf(cell).apply(Offset.zero), corner + Offset(0, table.rows[1]));

    deck.tree.apply(Edit([Change.text(cell.id, Delta([Op.insert('Encore '), Op.retain(cell.text!.length)]))]));
    final again = painter.tableOf(frame)!;
    expect(again, isNot(same(table)));
    expect(again.cells[cell.id]!.text.length, cell.text!.length + 7);
  });

  test('vertical text runs down the box, its lines from right to left', () {
    final flow = Delta([const Op.insert('Un texte assez long pour tenir sur plusieurs lignes\n')]);
    TextFrame layout(String vert) => TextFrame.layout(
      flow,
      box: const Rect.fromLTWH(0, 0, 60, 200),
      body: {'vert': vert},
      levels: (_) => const {'sz': '1200'},
      colors: const ColorContext(),
    );
    final down = layout('vert'), up = layout('vert270');
    // across the 200 points of the box: few lines, which stack leftward
    expect(down.box.width, closeTo(200 - 7.2, 1e-9));
    expect(down.box.height, closeTo(60 - 14.4, 1e-9));
    final first = down.caretAt(0), last = down.caretAt(flow.length - 1);
    expect(first.height, 0);
    expect(first.width, greaterThan(0));
    expect(first.right, closeTo(60 - 7.2, 1));
    expect(last.left, lessThan(first.left));
    expect(up.caretAt(0).left, closeTo(7.2, 1));
    for (final frame in [down, up]) {
      for (var i = 0; i < flow.length; i += 7) {
        final caret = frame.caretAt(i);
        final across = frame == down ? const Offset(0, 0.1) : const Offset(0, -0.1);
        expect(frame.offsetAt(caret.center + across), i, reason: 'offset $i at $caret');
      }
    }
  });

  test('a cell of vertical text is as high as its line is long', () {
    final deck = _fixture('tbl-cell');
    final painter = SlidePainter(deck);
    final frame = deck.tree['s257-2']!;
    final before = painter.tableOf(frame)!;
    final cell = deck.tree['s257-2-r2-c1']!;
    deck.tree.apply(Edit([Change.set(cell.id, attributes: {'vert': 'vert270'})]));
    final after = painter.tableOf(frame)!;
    final text = after.cells[cell.id]!.text;
    expect(after.rows[2] - after.rows[1], greaterThan(before.rows[2] - before.rows[1]));
    expect(text.caretAt(0).height, 0);
  });

  test('paints every slide of the fixtures', () {
    for (final name in fixtures) {
      final deck = _fixture(name);
      final painter = SlidePainter(deck);
      for (final slide in deck.slides) {
        final recorder = ui.PictureRecorder();
        painter.paint(Canvas(recorder), slide);
        recorder.endRecording().dispose();
      }
      for (final shape in deck.tree.nodes) {
        if (deck.pageOf(shape)?.type == 'slide' && shape.type != 'notes') {
          final t = painter.transformOf(shape);
          expect(t.invert().multiply(t).apply(const Offset(3, 4)).dx, closeTo(3, 1e-6), reason: shape.id);
        }
      }
    }
  });
}
