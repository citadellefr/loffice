import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/cell_painter.dart';
import 'package:loffice/src/excel/conditional.dart';
import 'package:loffice/src/excel/edits.dart';
import 'package:loffice/src/excel/number_format.dart';
import 'package:loffice/src/excel/rules.dart';
import 'package:loffice/src/excel/validation.dart';
import 'package:loffice/src/excel/workbook.dart';
import 'package:trame/trame.dart';

Tree sheetTree(Map<String, Object?> attributes, {List<Cell> cells = const [], Grid? looks}) => Tree.fromEdit(
  Edit([
    Change.create(const Node(id: 'book', type: 'book', key: 'V')),
    Change.create(Node(id: 'S1', type: 'sheet', parent: 'book', key: 'V', attributes: {'name': 'Feuil1', ...attributes}, grid: Grid(cells))),
    if (looks != null) Change.create(Node(id: 'S1.cf', type: 'looks', parent: 'S1', key: 'V', grid: looks)),
  ]),
)!;

Color argb(int rgb) => Color(0xFF000000 | rgb);

/// The rule that fills cells with [rgb].
Map<String, Object?> filling(String type, int rgb, [Map<String, Object?> more = const {}]) => {
  'type': type,
  'style': {
    'fill': {
      'pattern': 'solid',
      'fg': {'rgb': rgb.toRadixString(16).padLeft(6, '0')},
    },
  },
  ...more,
};

void main() {
  group('rules removed from an area', () {
    final rules = [
      {
        'ref': 'B2:B9',
        'type': 'formula',
        'f': ['B2>(5)'],
        'pri': 1,
      },
      {'ref': 'D1', 'type': 'duplicateValues', 'pri': 2},
      {'ref': 'A1:E5', 'type': 'uniqueValues', 'pri': 3},
    ];

    List<Map<String, Object?>> after(CellArea? area) {
      final tree = sheetTree({'cf': rules});
      final edit = SheetEdits(Workbook(tree)).clearConditionals(tree['S1']!, area);
      tree.apply(edit);
      final cf = tree['S1']!.attributes['cf'];
      return [for (final r in cf as List<Object?>? ?? const []) r! as Map<String, Object?>];
    }

    test('cut the rectangles they cover and move their formulas', () {
      final cf = after(const CellArea(2, 2, 3, 2));
      expect(cf[0]['ref'], 'B4:B9');
      expect(cf[0]['f'], ['B4>(5)']);
      expect(cf[1]['ref'], 'D1');
      expect(cf[2]['ref'], 'A1:E1 A4:E5 A2:A3 C2:E3');
    });

    test('go once no cell is left, the others numbered again', () {
      final cf = after(const CellArea(1, 4, 1, 4));
      expect(cf.map((r) => r['type']), ['formula', 'uniqueValues']);
      expect(cf.map((r) => r['pri']), [1, 2]);
      expect(cf[1]['ref'], 'A2:E5 A1:C1 E1');
    });

    test('all go for the whole sheet', () {
      expect(after(null), isEmpty);
      expect(after(const CellArea(1, 1, maxRows, maxCols)), isEmpty);
    });

    test('are added first, for an area', () {
      final tree = sheetTree({'cf': rules});
      tree.apply(SheetEdits(Workbook(tree)).addConditional(tree['S1']!, const CellArea(2, 3, 4, 3), topRule(3, 'green')));
      final cf = tree['S1']!.attributes['cf']! as List<Object?>;
      expect(cf.first, {'type': 'top10', 'rank': 3, 'style': highlightStyles['green'], 'ref': 'C2:C4', 'pri': 1});
      expect(cf, hasLength(4));
    });
  });

  test('a validation replaces those of the cells it covers', () {
    final tree = sheetTree({
      'valid': [
        {'ref': 'A1:A5', 'type': 'list', 'f1': '"a,b"'},
        {'ref': 'C1', 'type': 'custom', 'f1': 'C1>0'},
      ],
    });
    final edits = SheetEdits(Workbook(tree));
    tree.apply(edits.validate(tree['S1']!, const CellArea(3, 1, 4, 1), {'type': 'whole', 'f1': '1'}));
    final valid = tree['S1']!.attributes['valid']! as List<Object?>;
    expect(valid, [
      {'ref': 'A1:A2 A5', 'type': 'list', 'f1': '"a,b"'},
      {'ref': 'C1', 'type': 'custom', 'f1': 'C1>0'},
      {'ref': 'A3:A4', 'type': 'whole', 'f1': '1'},
    ]);
    tree.apply(SheetEdits(Workbook(tree)).validate(tree['S1']!, const CellArea(1, 1, 5, 3), null));
    expect(tree['S1']!.attributes['valid'], isNull);
  });

  group('validation', () {
    final tree = sheetTree(
      const {},
      cells: const [
        Cell(1, 2, {'v': 7}),
        Cell(1, 3, {'v': 45292}),
        Cell(2, 2, {'v': 'x'}),
      ],
    );
    final book = Workbook(tree);
    final sheet = tree['S1']!;
    bool takes(Map<String, Object?> rule, String typed) => Validation({'ref': 'A1', ...rule}).takes(typed, book, sheet, NumberLocale.fr);

    test('takes whole numbers and decimals between bounds', () {
      const whole = {'type': 'whole', 'f1': '1', 'f2': '10'};
      expect(
        [
          for (final t in ['1', '10', '5', '0', '11', '5,5', 'abc', '']) takes(whole, t),
        ],
        [true, true, true, false, false, false, false, false],
      );
      expect(takes({...whole, 'blank': true}, ''), isTrue);
      const decimal = {'type': 'decimal', 'op': 'greaterThan', 'f1': '2.5'};
      expect(
        [
          for (final t in ['2,6', '2,5', '3', '-1']) takes(decimal, t),
        ],
        [true, false, true, false],
      );
      expect(takes({'type': 'whole', 'op': 'notBetween', 'f1': '1', 'f2': '3'}, '2'), isFalse);
      expect(takes({'type': 'whole', 'op': 'notEqual', 'f1': '3'}, '4'), isTrue);
    });

    test('takes bounds that are cells, and what only the server can tell', () {
      expect(takes({'type': 'whole', 'op': 'lessThanOrEqual', 'f1': 'B1'}, '7'), isTrue);
      expect(takes({'type': 'whole', 'op': 'lessThanOrEqual', 'f1': r'Feuil1!$B$1'}, '8'), isFalse);
      expect(takes({'type': 'whole', 'op': 'lessThanOrEqual', 'f1': 'B2'}, '800'), isTrue);
      expect(takes({'type': 'whole', 'op': 'lessThanOrEqual', 'f1': 'SUM(B1:B2)'}, '800'), isTrue);
      expect(takes({'type': 'custom', 'f1': 'A1>3'}, '1'), isTrue);
    });

    test('takes dates and times', () {
      const date = {'type': 'date', 'op': 'greaterThanOrEqual', 'f1': 'DATE(2024,1,1)'};
      expect(takes(date, '01/01/2024'), isTrue);
      expect(takes(date, '31/12/2023'), isFalse);
      expect(takes(date, '12'), isFalse);
      expect(takes({'type': 'date', 'op': 'lessThan', 'f1': 'C1'}, '31/12/2023'), isTrue);
      const time = {'type': 'time', 'f1': '0.25', 'f2': '0.75'};
      expect(takes(time, '12:00'), isTrue);
      expect(takes(time, '23:00'), isFalse);
    });

    test('takes texts of a length and the values of a list', () {
      const length = {'type': 'textLength', 'op': 'lessThanOrEqual', 'f1': '3'};
      expect(takes(length, 'abc'), isTrue);
      expect(takes(length, 'abcd'), isFalse);
      expect(takes({'type': 'list', 'f1': '"Oui,Non"'}, 'non'), isTrue);
      expect(takes({'type': 'list', 'f1': '"Oui,Non"'}, 'peut-être'), isFalse);
      expect(takes({'type': 'list', 'f1': 'Fonction()'}, 'peut-être'), isTrue);
    });

    test('shows bounds in French and reads them back', () {
      expect(boundText('2.5'), '2,5');
      expect(boundText('45292', type: 'date'), '01/01/2024');
      expect(boundText('SUM(A1,B1)'), 'SOMME(A1;B1)');
      expect(boundFormula('2,5'), '2.5');
      expect(boundFormula('01/01/2024'), '45292');
      expect(boundFormula('=SOMME(A1;B1)'), 'SUM(A1,B1)');
      expect(boundFormula('B2'), 'B2');
    });
  });

  group('looks', () {
    final rules = [
      filling('formula', 0xFF0000),
      filling('top10', 0x00FF00),
      filling('aboveAverage', 0x0000FF),
      filling('duplicateValues', 0xFFFF00),
      filling('uniqueValues', 0x00FFFF),
      {
        'type': 'colorScale',
        'colors': [
          {'rgb': '000000'},
          {'rgb': 'FFFFFF'},
        ],
      },
      {
        'type': 'colorScale',
        'colors': [
          {'rgb': 'FF0000'},
          {'rgb': '00FF00'},
          {'rgb': '0000FF'},
        ],
      },
      {
        'type': 'dataBar',
        'colors': [
          {'rgb': 'FF8000'},
        ],
      },
      {'type': 'iconSet', 'icons': '5Quarters'},
    ];
    final tree = sheetTree(
      {'cf': rules},
      looks: Grid(const [
        Cell(1, 1, {
          'r': [0],
        }),
        Cell(2, 1, {
          'r': [1],
        }),
        Cell(3, 1, {
          'r': [2],
        }),
        Cell(4, 1, {
          'r': [3],
        }),
        Cell(5, 1, {
          'r': [4],
        }),
        Cell(6, 1, {
          's': [5, 0.5],
        }),
        Cell(7, 1, {
          's': [6, 0.75],
        }),
        Cell(8, 1, {
          'b': [7, 55],
        }),
        Cell(9, 1, {
          'i': [8, 3],
        }),
      ]),
    );
    final book = Workbook(tree);
    final looks = book.looks(tree['S1']!);

    test('give cells the styles of the rules that hold for them', () {
      final base = book.style(null);
      expect([for (var r = 1; r <= 5; r++) looks.style(base, null, r, 1).fill], [argb(0xFF0000), argb(0x00FF00), argb(0x0000FF), argb(0xFFFF00), argb(0x00FFFF)]);
      expect(looks.style(base, null, 10, 1).fill, isNull);
    });

    test('shade cells on scales of two and three colors', () {
      final base = book.style(null);
      expect(looks.style(base, null, 6, 1).fill!.toARGB32(), 0xFF808080);
      expect(looks.style(base, null, 7, 1).fill, scaleColor([argb(0xFF0000), argb(0x00FF00), argb(0x0000FF)], 0.75));
    });

    test('give cells a data bar and an icon', () {
      expect(looks.bar(8, 1), (55.0, argb(0xFF8000)));
      expect(looks.icon(9, 1), ('5Quarters', 3));
      expect(looks.icon(8, 1), isNull);
      expect(looks.bar(9, 1), isNull);
    });
  });

  group('icons', () {
    const sets = [
      '3Arrows', '3ArrowsGray', '3Flags', '3TrafficLights1', '3TrafficLights2', '3Signs', '3Symbols', '3Symbols2', '3Stars', '3Triangles', //
      '4Arrows', '4ArrowsGray', '4RedToBlack', '4Rating', '4TrafficLights', '5Arrows', '5ArrowsGray', '5Rating', '5Quarters', '5Boxes',
    ];

    Future<Uint8List> pixels(void Function(Canvas) paint) async {
      final recorder = ui.PictureRecorder();
      paint(Canvas(recorder));
      final image = await recorder.endRecording().toImage(16, 16);
      return (await image.toByteData())!.buffer.asUint8List();
    }

    test('are drawn, each of a set differently', () async {
      for (final set in sets) {
        final n = int.parse(set.substring(0, 1));
        final drawn = <String>{};
        for (var i = 0; i < n; i++) {
          final bytes = await pixels((c) => paintIcon(c, const Rect.fromLTWH(1, 1, 14, 14), set, i));
          expect(bytes.any((b) => b != 0), isTrue, reason: '$set $i is blank');
          drawn.add(bytes.join(','));
        }
        expect(drawn, hasLength(n), reason: '$set has icons alike');
      }
    });
  });

  group('painter', () {
    Future<List<int>> render(Tree tree, {Size size = const Size(200, 100)}) async {
      final book = Workbook(tree);
      final painter = CellPainter(book: book, sheet: tree['S1']!, locale: NumberLocale.fr, x: (c) => (c - 1) * 100.0, y: (r) => (r - 1) * 50.0, width: (_) => 100, height: (_) => 50, gridlines: false);
      final recorder = ui.PictureRecorder();
      painter.paint(Canvas(recorder), const CellArea(1, 1, 2, 2));
      final image = await recorder.endRecording().toImage(size.width.round(), size.height.round());
      return (await image.toByteData())!.buffer.asUint8List().toList();
    }

    int at(List<int> rgba, int x, int y, {int width = 200}) {
      final i = (y * width + x) * 4;
      return rgba[i] << 16 | rgba[i + 1] << 8 | rgba[i + 2];
    }

    test('fills cells with what the rules give them, empty ones too', () async {
      final tree = sheetTree(
        {
          'cf': [
            filling('formula', 0xFFC7CE),
            {
              'type': 'colorScale',
              'colors': [
                {'rgb': '000000'},
                {'rgb': 'FFFFFF'},
              ],
            },
          ],
        },
        looks: Grid(const [
          Cell(1, 1, {
            'r': [0],
          }),
          Cell(2, 2, {
            's': [1, 1.0],
          }),
        ]),
      );
      final p = await render(tree);
      expect(at(p, 50, 25), 0xFFC7CE);
      expect(at(p, 150, 75), 0xFFFFFF);
      expect(at(p, 150, 25), 0);
    });

    test('draws data bars as long as the looks say', () async {
      final tree = sheetTree(
        {
          'cf': [
            {
              'type': 'dataBar',
              'colors': [
                {'rgb': '0000FF'},
              ],
            },
          ],
        },
        cells: const [
          Cell(1, 1, {'v': 1}),
        ],
        looks: Grid(const [
          Cell(1, 1, {
            'b': [0, 50],
          }),
        ]),
      );
      final p = await render(tree);
      // the bar fades from its color
      expect(at(p, 5, 25) >> 16, lessThan(0x40));
      expect(at(p, 5, 25) & 0xFF, 0xFF);
      expect(at(p, 40, 25), isNot(0));
      expect(at(p, 90, 25), 0);
    });
  });
}
