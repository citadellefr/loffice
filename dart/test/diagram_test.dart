import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';
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
import 'package:trame/trame.dart';

import 'word_layout_test.dart' show document, text;

const _emu = 12700;

Map<String, Object?> _xfrm(num x, num y, num w, num h) => {'x': x * _emu, 'y': y * _emu, 'w': w * _emu, 'h': h * _emu};

Map<String, Object?> _solid(String rgb) => {
  'solid': {'rgb': rgb},
};

Map<String, Object?> get _smartArt => {
  'shapes': [
    {
      'xfrm': _xfrm(0, 0, 50, 50),
      'geom': {'prst': 'rect'},
      'fill': _solid('ff0000'),
    },
    {
      'xfrm': _xfrm(60, 0, 40, 50),
      'geom': {'prst': 'rect'},
      'fill': _solid('00ff00'),
      'text': [
        {
          'i': 'A',
          'a': {'sz': '1000', 'fill': jsonEncode(_solid('0000ff'))},
        },
        {'i': '\n'},
      ],
    },
  ],
};

Deck _deck(List<(String, Map<String, Object?>, String)> shapes) {
  final slide = Node(id: 's1', type: 'slide', parent: 'deck', key: 'K');
  return Deck(
    Tree.fromEdit(
      Edit([
        Change.create(
          Node(
            id: 'deck',
            type: 'deck',
            key: 'K',
            attributes: {
              'size': {'w': 200 * _emu, 'h': 100 * _emu},
            },
          ),
        ),
        Change.create(slide),
        for (final (i, s) in shapes.indexed) Change.create(Node(id: 's1-$i', type: s.$1, parent: 's1', key: s.$3, attributes: s.$2)),
      ]),
    )!,
  );
}

Map<String, Object?> _frame(String kind, Map<String, Object?> extra) => {'frame': kind, 'xfrm': _xfrm(10, 10, 100, 50), ...extra};

Future<ui.Image> _solidImage(Color color) async {
  final recorder = ui.PictureRecorder();
  ui.Canvas(recorder).drawRect(const Rect.fromLTWH(0, 0, 4, 4), Paint()..color = color);
  return recorder.endRecording().toImage(4, 4);
}

class _Pixels {
  _Pixels(this.data, this.width);

  final ByteData data;
  final int width;

  Color at(int x, int y) {
    final i = 4 * (y * width + x);
    return Color.fromARGB(data.getUint8(i + 3), data.getUint8(i), data.getUint8(i + 1), data.getUint8(i + 2));
  }
}

Future<_Pixels> _render(Size size, void Function(ui.Canvas) paint) async {
  final recorder = ui.PictureRecorder();
  paint(ui.Canvas(recorder));
  final image = await recorder.endRecording().toImage(size.width.ceil(), size.height.ceil());
  return _Pixels((await image.toByteData())!, image.width);
}

Future<_Pixels> _slide(Deck deck, SlidePainter painter) => _render(deck.size, (c) => painter.paint(c, deck.slides.first));

String _contents(Uint8List pdf) {
  final out = StringBuffer();
  final raw = latin1.decode(pdf);
  for (final m in RegExp(r'stream\r?\n').allMatches(raw)) {
    final end = raw.indexOf('endstream', m.end);
    try {
      out.write(latin1.decode(zlib.decode(pdf.sublist(m.end, end))));
    } on Object {
      // not a compressed stream
    }
  }
  return out.toString();
}

void main() {
  const red = Color(0xFFFF0000), green = Color(0xFF00FF00);

  setUpAll(() async {
    await OfficeFonts.register({
      for (final f in Directory('fonts').listSync().whereType<File>().where((f) => f.path.endsWith('.ttf'))) f.uri.pathSegments.last.replaceAll('.ttf', ''): f.readAsBytesSync(),
    });
  });

  test('SmartArt is drawn from its recorded drawing', () async {
    final deck = _deck([
      ('frame', _frame('diagram', {'diagram': _smartArt}), 'K'),
    ]);
    final p = await _slide(deck, SlidePainter(deck));
    // the frame is at 10, 10: the first shape covers 10–60, the second 70–110
    expect(p.at(30, 30), red);
    expect(p.at(105, 55), green);
    expect(p.at(65, 30), const Color(0xFFFFFFFF));
    expect(p.at(150, 30), const Color(0xFFFFFFFF));
    var blue = 0;
    for (var y = 10; y < 60; y++) {
      for (var x = 70; x < 110; x++) {
        if (p.at(x, y).b > 0.5 && p.at(x, y).r < 0.3) blue++;
      }
    }
    expect(blue, greaterThan(0), reason: 'the text of the shape');
  });

  test('an object draws its fallback image, a grey rectangle stands only for nothing', () async {
    final image = await _solidImage(red);
    final deck = _deck([
      (
        'frame',
        _frame('ole', {
          'blip': {'media': 'preview'},
        }),
        'K',
      ),
      ('other', {'xfrm': _xfrm(120, 10, 60, 50)}, 'L'),
    ]);
    final p = await _slide(deck, SlidePainter(deck, images: (m) => m == 'preview' ? image : null));
    expect(p.at(50, 30), red);
    final grey = p.at(150, 30);
    expect(grey.a, 1);
    expect(grey.r, lessThan(0.95));
    expect(grey.r, greaterThan(0.8));
  });

  test('the fallback of an alternate content is drawn', () async {
    final image = await _solidImage(green);
    final deck = Deck(
      Tree.fromEdit(
        Edit([
          Change.create(
            Node(
              id: 'deck',
              type: 'deck',
              key: 'K',
              attributes: {
                'size': {'w': 200 * _emu, 'h': 100 * _emu},
              },
            ),
          ),
          Change.create(const Node(id: 's1', type: 'slide', parent: 'deck', key: 'K')),
          Change.create(const Node(id: 's1-x', type: 'alt', parent: 's1', key: 'K')),
          Change.create(
            Node(
              id: 's1-x-4',
              type: 'pic',
              parent: 's1-x',
              key: 'K',
              attributes: {
                'xfrm': _xfrm(10, 10, 100, 50),
                'blip': {'media': 'fallback'},
              },
            ),
          ),
        ]),
      )!,
    );
    final p = await _slide(deck, SlidePainter(deck, images: (m) => m == 'fallback' ? image : null));
    expect(p.at(50, 30), green);
  });

  test('a presentation exported as a PDF draws them too', () async {
    final image = await _solidImage(red);
    final drawn = _deck([
      ('frame', _frame('diagram', {'diagram': _smartArt}), 'K'),
      (
        'frame',
        _frame('ole', {
          'blip': {'media': 'preview'},
          'xfrm': _xfrm(120, 10, 60, 50),
        }),
        'L',
      ),
    ]);
    final pdf = _contents(await deckPdf(drawn, SlidePainter(drawn, images: (m) => image)));
    expect(pdf, contains('1 0 0 rg'));
    expect(pdf, contains('0 1 0 rg'));
    expect(pdf, contains('/Im1 Do'), reason: 'the preview of the object');
    final blank = _deck([
      ('other', {'xfrm': _xfrm(10, 10, 100, 50)}, 'K'),
    ]);
    expect(_contents(await deckPdf(blank, SlidePainter(blank))), isNot(contains('1 0 0 rg')));
  });

  group('Word', () {
    WordDocument doc() => document([
      text(
        'p1',
        'a',
        Delta()
          ..insert('\uFFFC', {
            'img': jsonEncode({'w': 100 * _emu, 'h': 50 * _emu, 'diagram': _smartArt}),
          })
          ..insert('\n'),
      ),
    ]);

    Future<_Pixels> page(WordDocument d, PagePainter painter, WordLayout layout) => _render(layout.pages.first.size, (c) => painter.paint(c, layout.pages.first));

    test('SmartArt is drawn from its recorded drawing', () async {
      final d = doc();
      final context = WordContext(d);
      final layout = WordLayout(d, context, ParaCache());
      final p = await page(d, PagePainter(context: context), layout);
      var reds = 0, greens = 0;
      final body = layout.pages.first.body;
      for (var y = body.top.floor(); y < body.top + 60; y++) {
        for (var x = body.left.floor(); x < body.left + 110; x++) {
          final c = p.at(x, y);
          if (c == red) reds++;
          if (c == green) greens++;
        }
      }
      expect(reds, greaterThan(1000));
      expect(greens, greaterThan(500));
    });

    test('a page exported as a PDF draws it too', () async {
      final d = doc();
      final context = WordContext(d);
      final layout = WordLayout(d, context, ParaCache());
      final pdf = _contents(await wordPdf(layout, PagePainter(context: context)));
      expect(pdf, contains('1 0 0 rg'));
      expect(pdf, contains('0 1 0 rg'));
    });
  });
}
