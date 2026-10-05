import 'dart:convert';
import 'dart:ui';

import 'package:trame/trame.dart';

import '../drawing/color.dart';
import '../text/text_frame.dart';
import 'builtin_styles.dart';

/// Office's built-in table styles, by id.
final builtinTableStyles = jsonDecode(builtinTableStylesJson) as Map<String, Object?>;

/// EMU in a point: the positions and sizes of a document are in EMU,
/// what is drawn in points.
const emuPerPoint = 12700.0;

/// Whether a child of a slide is a shape, not its notes or a comment.
bool isShape(Node n) => n.type != 'notes' && n.type != 'comment';

/// A presentation as a session's tree holds it, read to be shown: what
/// each slide inherits from its layout and master.
class Deck {
  Deck(this.tree);

  final Tree tree;

  final _shapes = Expando<ShapeStyle>();

  /// The size of the slides, in points.
  Size get size {
    final s = _map(tree['deck']?.attributes['size']);
    final w = s?['w'], h = s?['h'];
    return w is num && h is num && w > 0 && h > 0 ? Size(w / emuPerPoint, h / emuPerPoint) : const Size(960, 540);
  }

  List<Node> get slides => [for (final n in tree.children('deck')) if (n.type == 'slide') n];

  Node? layoutOf(Node slide) {
    final id = slide.attributes['layout'];
    return id is String ? tree[id] : null;
  }

  /// The master of a slide or a layout.
  Node? masterOf(Node node) {
    final layout = node.type == 'layout' ? node : layoutOf(node);
    final master = layout == null ? null : tree[layout.parent];
    if (master != null) return master;
    for (final n in tree.children('')) {
      if (n.type == 'master') return n;
    }
    return null;
  }

  /// The theme of a slide, layout or master.
  DeckTheme themeOf(Node node) => DeckTheme(_map((node.type == 'master' ? node : masterOf(node))?.attributes['theme']));

  /// The colors of a slide, layout or master: its theme's, through the
  /// color map of the master and the overrides of the layout and slide.
  ColorContext colorsOf(Node node) {
    final master = node.type == 'master' ? node : masterOf(node);
    final map = <String, String>{..._strings(master?.attributes['clrMap'])};
    final layout = node.type == 'slide' ? layoutOf(node) : (node.type == 'layout' ? node : null);
    for (final n in [layout, if (node.type == 'slide') node]) {
      final o = _strings(n?.attributes['clrMapOvr']);
      if (o.isNotEmpty) {
        map
          ..clear()
          ..addAll(o);
      }
    }
    return ColorContext(scheme: themeOf(node).colors, map: map);
  }

  /// The background of a slide: its own, its layout's or its master's.
  Map<String, Object?>? backgroundOf(Node slide) {
    for (final n in [slide, layoutOf(slide), masterOf(slide)]) {
      final bg = _map(n?.attributes['bg']);
      if (bg != null) return bg;
    }
    return null;
  }

  /// The shapes a slide shows behind its own: those of its master and
  /// layout that are not placeholders, unless it hides them.
  List<Node> backdropOf(Node slide) {
    final out = <Node>[];
    final layout = layoutOf(slide);
    if (slide.attributes['showMasterSp'] == false) return out;
    final master = masterOf(slide);
    if (layout?.attributes['showMasterSp'] != false && master != null) {
      out.addAll(tree.children(master.id).where((n) => !_isPlaceholder(n)));
    }
    if (layout != null) out.addAll(tree.children(layout.id).where((n) => !_isPlaceholder(n)));
    return out;
  }

  /// The shapes of a slide, notes left out.
  List<Node> shapesOf(Node parent) => [for (final n in tree.children(parent.id)) if (isShape(n)) n];

  /// The slide, layout or master a shape is on.
  Node? pageOf(Node shape) {
    Node? n = shape;
    while (n != null && n.type != 'slide' && n.type != 'layout' && n.type != 'master') {
      n = tree[n.parent];
    }
    return n;
  }

  /// The parts of the style a table names, by name: "wholeTbl",
  /// "firstRow"…
  Map<String, Map<String, Object?>> tableStyleOf(Node table) => tableStyle(_map(table.attributes['tbl'])?['style']);

  /// The ids of the table styles the presentation holds.
  Iterable<String> get tableStyleIds => _map(tree['deck']?.attributes['tblStyles'])?.keys ?? const [];

  /// The parts of a table style by name, those of the presentation before
  /// Office's built-in ones.
  Map<String, Map<String, Object?>> tableStyle(Object? id) {
    final style = _map(_map(tree['deck']?.attributes['tblStyles'])?[id] ?? builtinTableStyles[id]);
    return {
      for (final e in (style ?? const {}).entries)
        if (e.value is Map<String, Object?>) e.key: e.value! as Map<String, Object?>,
    };
  }

  /// The list styles the text of a cell inherits, from the most general:
  /// those of the text that is neither title nor body.
  List<Map<String, Props>> cellLevels(Node cell) {
    final page = pageOf(cell);
    final master = page == null ? null : (page.type == 'master' ? page : masterOf(page));
    return [
      _levels(tree['deck']?.attributes['lst']),
      if (master != null) _levels(master.attributes['other']),
      _levels(cell.attributes['lst']),
    ];
  }

  /// What a shape looks like once it inherited from its placeholders.
  ShapeStyle styleOf(Node shape) => _shapes[shape] ??= _resolve(shape);

  ShapeStyle _resolve(Node shape) {
    final page = pageOf(shape);
    final chain = <Node>[shape];
    if (page != null && _isPlaceholder(shape)) {
      if (page.type == 'slide') {
        final layout = layoutOf(page);
        final fromLayout = layout == null ? null : _match(shape, layout, byIndex: true);
        if (fromLayout != null) chain.insert(0, fromLayout);
        final master = masterOf(page);
        final fromMaster = master == null ? null : _match(fromLayout ?? shape, master, byIndex: false);
        if (fromMaster != null) chain.insert(0, fromMaster);
      } else if (page.type == 'layout') {
        final master = masterOf(page);
        final fromMaster = master == null ? null : _match(shape, master, byIndex: false);
        if (fromMaster != null) chain.insert(0, fromMaster);
      }
    }
    Object? last(String key) {
      for (final n in chain.reversed) {
        final v = n.attributes[key];
        if (v != null) return v;
      }
      return null;
    }

    final body = <String, String>{};
    for (final n in chain) {
      body.addAll(_strings(n.attributes['body']));
    }
    final type = _placeholderType(chain.last);
    final master = page == null ? null : (page.type == 'master' ? page : masterOf(page));
    final deckStyle = _levels(tree['deck']?.attributes['lst']);
    final levels = <Map<String, Props>>[deckStyle];
    if (master != null) {
      levels.add(_levels(master.attributes[switch (type) {
        'title' => 'title',
        'body' => 'body',
        _ => 'other',
      }]));
    }
    for (final n in chain) {
      levels.add(_levels(n.attributes['lst']));
    }
    return ShapeStyle(
      xfrm: _map(last('xfrm')),
      geometry: _map(last('geom')),
      fill: _map(last('fill')),
      line: _map(last('line')),
      style: _map(last('style')),
      body: body,
      levels: levels,
      placeholder: type,
    );
  }

  /// The placeholder of container a shape inherits from: the one with its
  /// index on a layout, else the one of its kind.
  Node? _match(Node shape, Node container, {required bool byIndex}) {
    final ph = _map(shape.attributes['ph']) ?? const {};
    final idx = ph['idx'] ?? '0';
    final type = _kind(ph['type'] as String? ?? 'obj');
    final candidates = tree.children(container.id).where(_isPlaceholder).toList();
    if (byIndex) {
      for (final c in candidates) {
        if ((_map(c.attributes['ph'])?['idx'] ?? '0') == idx && (idx != '0' || _kind(_phType(c)) == type)) return c;
      }
    }
    for (final c in candidates) {
      if (_kind(_phType(c)) == type) return c;
    }
    return null;
  }
}

/// What a shape looks like, what it inherited included. Values are as the
/// Go package writes them.
class ShapeStyle {
  const ShapeStyle({
    this.xfrm,
    this.geometry,
    this.fill,
    this.line,
    this.style,
    this.body = const {},
    this.levels = const [],
    this.placeholder,
  });

  final Map<String, Object?>? xfrm;
  final Map<String, Object?>? geometry;
  final Map<String, Object?>? fill;
  final Map<String, Object?>? line;
  final Map<String, Object?>? style;
  final Props body;

  /// The list styles text inherits, from the most general.
  final List<Map<String, Props>> levels;

  /// "title", "body" or another kind for a placeholder.
  final String? placeholder;

  /// Where the shape is, in points, null when nothing places it.
  Rect? get bounds {
    final x = xfrm;
    if (x == null) return null;
    double v(String k) => (x[k] is num ? (x[k]! as num).toDouble() : 0) / emuPerPoint;
    return Rect.fromLTWH(v('x'), v('y'), v('w'), v('h'));
  }

  /// The paragraph and run properties a paragraph of that level starts
  /// from.
  Props level(int lvl) {
    final out = <String, String>{};
    for (final l in levels) {
      out.addAll(l['0'] ?? const {});
      out.addAll(l['${lvl + 1}'] ?? const {});
    }
    return out;
  }
}

/// A master's theme: its colors, fonts and the fills and lines its styles
/// refer to.
class DeckTheme {
  DeckTheme(Map<String, Object?>? json)
    : colors = {
        for (final e in (_map(json?['colors']) ?? const {}).entries)
          e.key: ?const ColorContext().resolve(e.value),
      },
      fonts = {
        for (final e in (_map(json?['fonts']) ?? const {}).entries) e.key: _strings(e.value),
      },
      fills = _list(json?['fills']),
      lines = _list(json?['lines']),
      backgroundFills = _list(json?['bgFills']);

  final Map<String, Color> colors;
  final Map<String, Map<String, String>> fonts;
  final List<Object?> fills;
  final List<Object?> lines;
  final List<Object?> backgroundFills;

  /// The typeface a font of the text names: "+mj-lt" is the major latin
  /// font of the theme, "+mn-ea" the minor east asian one.
  String typeface(String name) {
    if (!name.startsWith('+')) return name;
    final kind = name.startsWith('+mj') ? 'major' : 'minor';
    final script = switch (name.substring(name.length - 2)) {
      'ea' => 'ea',
      'cs' => 'cs',
      _ => 'latin',
    };
    return fonts[kind]?[script] ?? fonts[kind]?['latin'] ?? 'Calibri';
  }
}

bool _isPlaceholder(Node n) => n.attributes['ph'] != null;

String _phType(Node n) => _map(n.attributes['ph'])?['type'] as String? ?? 'obj';

/// The kind of placeholder a type stands for on a layout.
String _kind(String type) => switch (type) {
  'ctrTitle' => 'title',
  'subTitle' || 'obj' || 'body' || 'chart' || 'tbl' || 'clipArt' || 'dgm' || 'media' || 'pic' => 'body',
  _ => type,
};

String? _placeholderType(Node n) {
  if (!_isPlaceholder(n)) return null;
  return _kind(_phType(n));
}

Map<String, Object?>? _map(Object? v) => v is Map<String, Object?> ? v : null;

List<Object?> _list(Object? v) => v is List<Object?> ? v : const [];

Map<String, String> _strings(Object? v) => {
  if (v is Map<String, Object?>)
    for (final e in v.entries)
      if (e.value is String) e.key: e.value! as String,
};

Map<String, Props> _levels(Object? v) => {
  if (v is Map<String, Object?>)
    for (final e in v.entries) e.key: _strings(e.value),
};
