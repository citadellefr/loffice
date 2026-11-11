import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/animation_sequence.dart';
import 'package:loffice/src/powerpoint/deck.dart';
import 'package:loffice/src/powerpoint/slide_painter.dart';
import 'package:loffice/src/powerpoint/slideshow.dart';
import 'package:trame/trame.dart';

class _Recorder extends SlidePainter {
  _Recorder(super.deck);

  Node? slide;
  Map<String, ShapeLook>? looks;

  @override
  void paint(Canvas canvas, Node slide, {Map<String, ShapeLook>? looks}) {
    this.slide = slide;
    this.looks = looks;
    super.paint(canvas, slide, looks: looks);
  }
}

void main() {
  // shp-shapes has two slides
  Future<_Recorder> show(WidgetTester tester, List<Map<String, Object?>> transitions, {bool presenter = false, List<Map<String, Object?>>? animations}) async {
    final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
    final deck = Deck(tree);
    for (final (i, t) in transitions.indexed) {
      expect(
        tree.apply(
          Edit([
            Change.set(deck.slides[i].id, attributes: {'transition': t}),
          ]),
        ),
        isNotNull,
      );
    }
    if (animations != null) {
      expect(
        tree.apply(
          Edit([
            Change.set(deck.slides[0].id, attributes: {'animations': animations}),
          ]),
        ),
        isNotNull,
      );
    }
    if (presenter) {
      final notes = tree
          .children(deck.slides[0].id)
          .firstWhere(
            (n) => n.type == 'notes',
            orElse: () => const Node(id: '', type: '', key: ''),
          );
      if (notes.id.isEmpty) {
        expect(
          tree.apply(
            Edit([
              Change.create(Node(id: 'notes', type: 'notes', parent: deck.slides[0].id, key: 'V', text: Delta([const Op.insert('Saluer la salle\n')]))),
            ]),
          ),
          isNotNull,
        );
      }
    }
    final painter = _Recorder(deck);
    await tester.pumpWidget(
      MaterialApp(
        home: Slideshow(deck: deck, painter: painter, presenter: presenter),
      ),
    );
    return painter;
  }

  const end = 'Fin du diaporama, cliquez pour quitter.';

  testWidgets('goes on by itself, the next slide pushed in', (tester) async {
    await show(tester, [
      {'dur': 500, 'after': 1000},
      {'effect': 'push', 'dir': 'u', 'dur': 300, 'after': 1000},
    ]);
    await tester.pump(const Duration(milliseconds: 1100));
    await tester.pump(const Duration(milliseconds: 150));
    expect(find.text(end), findsNothing);
    await tester.pump(const Duration(milliseconds: 200));
    await tester.pump(const Duration(milliseconds: 1100));
    expect(find.text(end), findsOneWidget);
  });

  testWidgets('a slide a click does not go on from goes on with the keyboard', (tester) async {
    await show(tester, [
      {'dur': 500, 'noClick': true},
      {'effect': 'fade', 'thruBlk': true, 'dur': 0},
    ]);
    await tester.tap(find.byType(CustomPaint).last);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.space);
    await tester.pump();
    await tester.tap(find.byType(CustomPaint).last);
    await tester.pump();
    expect(find.text(end), findsOneWidget);
  });

  testWidgets('the presenter view shows the notes, the next slide and the time', (tester) async {
    tester.view.physicalSize = const Size(1400, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await show(tester, [], presenter: true);
    expect(find.text('Diapositive 1 sur 2'), findsOneWidget);
    expect(find.textContaining('Saluer la salle'), findsOneWidget);
    expect(find.text('00:00'), findsOneWidget);
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('00:02'), findsOneWidget);
    await tester.tap(find.byTooltip('Diapositive suivante'));
    await tester.pump();
    expect(find.text('Diapositive 2 sur 2'), findsOneWidget);
    expect(find.text(end), findsOneWidget);
    await tester.tap(find.byTooltip('Suspendre le minuteur'));
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('00:02'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });

  group('animations', () {
    const shape = 's256-5';
    Map<String, Object?> effect(String trigger, {String kind = 'entr', int preset = 10, int dur = 500}) => {'target': shape, 'class': kind, 'preset': preset, 'trigger': trigger, 'dur': dur};

    Future<void> click(WidgetTester tester) async {
      await tester.tap(find.byType(CustomPaint).last);
      await tester.pump();
      await tester.pump(const Duration(seconds: 1));
    }

    testWidgets('a shape that comes in at a click is hidden until the click', (tester) async {
      final shown = await show(tester, [], animations: [effect('click')]);
      expect(shown.looks![shape]!.opacity, 0);
      await click(tester);
      expect(shown.looks![shape], isNull);
      expect(shown.slide!.id, 's256');
      await click(tester);
      expect(shown.slide!.id, 's257');
    });

    testWidgets('the show goes back one step before the slide before', (tester) async {
      final shown = await show(tester, [], animations: [effect('click')]);
      await click(tester);
      await click(tester);
      expect(shown.slide!.id, 's257');
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(shown.slide!.id, 's256');
      expect(shown.looks![shape], isNull);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(shown.slide!.id, 's256');
      expect(shown.looks![shape]!.opacity, 0);
    });

    testWidgets('going back while a step plays stops it', (tester) async {
      final shown = await show(tester, [], animations: [effect('click', dur: 2000)]);
      await tester.sendKeyEvent(LogicalKeyboardKey.space);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 1000));
      expect(shown.looks![shape]!.opacity, closeTo(0.5, 0.01));
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(shown.looks![shape]!.opacity, 0);
      expect(shown.slide!.id, 's256');
    });

    testWidgets('a click while a step plays ends it and plays the next', (tester) async {
      final shown = await show(
        tester,
        [],
        animations: [
          effect('click', dur: 2000),
          effect('click', kind: 'exit', dur: 2000),
        ],
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.space);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      await tester.sendKeyEvent(LogicalKeyboardKey.space);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 1000));
      expect(shown.looks![shape]!.opacity, closeTo(0.5, 0.1));
    });

    testWidgets('the effects before the first click play when the slide comes on', (tester) async {
      final shown = await show(
        tester,
        [],
        animations: [
          effect('after'),
          effect('click', kind: 'exit'),
        ],
      );
      await tester.pump();
      await tester.pump(const Duration(seconds: 1));
      expect(shown.looks![shape], isNull);
      await click(tester);
      expect(shown.looks![shape]!.opacity, 0);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(shown.looks![shape], isNull);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowLeft);
      await tester.pump();
      expect(shown.slide!.id, 's256');
    });

    testWidgets('the presenter view counts the steps left', (tester) async {
      tester.view.physicalSize = const Size(1400, 900);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      await show(
        tester,
        [],
        presenter: true,
        animations: [
          effect('click'),
          effect('click', kind: 'exit'),
        ],
      );
      expect(find.text('2 animations restantes'), findsOneWidget);
      await tester.sendKeyEvent(LogicalKeyboardKey.space);
      await tester.pump();
      await tester.pump(const Duration(seconds: 1));
      expect(find.text('1 animation restante'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
    });

    testWidgets('a hidden shape is not painted and every effect paints', (tester) async {
      final tree = Tree.fromEdit(Edit.fromJson(jsonDecode(File('../testdata/pptx/shp-shapes.json').readAsStringSync()))!)!;
      final deck = Deck(tree);
      final painter = SlidePainter(deck);
      Future<List<int>> render(Map<String, ShapeLook>? looks) async {
        final recorder = ui.PictureRecorder();
        painter.paint(Canvas(recorder), deck.slides[0], looks: looks);
        final image = await recorder.endRecording().toImage(720, 540);
        return (await image.toByteData())!.buffer.asUint8List();
      }

      await tester.runAsync(() async {
        final plain = await render(null);
        expect(await render({shape: const ShapeLook()}), plain);
        expect(await render({shape: ShapeLook.hidden}), isNot(plain));
        for (final (kind, preset, subtype) in [
          ('entr', 2, 4),
          ('entr', 22, 8),
          ('exit', 22, 1),
          ('entr', 53, 16),
          ('emph', 26, 0),
          ('emph', 8, 0),
          ('entr', 999, 0),
          ('exit', 999, 0),
          ('emph', 999, 0),
        ]) {
          final e = AnimationSequence.of([
            {'target': shape, 'class': kind, 'preset': preset, 'subtype': subtype, 'trigger': 'click', 'dur': 1000},
          ]);
          final mid = await render(e.looks(0, 500));
          expect(mid, isNot(plain), reason: '$kind $preset');
        }
      });
    });
  });
}
