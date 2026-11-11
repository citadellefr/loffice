import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/powerpoint/animation_sequence.dart';

Map<String, Object?> effect(String target, String trigger, {String kind = 'entr', int preset = 10, int delay = 0, int dur = 500, int subtype = 0}) => {
  'target': target,
  'class': kind,
  'preset': preset,
  'subtype': subtype,
  'trigger': trigger,
  'delay': delay,
  'dur': dur,
};

void main() {
  test('the effects with and after the one that starts a step start in it, in order, with their delays', () {
    final sequence = AnimationSequence.of([
      effect('a', 'click', dur: 500),
      effect('b', 'with', delay: 200, dur: 1000),
      effect('c', 'after', delay: 100, dur: 400),
      effect('d', 'with', dur: 100),
      effect('e', 'click'),
      effect('f', 'after'),
    ]);
    expect(sequence.auto, isFalse);
    expect(sequence.steps.length, 2);
    final first = sequence.steps[0];
    expect([for (final e in first.effects) e.target], ['a', 'b', 'c', 'd']);
    expect([for (final e in first.effects) e.start], [0, 200, 1300, 1200]);
    expect(first.dur, 1700);
    expect([for (final e in sequence.steps[1].effects) (e.target, e.start)], [('e', 0), ('f', 500)]);
  });

  test('the effects before the first click make a step that plays by itself', () {
    final sequence = AnimationSequence.of([effect('a', 'after'), effect('b', 'click')]);
    expect(sequence.auto, isTrue);
    expect(sequence.steps.length, 2);
    expect(sequence.floor, 1);
    expect(AnimationSequence.of([effect('a', 'click')]).floor, 0);
  });

  test('an entrance hides its shape until it starts and an exit shows it until it ends', () {
    final sequence = AnimationSequence.of([effect('a', 'click', delay: 100), effect('b', 'click', kind: 'exit'), effect('c', 'click', kind: 'emph', preset: 26)]);
    expect(sequence.looks(0)['a'], same(ShapeLook.hidden));
    expect(sequence.looks(0).containsKey('b'), isFalse);
    expect(sequence.looks(0, 50)['a'], same(ShapeLook.hidden));
    expect(sequence.looks(0, 350)['a']!.opacity, closeTo(0.5, 1e-9));
    expect(sequence.looks(0, 600).containsKey('a'), isFalse);
    expect(sequence.looks(1)['b'], isNull);
    expect(sequence.looks(1, 250)['b']!.opacity, closeTo(0.5, 1e-9));
    expect(sequence.looks(2)['b'], same(ShapeLook.hidden));
    expect(sequence.looks(3).containsKey('c'), isFalse);
  });

  test('an effect of the gallery moves, scales, turns or reveals the shape', () {
    ShapeLook? at(String kind, int preset, {int subtype = 0, int elapsed = 250}) => AnimationSequence.of([effect('a', 'click', kind: kind, preset: preset, subtype: subtype)]).looks(0, elapsed)['a'];
    expect(at('entr', 2, subtype: 8)!.from, const Offset(-1, 0));
    expect(at('entr', 2, subtype: 1)!.from, const Offset(0, -1));
    expect(at('entr', 2, subtype: 4)!.away, closeTo(0.5, 1e-9));
    expect(at('entr', 53)!.scale, closeTo(0.5, 1e-9));
    expect(at('entr', 22, subtype: 8)!.reveal, const Rect.fromLTRB(0, 0, 0.5, 1));
    expect(at('exit', 22, subtype: 8)!.reveal, const Rect.fromLTRB(0.5, 0, 1, 1));
    expect(at('emph', 26)!.scale, greaterThan(1));
    expect(at('emph', 8, elapsed: 125)!.angle, closeTo(3.14159265 / 2, 1e-6));
  });

  test('a preset that is not known fades, whatever its class', () {
    for (final (kind, preset) in [('entr', 999), ('entr', 0), ('exit', 999), ('emph', 999), ('path', 0)]) {
      final look = AnimationSequence.of([effect('a', 'click', kind: kind, preset: preset)]).looks(0, 250)['a']!;
      expect(look.opacity, kind == 'entr' ? 0.5 : (kind == 'exit' ? 0.5 : lessThan(1)), reason: '$kind $preset');
      expect(look.scale, 1);
      expect(look.reveal, isNull);
    }
  });
}
