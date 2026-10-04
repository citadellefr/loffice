import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/word/blocks.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/layout.dart';
import 'package:loffice/src/word/paragraph.dart';
import 'package:trame/trame.dart';

/// A document of A4 pages with 72 pt margins, holding [body], a list of
/// nodes under "body" built from their type and text.
WordDocument document(List<Node> body, {Map<String, Object?> doc = const {}}) {
  final tree = Tree.fromEdit(Edit([
    Change.create(Node(id: 'doc', type: 'doc', key: 'a', attributes: {
      'sect': {'w': 11906, 'h': 16838, 'top': 1440, 'bottom': 1440, 'left': 1440, 'right': 1440, 'header': 720, 'footer': 720},
      'settings': {'tab': 720},
      'defaults': {'p': <String, String>{}, 'r': {'sz': '20'}},
      ...doc,
    })),
    Change.create(const Node(id: 'body', type: 'body', parent: 'doc', key: 'b')),
    for (final n in body) Change.create(n),
  ]))!;
  return WordDocument(tree);
}

Node text(String id, String key, Delta flow, {String parent = 'body'}) => Node(id: id, type: 'text', parent: parent, key: key, text: flow);

Delta plain(String s, [Attributes? mark]) {
  final d = Delta();
  for (final p in s.split('\n')) {
    if (p.isNotEmpty) d.insert(p);
    d.insert('\n', mark);
  }
  return d;
}

WordLayout lay(WordDocument doc) => WordLayout(doc, WordContext(doc), ParaCache());

void main() {
  test('places paragraphs in the text area, the space between them added', () {
    final doc = document([text('t', 'V', plain('Un\nDeux', {'sp.after': '200', 'sp.before': '100'}))]);
    final layout = lay(doc);
    expect(layout.pages, hasLength(1));
    final page = layout.pages.single;
    expect(page.size.width, closeTo(595.3, 0.1));
    expect(page.body.left, 72);
    final lines = page.lines;
    expect(lines, hasLength(2));
    // Word keeps the space before the first paragraph of the document
    expect(lines[0].origin, const Offset(72, 77));
    // 10 pt after the first, 5 pt before the second
    expect(lines[1].top, closeTo(lines[0].bottom + 15, 0.01));
  });

  test('a caret and a click find each other', () {
    final doc = document([text('t', 'V', plain('Bonjour le monde\nSecond'))]);
    final layout = lay(doc);
    for (final offset in [0, 3, 16, 17, 20, 23]) {
      final (page, caret) = layout.caret('t', offset)!;
      expect(page, 0);
      expect(layout.hit(page, caret.center + const Offset(0.1, 0)), ('t', offset), reason: 'offset $offset');
    }
    expect(layout.selection('t', 0, 20).map((e) => e.$1).toSet(), {0});
    expect(layout.lineRange('t', 20), (17, 23));
  });

  test('a page break starts a page, and the text after it goes there', () {
    final flow = Delta()
      ..insert('Avant')
      ..insert('￼', {'o': '<w:br w:type="page"/>', 'br': 'page'})
      ..insert('Après\nSuite\n');
    final layout = lay(document([text('t', 'V', flow)]));
    expect(layout.pages, hasLength(2));
    expect(layout.caret('t', 6)!.$1, 1);
    expect(layout.caret('t', 12)!.$1, 1);
    expect(layout.pages[1].lines.first.top, 72);
  });

  test('a page break the editor inserted breaks the page too', () {
    final flow = Delta()
      ..insert('Avant')
      ..insert('￼', {'br': 'page'})
      ..insert('Après\n');
    expect(lay(document([text('t', 'V', flow)])).pages, hasLength(2));
  });

  test('a page number the editor inserted reads the page', () {
    final doc = document([text('t', 'V', plain('Texte'))]);
    final source = ParaSource(
      flow: 't',
      start: 0,
      ops: [const Op.insert('1', {'field': 'PAGE'})],
      mark: const {},
      para: const {},
      runBase: const {'sz': '20'},
    );
    final ctx = WordContext(doc);
    final one = ParaBox.layout(source, ctx, 400, fields: const FieldValues(page: '1'));
    final many = ParaBox.layout(source, ctx, 400, fields: const FieldValues(page: '12345'));
    expect(many.caretAt(1).left, greaterThan(one.caretAt(1).left + 20));
  });

  test('puts a footnote at the bottom of the page of its reference', () {
    final flow = Delta()
      ..insert('Texte')
      ..insert('￼', {'o': '<w:footnoteReference w:id="1"/>', 'note': 'footnote:1'})
      ..insert('\n');
    final doc = document([
      text('t', 'V', flow),
      const Node(id: 'fn1', type: 'note', parent: 'doc', key: 'z', attributes: {'kind': 'footnote'}),
      text('n', 'V', Delta()..insert('￼', {'o': '<w:footnoteRef/>', 'note': 'ref'})..insert(' Source.\n'), parent: 'fn1'),
    ]);
    final layout = lay(doc);
    final page = layout.pages.single;
    expect(page.rules, hasLength(1));
    final (_, note) = layout.caret('n', 2)!;
    expect(note.bottom, closeTo(page.body.bottom, 1));
    expect(note.top, greaterThan(page.body.bottom - 30));
  });

  test('text flows onto the next pages, lines kept whole', () {
    final long = List.filled(200, 'Ligne').join('\n');
    final layout = lay(document([text('t', 'V', plain(long, {'widowControl': '1'}))]));
    expect(layout.pages.length, greaterThan(1));
    for (final page in layout.pages) {
      for (final l in page.lines) {
        expect(l.bottom, lessThanOrEqualTo(page.body.bottom + 0.01));
      }
    }
    // every paragraph drawn once
    expect(layout.pages.fold(0, (n, p) => n + p.lines.length), 200);
  });

  test('keeps a heading with the paragraph after it', () {
    final flow = Delta();
    for (var i = 0; i < 46; i++) {
      flow.insert('Ligne $i\n');
    }
    flow
      ..insert('Titre')
      ..insert('\n', {'keepNext': '1'})
      ..insert('Texte\n');
    final layout = lay(document([text('t', 'V', flow)]));
    final (titlePage, _) = layout.caret('t', flow.length - 12)!;
    final (textPage, _) = layout.caret('t', flow.length - 3)!;
    expect(titlePage, textPage);
  });

  test('lays out a table: its cells side by side, their text in them', () {
    final doc = document([
      text('a', 'F', plain('Avant')),
      Node(id: 'tbl', type: 'tbl', parent: 'body', key: 'V', attributes: {
        'grid': [2000, 3000],
        'borders': {'top': {'val': 'single', 'sz': 4}},
      }),
      const Node(id: 'r1', type: 'tr', parent: 'tbl', key: 'V'),
      const Node(id: 'c1', type: 'tc', parent: 'r1', key: 'F'),
      const Node(id: 'c2', type: 'tc', parent: 'r1', key: 'V'),
      text('x', 'V', plain('Gauche'), parent: 'c1'),
      text('y', 'V', plain('Droite\nDessous'), parent: 'c2'),
      text('b', 'k', plain('Après')),
    ]);
    final layout = lay(doc);
    final page = layout.pages.single;
    expect(page.cells, hasLength(2));
    expect(page.cells[1].rect.left - page.cells[0].rect.left, closeTo(100, 0.01));
    final (_, left) = layout.caret('x', 0)!;
    final (_, right) = layout.caret('y', 0)!;
    expect(right.left - left.left, closeTo(100, 0.5));
    final (_, after) = layout.caret('b', 0)!;
    expect(after.top, greaterThanOrEqualTo(page.cells[1].rect.bottom - 0.01));
  });

  test('numbers the paragraphs of a list', () {
    final doc = document(
      [text('t', 'V', plain('Un\nDeux\nTrois', {'num': '1', 'lvl': '0'}))],
      doc: {
        'numbering': {
          '1': {
            'abstract': '0',
            'levels': [
              {'start': 1, 'fmt': 'decimal', 'text': '%1.', 'p': {'ind.left': '1440', 'ind.first': '-720'}},
            ],
          },
        },
      },
    );
    final builder = BlockBuilder(WordContext(doc), ParaCache());
    final labels = [for (final (s, _) in builder.sources(doc.tree['t']!)) s.label?.text];
    expect(labels, ['1.', '2.', '3.']);
    final layout = lay(doc);
    // the number hangs at 36 pt, the text starts at the indent
    expect(layout.caret('t', 0)!.$2.left, closeTo(72 + 72, 0.5));
  });

  test('a paragraph laid out once is laid out again only when it changes', () {
    final cache = ParaCache();
    final doc = document([text('t', 'V', plain('Un\nDeux'))]);
    final a = WordLayout(doc, WordContext(doc), cache);
    final changed = document([text('t', 'V', plain('Un\nDeux !'))]);
    final b = WordLayout(changed, WordContext(changed), cache);
    expect(identical(a.pages.single.lines.first.box, b.pages.single.lines.first.box), isTrue);
    expect(identical(a.pages.single.lines.last.box, b.pages.single.lines.last.box), isFalse);
  });
}
