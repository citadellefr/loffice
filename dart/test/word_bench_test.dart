import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/word/blocks.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/layout.dart';
import 'package:loffice/src/word/paragraph.dart';
import 'package:trame/trame.dart';

/// How long laying out a long document takes, at first and after a
/// keystroke: a measure, not a test, run when $LOFFICE_BENCH is set.
void main() {
  test('lays out 2 000 paragraphs, then again after a keystroke', skip: Platform.environment['LOFFICE_BENCH'] == null ? 'LOFFICE_BENCH not set' : false, () {
    const words = 'Le chiffre d’affaires du trimestre progresse nettement grâce aux nouveaux clients et aux services rendus';
    Tree build(String extra) {
      final flow = Delta();
      for (var i = 0; i < 2000; i++) {
        flow.insert('$words $i${i == 1000 ? extra : ''}');
        flow.insert('\n', {'sp.after': '160', 'widowControl': '1'});
      }
      return Tree.fromEdit(Edit([
        Change.create(const Node(id: 'doc', type: 'doc', key: 'a', attributes: {'defaults': {'r': {'sz': '22'}}})),
        Change.create(const Node(id: 'body', type: 'body', parent: 'doc', key: 'b')),
        Change.create(Node(id: 't', type: 'text', parent: 'body', key: 'V', text: flow)),
      ]))!;
    }

    final cache = ParaCache();
    var watch = Stopwatch()..start();
    var doc = WordDocument(build(''));
    final first = WordLayout(doc, WordContext(doc), cache);
    final initial = watch.elapsedMilliseconds;
    var total = 0;
    for (var k = 0; k < 5; k++) {
      doc = WordDocument(build('x' * (k + 1)), previous: doc);
      watch = Stopwatch()..start();
      WordLayout(doc, WordContext(doc), cache);
      total += watch.elapsedMilliseconds;
    }
    // ignore: avoid_print
    print('${first.pages.length} pages: first layout $initial ms, after a keystroke ${total / 5} ms');
  });
}
