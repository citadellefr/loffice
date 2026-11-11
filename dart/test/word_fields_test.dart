import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/word/blocks.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/fields.dart';
import 'package:loffice/src/word/layout.dart';
import 'package:loffice/src/word/paragraph.dart';
import 'package:loffice/src/word/revisions.dart';

import 'word_layout_test.dart' show document, text;

const _object = '￼';
const _styles = {
  'Heading1': {'type': 'paragraph', 'name': 'heading 1'},
  'Heading2': {'type': 'paragraph', 'name': 'heading 2'},
};
const _strings = LofficeStrings();

Delta field(String instructions, String result, {Attributes? mark}) => Delta()
  ..insert(_object, {'fld': 'begin'})
  ..insert(_object, {'instr': instructions})
  ..insert(_object, {'fld': 'separate'})
  ..insert(result)
  ..insert(_object, {'fld': 'end'});

Delta heading(String title, [String style = 'Heading1']) => Delta()
  ..insert(title)
  ..insert('\n', {'pstyle': style});

Delta paragraph(String s) => Delta()..insert('$s\n');

final pageBreak = Delta()
  ..insert(_object, {'br': 'page'})
  ..insert('\n');

WordDocument build(List<Node> body) => document(body, doc: {'styles': _styles});

WordLayout lay(WordDocument doc) => WordLayout(doc, WordContext(doc), ParaCache());

/// The document after [edit], read again from its tree.
WordDocument applied(WordDocument doc, Edit edit) {
  expect(doc.tree.apply(edit), isNotNull);
  return WordDocument(doc.tree);
}

String shown(WordDocument doc, String id) => doc.tree[id]!.text!.ops.where((op) => op.attributes?['del'] == null).map((op) => op.insert!).join().replaceAll(_object, '');

Edit update(WordDocument doc, {DateTime? now, bool Function(WordField)? where}) => updateFields(doc, lay(doc), _strings, now: now, where: where);

void main() {
  test('rewrites a table of contents entry whose title changed and whose page moved', () {
    var doc = build([
      text('toc', 'a', field(r'TOC \o "1-3"', 'Ancien titre\t1\nSuite\t1\n')..insert('\n')),
      text('h1', 'b', heading('Titre changé')),
      text('pb', 'c', pageBreak),
      text('h2', 'd', heading('Suite')),
    ]);
    doc = applied(doc, update(doc));
    expect(shown(doc, 'toc'), 'Titre changé\t1\nSuite\t2\n\n');
    expect(update(doc).changes, isEmpty);
    expect(fieldsOf(doc.tree['toc']!).single.name, 'TOC');
  });

  test('links the entries to the bookmarks of the headings, made when they have none', () {
    var doc = build([text('toc', 'a', field(r'TOC \o "1-2" \h', '')..insert('\n')), text('h1', 'b', heading('Premier')), text('h2', 'c', heading('Second', 'Heading2'))]);
    doc = applied(doc, update(doc));
    final toc = doc.tree['toc']!.text!;
    final links = {
      for (final op in toc.ops)
        if (op.attributes?['link'] != null) op.attributes!['link'],
    };
    expect(links, hasLength(2));
    for (final link in links) {
      final name = link!.substring(1);
      expect(name, startsWith('_Toc'));
      expect(doc.tree['h1']!.text!.ops.any((op) => op.attributes?['bm'] == name) || doc.tree['h2']!.text!.ops.any((op) => op.attributes?['bm'] == name), isTrue);
    }
    expect(toc.ops.any((op) => op.attributes?['instr']?.startsWith('PAGEREF _Toc') ?? false), isTrue);
    expect(toc.ops.any((op) => op.attributes?['pstyle'] == 'TOC2'), isTrue);
    expect(update(doc).changes, isEmpty);
  });

  test('numbers the sequences by identifier and a reference takes the text of its bookmark', () {
    Delta figure(String n, {String? mark}) {
      final d = Delta();
      if (mark != null) d.insert(_object, {'bm': mark});
      d.insert('Figure ');
      for (final op in field(r'SEQ Figure \* ARABIC', n).ops) {
        d.push(op);
      }
      if (mark != null) d.insert(_object, {'bme': mark});
      return d..insert('\n');
    }

    var doc = build([
      text('f1', 'a', figure('1')),
      text('f2', 'b', figure('1')),
      text('f3', 'c', figure('2', mark: 'cible')),
      text('t', 'd', field('SEQ Tableau', '9')..insert('\n')),
      text('r', 'e', field('REF cible', 'obsolète')..insert('\n')),
    ]);
    doc = applied(doc, update(doc));
    doc = applied(doc, update(doc));
    expect(shown(doc, 'f1'), 'Figure 1\n');
    expect(shown(doc, 'f2'), 'Figure 2\n');
    expect(shown(doc, 'f3'), 'Figure 3\n');
    expect(shown(doc, 't'), '1\n');
    expect(shown(doc, 'r'), 'Figure 3\n');
  });

  test('writes the date and time in the format asked for, in French', () {
    final now = DateTime(2026, 3, 3, 9, 5);
    var doc = build([
      text('a', 'a', field(r'DATE \@ "dd/MM/yyyy"', 'x')..insert('\n')),
      text('b', 'b', field(r'DATE \@ "d MMMM yyyy"', 'x')..insert('\n')),
      text('c', 'c', field(r'TIME \@ "dddd d MMMM yyyy HH:mm"', 'x')..insert('\n')),
    ]);
    doc = applied(doc, update(doc, now: now));
    expect(shown(doc, 'a'), '03/03/2026\n');
    expect(shown(doc, 'b'), '3 mars 2026\n');
    expect(shown(doc, 'c'), 'mardi 3 mars 2026 09:05\n');
  });

  test('tracks the rewrite of a field result as a deletion and an insertion signed by the author', () {
    final now = DateTime(2026, 3, 3);
    var doc = build([text('a', 'a', field(r'DATE \@ "yyyy"', 'x')..insert('\n'))]);
    final direct = update(doc, now: now);
    final tracked = trackEdit(doc.tree, direct, 'Alice', revisionDate(now));
    expect(tracked.changes, isNot(direct.changes));
    doc = applied(doc, tracked);
    final ops = doc.tree['a']!.text!.ops;
    expect(ops.where((op) => op.attributes?['del'] == 'Alice').map((op) => op.insert).join(), 'x');
    expect(ops.where((op) => op.attributes?['ins'] == 'Alice').map((op) => op.insert).join(), '2026');
    expect(shown(doc, 'a'), '2026\n');
  });

  test('keeps the result of a field it does not know', () {
    final doc = build([text('a', 'a', field(r'MERGEFIELD nom', '«nom»')..insert('\n'))]);
    expect(update(doc).changes, isEmpty);
  });

  test('says so when a reference has no bookmark', () {
    var doc = build([text('a', 'a', field(r'REF absent', 'gardé')..insert('\n'))]);
    doc = applied(doc, update(doc));
    expect(shown(doc, 'a'), '${_strings.referenceNotFound}\n');
  });

  test('updates only the fields asked for', () {
    var doc = build([text('a', 'a', field(r'DATE \@ "yyyy"', 'x')..insert('\n')), text('b', 'b', field(r'DATE \@ "yyyy"', 'x')..insert('\n'))]);
    doc = applied(doc, update(doc, now: DateTime(2026, 3, 3), where: (f) => f.flow == 'b'));
    expect(shown(doc, 'a'), 'x\n');
    expect(shown(doc, 'b'), '2026\n');
  });

  test('inserts a table of contents Go can read back, with an entry for each heading', () {
    var doc = build([text('intro', 'a', paragraph('Introduction')), text('h1', 'b', heading('Premier')), text('h2', 'c', heading('Détail', 'Heading2'))]);
    doc = applied(doc, insertToc(doc, lay(doc), _strings, doc.tree['intro']!, 0));
    final toc = fieldsOf(doc.tree['intro']!).single;
    expect(toc.instructions.trim(), r'TOC \o "1-3" \h');
    final body = shown(doc, 'intro');
    expect(body, contains('Premier'));
    expect(body, contains('Détail'));
    expect(body, endsWith('Introduction\n'));
    expect(update(doc).changes, isEmpty);
  });
}
