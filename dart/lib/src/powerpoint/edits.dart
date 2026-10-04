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

  /// Places shapes; each rectangle is in points, on the slide.
  Edit place(Map<Node, Rect> shapes, {double? rotation}) => Edit([
    for (final e in shapes.entries) Change.set(e.key.id, attributes: {'xfrm': xfrm(e.key, e.value, rotation: rotation)}),
  ]);

  Edit rotate(Node shape, double degrees) {
    final bounds = deck.styleOf(shape).bounds;
    if (bounds == null) return Edit();
    return Edit([Change.set(shape.id, attributes: {'xfrm': xfrm(shape, bounds, rotation: degrees)})]);
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
