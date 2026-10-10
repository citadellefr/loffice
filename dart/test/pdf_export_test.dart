import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/loffice.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/export.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/word/blocks.dart';
import 'package:loffice/src/word/document.dart';
import 'package:loffice/src/word/export.dart';
import 'package:loffice/src/word/layout.dart';
import 'package:loffice/src/word/page_painter.dart';
import 'package:loffice/src/word/paragraph.dart';

/// The documents of ../testdata exported. With $LOFFICE_PDF set, the files
/// are written there, each beside its first page as the screen draws it.
void main() {
  final out = Platform.environment['LOFFICE_PDF'];

  setUpAll(() async {
    await OfficeFonts.register({
      for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf')))
        f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
    });
  });

  Tree tree(File file) => Tree.fromEdit(Edit.fromJson(jsonDecode(file.readAsStringSync()))!)!;
  Iterable<File> trees(String dir) =>
      Directory('../testdata/$dir').listSync().whereType<File>().where((f) => f.path.endsWith('.json'));
  String name(File f) => f.uri.pathSegments.last.replaceAll('.json', '');

  Future<void> screen(String name, ui.Size size, void Function(ui.Canvas) paint) async {
    if (out == null) return;
    final recorder = ui.PictureRecorder();
    paint(ui.Canvas(recorder));
    final image = await recorder.endRecording().toImage(size.width.ceil(), size.height.ceil());
    final png = await image.toByteData(format: ui.ImageByteFormat.png);
    File('$out/$name-screen.png').writeAsBytesSync(png!.buffer.asUint8List());
  }

  void check(String name, Uint8List pdf, int pages, Set<String> missing) {
    expect(latin1.decode(pdf.sublist(0, 8)), '%PDF-1.7');
    expect(RegExp(r'/Type /Page ').allMatches(latin1.decode(pdf)).length, pages, reason: name);
    expect(missing, isEmpty, reason: name);
    if (out != null) File('$out/$name.pdf').writeAsBytesSync(pdf);
  }

  test('Word documents', () async {
    for (final file in trees('docx')) {
      final doc = WordDocument(tree(file));
      final context = WordContext(doc);
      final layout = WordLayout(doc, context, ParaCache());
      final painter = PagePainter(context: context);
      var missing = <String>{};
      final pdf = await wordPdf(layout, painter, title: name(file), missing: (m) => missing = m);
      check(name(file), pdf, layout.pages.length, missing);
      await screen(name(file), layout.pages.first.size, (c) => painter.paint(c, layout.pages.first));
    }
  });

  test('presentations', () async {
    for (final file in trees('pptx')) {
      final deck = Deck(tree(file));
      final painter = SlidePainter(deck);
      var missing = <String>{};
      final pdf = await deckPdf(deck, painter, title: name(file), missing: (m) => missing = m);
      check(name(file), pdf, deck.slides.length, missing);
      await screen(name(file), deck.size, (c) => painter.paint(c, deck.slides.first));
    }
  });
}
