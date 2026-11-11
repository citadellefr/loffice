import 'dart:math' as math;
import 'dart:ui';

import 'package:trame/trame.dart';

/// How a shape is drawn while an effect acts on it: faded, moved in from the
/// side of the slide it comes from, scaled, turned, or cut to the part
/// [reveal] of its box (in fractions of it) that shows.
class ShapeLook {
  const ShapeLook({this.opacity = 1, this.scale = 1, this.angle = 0, this.from = Offset.zero, this.away = 0, this.reveal});

  static const hidden = ShapeLook(opacity: 0);

  final double opacity;
  final double scale;

  /// In radians, clockwise.
  final double angle;

  /// The side the shape comes from or goes to, as a unit step (1, 0 is the
  /// right edge), and [away] how much of the way to that edge is left.
  final Offset from;
  final double away;
  final Rect? reveal;
}

/// An effect of a slide as it plays: [start] and [dur] are in milliseconds
/// from the start of its step.
class ShowEffect {
  const ShowEffect({required this.target, required this.kind, required this.preset, required this.subtype, required this.start, required this.dur});

  final String? target;

  /// "entr", "emph", "exit" or "path".
  final String kind;
  final int preset, subtype;
  final int start, dur;

  bool get enters => kind == 'entr';
  bool get exits => kind == 'exit';

  /// The look of the shape [elapsed] milliseconds into the step, null once the
  /// effect is over.
  ShapeLook? at(int elapsed) {
    if (elapsed < start) return null;
    if (elapsed >= start + dur) return null;
    final p = (elapsed - start) / dur;
    final side = Offset(((subtype & 2) != 0 ? 1 : 0) - ((subtype & 8) != 0 ? 1 : 0), ((subtype & 4) != 0 ? 1 : 0) - ((subtype & 1) != 0 ? 1 : 0));
    final from = side == Offset.zero ? const Offset(0, 1) : side;
    final fade = enters ? p : 1 - p;
    return switch ((kind, preset)) {
      ('entr', 2) => ShapeLook(from: from, away: 1 - p),
      ('entr', 53) => ShapeLook(opacity: p, scale: p),
      ('entr', 22) => ShapeLook(reveal: _wipe(from, p)),
      ('exit', 22) => ShapeLook(reveal: _wipe(-from, 1 - p)),
      ('emph', 26) => ShapeLook(scale: 1 + 0.15 * math.sin(math.pi * p)),
      ('emph', 8) => ShapeLook(angle: 2 * math.pi * p),
      ('emph', _) || ('path', _) => ShapeLook(opacity: 1 - 0.6 * math.sin(math.pi * p)),
      _ => ShapeLook(opacity: fade),
    };
  }

  /// The part of the box a wipe shows, [p] of it from the side [from].
  static Rect _wipe(Offset from, double p) => Rect.fromLTRB(from.dx > 0 ? 1 - p : 0, from.dy > 0 ? 1 - p : 0, from.dx < 0 ? p : 1, from.dy < 0 ? p : 1);
}

/// A click of the show: the effects that start at it, with the ones that go
/// with or after them.
class ShowStep {
  const ShowStep(this.effects, this.dur);

  final List<ShowEffect> effects;

  /// When the last effect ends, in milliseconds.
  final int dur;
}

/// The animations of a slide cut into the steps a show plays one by one: an
/// effect that starts at a click opens a step, one that goes "with" the one
/// before starts with it, one that goes "after" starts when what comes
/// before ends. Effects before the first click play when the slide comes
/// on, as [auto] says.
class AnimationSequence {
  AnimationSequence(Node slide) : this.of(slide.attributes['animations'] as List<Object?>? ?? const []);

  AnimationSequence.of(List<Object?> effects) : auto = effects.isNotEmpty && ((effects.first as Map<String, Object?>)['trigger'] ?? 'click') != 'click' {
    var current = <ShowEffect>[];
    var base = 0, end = 0;
    void close() {
      if (current.isNotEmpty) steps.add(ShowStep(current, end));
      current = [];
    }

    for (final e in effects.cast<Map<String, Object?>>()) {
      final trigger = e['trigger'] ?? 'click';
      if (trigger == 'click' || current.isEmpty) {
        close();
        base = end = 0;
      } else if (trigger == 'after') {
        base = end;
      }
      final kind = e['class'] as String? ?? 'entr';
      final preset = (e['preset'] as num?)?.toInt() ?? 0;
      final dur = (e['dur'] as num?)?.toInt() ?? 0;
      final start = base + ((e['delay'] as num?)?.toInt() ?? 0);
      final instant = preset == 1 && (kind == 'entr' || kind == 'exit');
      final effect = ShowEffect(target: e['target'] as String?, kind: kind, preset: preset, subtype: (e['subtype'] as num?)?.toInt() ?? 0, start: start, dur: instant ? 0 : (dur > 0 ? dur : 500));
      current.add(effect);
      end = math.max(end, effect.start + effect.dur);
    }
    close();
  }

  final steps = <ShowStep>[];

  /// Whether the first step plays by itself when the slide comes on.
  final bool auto;

  /// The first step the audience can go back before: the one that plays by
  /// itself stays.
  int get floor => auto ? 1 : 0;

  /// The looks of the shapes after [done] steps and, when one plays, [elapsed]
  /// milliseconds into the step after them. A shape whose first effect is an
  /// entrance is hidden until then; absent shapes are drawn as they are.
  Map<String, ShapeLook> looks(int done, [int? elapsed]) {
    final shown = <String, bool>{};
    for (final step in steps) {
      for (final e in step.effects) {
        if (e.target != null) shown.putIfAbsent(e.target!, () => !e.enters);
      }
    }
    void finish(ShowEffect e) {
      if (e.target != null && (e.enters || e.exits)) shown[e.target!] = e.enters;
    }

    for (final step in steps.take(done)) {
      step.effects.forEach(finish);
    }
    final moving = <String, ShapeLook>{};
    final playing = elapsed == null ? const <ShowEffect>[] : steps.elementAtOrNull(done)?.effects ?? const <ShowEffect>[];
    for (final e in playing) {
      if (e.target == null || elapsed! < e.start) continue;
      final look = e.at(elapsed);
      if (look == null) {
        finish(e);
        moving.remove(e.target);
      } else {
        moving[e.target!] = look;
      }
    }
    return {
      for (final MapEntry(:key, :value) in shown.entries)
        if (moving[key] != null) key: moving[key]! else if (!value) key: ShapeLook.hidden,
    };
  }
}
