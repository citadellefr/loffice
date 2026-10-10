import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/pdf/document.dart';
import 'package:loffice/src/pdf/truetype.dart';

void main() {
  final carlito = TrueTypeFont(File('fonts/Carlito-Regular.ttf').readAsBytesSync());

  test('a font tells its glyphs, their advances and its measures', () {
    expect(carlito.name, 'Carlito');
    expect(carlito.unitsPerEm, 2048);
    expect(carlito.glyph(0x41), isNot(0));
    expect(carlito.glyph(0xE9), isNot(0));
    expect(carlito.glyph(0x4E2D), 0);
    // Calibri's: an A of 1185 units, a space of 463
    expect(carlito.advance(carlito.glyph(0x41)), 1185);
    expect(carlito.advance(carlito.glyph(0x20)), 463);
    expect(carlito.bold, isFalse);
    expect(TrueTypeFont(File('fonts/Carlito-BoldItalic.ttf').readAsBytesSync()).italicAngle, isNot(0));
  });

  test('a subset keeps the glyphs asked for, under their numbers', () {
    final used = {carlito.glyph(0x41), carlito.glyph(0xE9)};
    final subset = carlito.subset(used);
    expect(subset.length, lessThan(carlito.bytes.length ~/ 10));
    // a TrueType file of nine tables, the checksum of which is the one fonts have
    final data = ByteData.sublistView(subset);
    expect(data.getUint32(0), 0x00010000);
    expect(data.getUint16(4), 9);
    var sum = 0;
    for (var i = 0; i < subset.length; i += 4) {
      sum = (sum + data.getUint32(i)) & 0xFFFFFFFF;
    }
    expect(sum, 0xB1B0AFBA);
  });

  test('numbers are written short', () {
    expect([0.0, -0.0001, 1.5, 12.3456, -3.0, 100.0].map(pdfNumber), ['0', '0', '1.5', '12.346', '-3', '100']);
  });

  test('a document of two pages, with text, a picture, opacity and a gradient', () {
    final doc = PdfDocument();
    final font = doc.font(carlito);
    final text = [for (final c in 'Été à Noël'.runes) font.glyph(c)!].join();
    final image = doc.image(2, 2, Uint8List.fromList([255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 0]), Uint8List.fromList([255, 255, 255, 64]));
    final state = doc.alpha(0.5, 1);
    final shade = doc.shading([72, 0, 300, 0], [[1, 0, 0], [1, 1, 0], [0, 0, 1]], [0, 0.3, 1]);
    doc.addPage(595, 842, '''
BT /${font.name} 24 Tf 0 0 0 rg 1 0 0 -1 72 100 Tm <$text> Tj ET
q 72 120 228 60 re W n /$shade sh Q
q /$state gs 0 0.4 0.8 rg 72 200 100 50 re f Q
q 100 0 0 -100 72 400 cm /$image Do Q
''');
    doc.addPage(842, 595, '0 0 0 RG 2 w 20 20 m 822 575 l S');
    final bytes = doc.save(title: 'Essai é');
    expect(String.fromCharCodes(bytes.take(8)), '%PDF-1.7');
    final out = Platform.environment['LOFFICE_PDF'];
    if (out != null) File('$out/document.pdf').writeAsBytesSync(bytes);
  });
}
