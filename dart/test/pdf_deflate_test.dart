import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/pdf/deflate.dart';

void main() {
  test('what is deflated inflates to the same, whatever it is', () {
    final random = Random(7);
    final inputs = <Uint8List>[
      Uint8List(0),
      Uint8List.fromList([1]),
      Uint8List.fromList('abcabcabcabcabcabc'.codeUnits),
      Uint8List.fromList(List.filled(100000, 65)),
      Uint8List.fromList(List.generate(70000, (_) => random.nextInt(256))),
      Uint8List.fromList(List.generate(200000, (i) => 'BT /F1 12 Tf 1 0 0 -1 72 ${i % 97} Tm <0041> Tj ET\n'.codeUnitAt(i % 40))),
      File('fonts/Carlito-Regular.ttf').readAsBytesSync(),
    ];
    for (final input in inputs) {
      final packed = deflate(input);
      expect(zlib.decode(packed), input);
      if (input.length > 1000 && input.first == 65) expect(packed.length, lessThan(2000));
    }
    final font = inputs.last;
    expect(deflate(font).length, lessThan(font.length * 0.8));
  });
}
