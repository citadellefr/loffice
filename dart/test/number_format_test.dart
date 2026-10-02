import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/number_format.dart';

void main() {
  // written by the Go engine: go test ./formula -run FormatVectors -update
  final vectors = jsonDecode(File('../testdata/formula/formats.json').readAsStringSync()) as List<Object?>;

  test('formats write numbers as the engine does', () {
    for (final v in vectors.cast<Map<String, Object?>>()) {
      final locale = v['locale'] == 'fr' ? NumberLocale.fr : NumberLocale.en;
      final got = NumberFormat(v['code']! as String).format((v['n']! as num).toDouble(), locale).text;
      expect(got, v['want'], reason: '${v['n']} with ${v['code']}');
    }
  });

  test('text in the fourth section', () {
    expect(NumberFormat('0;0;0;"<"@">"').formatText('abc').text, '<abc>');
    expect(NumberFormat('0.00').formatText('abc').text, 'abc');
  });

  test('colors of sections', () {
    expect(NumberFormat('0;[Red]-0').format(-1, NumberLocale.en).color, 'red');
  });

  test('serial numbers', () {
    expect(serialOf(2024, 12, 25), 45651);
    expect(civilOf(45651), (2024, 12, 25));
    expect(civilOf(60), (1900, 2, 29));
    expect(weekdayOf(45651), 3);
  });

  test('plain writing', () {
    expect(plain(1e21), '1000000000000000000000');
    expect(plain(1.5e-7), '0.00000015');
    expect(plain(-2.0), '-2');
  });
}
