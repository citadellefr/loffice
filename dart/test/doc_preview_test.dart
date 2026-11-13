import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';

Tree load(String path) => Tree.fromEdit(Edit.fromJson(jsonDecode(File(path).readAsStringSync()))!)!;

Future<Size> preview(WidgetTester tester, Tree tree, DocKind kind, double width) async {
  await tester.pumpWidget(
    Directionality(
      textDirection: TextDirection.ltr,
      child: Align(
        alignment: Alignment.topLeft,
        child: SizedBox(width: width, child: DocPreview(tree: tree, kind: kind)),
      ),
    ),
  );
  expect(tester.takeException(), isNull);
  return tester.getSize(find.byType(DocPreview));
}

void main() {
  final trees = {
    DocKind.word: load('../testdata/docx/par-known-styles.json'),
    DocKind.deck: load('../testdata/pptx/shp-shapes.json'),
    DocKind.book: load('../testdata/xlsx/47737.json'),
  };

  for (final kind in DocKind.values) {
    testWidgets('$kind: the first page is as wide as the room it is given, its ratio kept', (tester) async {
      final narrow = await preview(tester, trees[kind]!, kind, 200);
      final wide = await preview(tester, trees[kind]!, kind, 400);
      expect(narrow.width, 200);
      expect(wide.width, 400);
      expect(narrow.height, greaterThan(0));
      expect(wide.height, closeTo(narrow.height * 2, 0.01));
    });

    testWidgets('$kind: an empty tree is a blank page', (tester) async {
      final size = await preview(tester, Tree(), kind, 300);
      expect(size.width, 300);
      expect(size.height, greaterThan(0));
    });

    testWidgets('$kind: the tree is left as it was', (tester) async {
      final tree = trees[kind]!;
      final before = [...tree.nodes];
      await preview(tester, tree, kind, 300);
      expect(tree.nodes, unorderedEquals(before));
    });
  }
}
