import 'dart:math' as math;
import 'dart:ui';

import 'package:trame/trame.dart';

import 'deck.dart';

/// The edits the presentation editor makes on the tree of a deck.
class DeckEdits {
  DeckEdits(this.deck);

  final Deck deck;

  Tree get tree => deck.tree;

  /// The attributes that place a shape at [bounds], in points, keeping its
  /// rotation, flips and, for a group, the coordinates of its children.
  Map<String, Object?> xfrm(Node shape, Rect bounds, {double? rotation}) {
    final old = deck.styleOf(shape).xfrm ?? const {};
    int emu(double v) => (v * emuPerPoint).round();
    return {
      ...old,
      'x': emu(bounds.left),
      'y': emu(bounds.top),
      'w': emu(bounds.width.abs()),
      'h': emu(bounds.height.abs()),
      if (rotation != null) 'rot': (rotation * 60000).round() % 21600000,
    }..removeWhere((k, v) => k == 'rot' && v == 0);
  }

  /// Places shapes; each rectangle is in points, in the coordinates of
  /// the slide or group the shape is in.
  Edit place(Map<Node, Rect> shapes, {double? rotation}) => _fitted([
    for (final e in shapes.entries) Change.set(e.key.id, attributes: {'xfrm': xfrm(e.key, e.value, rotation: rotation)}),
  ]);

  Edit rotate(Node shape, double degrees) {
    final bounds = deck.styleOf(shape).bounds;
    if (bounds == null) return Edit();
    return _fitted([Change.set(shape.id, attributes: {'xfrm': xfrm(shape, bounds, rotation: degrees)})]);
  }

  /// [changes], then those that keep the groups whose shapes they place
  /// the box of their shapes, as PowerPoint keeps them: the shapes stay
  /// where they are on the slide.
  Edit _fitted(List<Change> changes) {
    final after = tree.copy();
    if (after.apply(Edit(changes)) == null) return Edit(changes);
    final out = [...changes];
    int depth(Node n) {
      var d = 0;
      for (Node? p = n; p != null && p.type == 'grp'; p = after[p.parent]) {
        d++;
      }
      return d;
    }

    var groups = {for (final c in changes) ?after[after[c.id]?.parent ?? '']}.where((g) => g.type == 'grp').toList();
    while (groups.isNotEmpty) {
      groups.sort((a, b) => depth(b) - depth(a));
      final next = <Node>[];
      for (final g in groups) {
        final x = _fit(after, after[g.id]!);
        if (x == null) continue;
        final set = Change.set(g.id, attributes: {'xfrm': x});
        after.apply(Edit([set]));
        out.add(set);
        if (after[g.parent] case final parent? when parent.type == 'grp') next.add(parent);
      }
      groups = next;
    }
    return Edit(out);
  }

  /// The place of a group around its shapes, null when it is already.
  static Map<String, Object?>? _fit(Tree tree, Node group) {
    final x = group.attributes['xfrm'];
    if (x is! Map<String, Object?>) return null;
    double v(String k) => x[k] is num ? (x[k]! as num).toDouble() : 0;
    Rect? box;
    for (final child in tree.children(group.id)) {
      final c = child.attributes['xfrm'];
      if (c is! Map<String, Object?>) continue;
      final b = _turned(c);
      box = box == null ? b : box.expandToInclude(b);
    }
    if (box == null || (box.left - v('cx')).abs() < 1 && (box.top - v('cy')).abs() < 1 && (box.width - v('cw')).abs() < 1 && (box.height - v('ch')).abs() < 1) return null;
    final sx = v('cw') > 0 ? v('w') / v('cw') : 1.0, sy = v('ch') > 0 ? v('h') / v('ch') : 1.0;
    final (w, h) = (box.width * sx, box.height * sy);
    // the center of the shapes, where the group's box turns about now
    final half = Offset(v('w') / 2, v('h') / 2);
    var q = Offset((box.center.dx - v('cx')) * sx, (box.center.dy - v('cy')) * sy) - half;
    q = Offset(x['flipH'] == true ? -q.dx : q.dx, x['flipV'] == true ? -q.dy : q.dy);
    final a = v('rot') / 60000 * math.pi / 180;
    final center = Offset(v('x'), v('y')) + half + Offset(q.dx * math.cos(a) - q.dy * math.sin(a), q.dx * math.sin(a) + q.dy * math.cos(a));
    return {
      ...x,
      'x': (center.dx - w / 2).round(),
      'y': (center.dy - h / 2).round(),
      'w': w.round(),
      'h': h.round(),
      'cx': box.left.round(),
      'cy': box.top.round(),
      'cw': box.width.round(),
      'ch': box.height.round(),
    };
  }

  /// The box a shape covers once turned, in the coordinates of its xfrm.
  static Rect _turned(Map<String, Object?> x) {
    double v(String k) => x[k] is num ? (x[k]! as num).toDouble() : 0;
    final r = Rect.fromLTWH(v('x'), v('y'), v('w'), v('h'));
    final a = v('rot') / 60000 * math.pi / 180;
    if (a == 0) return r;
    final (c, s) = (math.cos(a).abs(), math.sin(a).abs());
    return Rect.fromCenter(center: r.center, width: r.width * c + r.height * s, height: r.width * s + r.height * c);
  }

  /// A key after every child of [parent], or between two of them.
  String keyAfter(String parent, [Node? after]) {
    final kids = tree.children(parent).where((n) => n.type != 'notes').toList();
    if (after == null) return keyBetween(kids.isEmpty ? '' : kids.last.key, '');
    final i = kids.indexWhere((n) => n.id == after.id);
    final next = i + 1 < kids.length ? kids[i + 1].key : '';
    return keyBetween(after.key, next == after.key ? '' : next);
  }

  /// A key before every child of [parent].
  String keyFirst(String parent) {
    final kids = tree.children(parent).where((n) => n.type != 'notes').toList();
    return keyBetween('', kids.isEmpty ? '' : kids.first.key);
  }

  /// A new shape of a preset geometry, as PowerPoint draws them: the first
  /// accent of the theme, a darker outline, light text.
  Edit newShape(Node slide, String preset, Rect bounds, {required String name}) {
    final id = randomId();
    final node = Node(
      id: id,
      type: 'sp',
      parent: slide.id,
      key: keyAfter(slide.id),
      attributes: {
        'name': name,
        'geom': {'prst': preset},
        'xfrm': _xfrm(bounds),
        'style': {
          'ln': {
            'idx': '2',
            'color': {
              'scheme': 'accent1',
              'mods': [
                ['shade', 50000],
              ],
            },
          },
          'fill': {'idx': '1', 'color': {'scheme': 'accent1'}},
          'effect': {'idx': '0', 'color': {'scheme': 'accent1'}},
          'font': {'idx': 'minor', 'color': {'scheme': 'lt1'}},
        },
        'body': {'anchor': 'ctr', 'rtlCol': '0'},
      },
      text: Delta([const Op.insert('\n', {'algn': 'ctr'})]),
    );
    return Edit([Change.create(node)]);
  }

  /// A new text box, growing with its text.
  Edit newTextBox(Node slide, Rect bounds, {required String name}) => Edit([
    Change.create(Node(
      id: randomId(),
      type: 'sp',
      parent: slide.id,
      key: keyAfter(slide.id),
      attributes: {
        'name': name,
        'geom': {'prst': 'rect'},
        'xfrm': _xfrm(bounds),
        'fill': {'none': true},
        'body': {'wrap': 'square', 'fit': 'shape', 'rtlCol': '0'},
      },
      text: Delta([const Op.insert('\n')]),
    )),
  ]);

  Map<String, Object?> _xfrm(Rect r) {
    int emu(double v) => (v * emuPerPoint).round();
    return {'x': emu(r.left), 'y': emu(r.top), 'w': emu(r.width), 'h': emu(r.height)};
  }

  /// A new slide on a layout, with the placeholders the layout gives it,
  /// after [after] or last.
  Edit newSlide(Node layout, {Node? after}) {
    final id = randomId();
    final changes = <Change>[
      Change.create(Node(
        id: id,
        type: 'slide',
        parent: 'deck',
        key: after == null ? keyAfter('deck') : keyAfter('deck', after),
        attributes: {'layout': layout.id},
      )),
    ];
    var key = '';
    for (final ph in tree.children(layout.id)) {
      final p = ph.attributes['ph'];
      if (p is! Map<String, Object?> || const {'dt', 'ftr', 'sldNum', 'hdr'}.contains(p['type'])) continue;
      key = keyBetween(key, '');
      changes.add(Change.create(Node(
        id: randomId(),
        type: 'sp',
        parent: id,
        key: key,
        attributes: {
          'ph': {for (final e in p.entries) if (e.key == 'type' || e.key == 'idx' || e.key == 'orient' || e.key == 'sz') e.key: e.value},
          if (ph.attributes['name'] case final String name) 'name': name,
        },
        text: Delta([const Op.insert('\n')]),
      )));
    }
    return Edit(changes);
  }

  /// A copy of a slide right after it, its shapes under new ids.
  Edit duplicateSlide(Node slide) {
    final id = randomId();
    return Edit([
      Change.create(Node(id: id, type: 'slide', parent: 'deck', key: keyAfter('deck', slide), attributes: slide.attributes)),
      ...copies(tree.children(slide.id).where((n) => n.type != 'notes'), id),
    ]);
  }

  /// Copies of nodes and everything under them, under new ids, into
  /// [parent], moved by [shift] points.
  List<Change> copies(Iterable<Node> nodes, String parent, {Offset shift = Offset.zero, bool last = false}) {
    final out = <Change>[];
    var key = last ? keyAfter(parent) : '';
    void copy(Node n, String into, {String? at}) {
      final id = randomId();
      var attributes = n.attributes;
      if (shift != Offset.zero && into == parent && attributes['xfrm'] is Map<String, Object?>) {
        final x = {...attributes['xfrm']! as Map<String, Object?>};
        x['x'] = ((x['x'] as num? ?? 0) + shift.dx * emuPerPoint).round();
        x['y'] = ((x['y'] as num? ?? 0) + shift.dy * emuPerPoint).round();
        attributes = {...attributes, 'xfrm': x};
      }
      out.add(Change.create(Node(id: id, type: n.type, parent: into, key: at ?? n.key, attributes: attributes, text: n.text)));
      for (final c in tree.children(n.id)) {
        copy(c, id);
      }
    }

    for (final n in nodes) {
      if (last) {
        copy(n, parent, at: key);
        key = keyBetween(key, '');
      } else {
        copy(n, parent);
      }
    }
    return out;
  }

  Edit delete(Iterable<Node> nodes) => Edit([for (final n in nodes) Change.delete(n.id)]);

  /// Moves a child of its parent to [index] among the others of its kind.
  Edit reorder(Node node, int index) {
    final kids = tree.children(node.parent).where((n) => n.type != 'notes' && n.id != node.id).toList();
    final i = index.clamp(0, kids.length);
    final before = i > 0 ? kids[i - 1].key : '';
    final after = i < kids.length ? kids[i].key : '';
    if (before == after || (after.isNotEmpty && before.compareTo(after) >= 0)) {
      // equal keys: make room by moving the ones after
      return Edit([Change.set(node.id, key: keyBetween(before, ''))]);
    }
    return Edit([Change.set(node.id, key: keyBetween(before, after))]);
  }
}
