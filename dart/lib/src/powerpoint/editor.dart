import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/ribbon.dart';
import '../chrome/strings.dart';
import '../drawing/geometry.dart';
import '../plain_text_editor.dart';
import '../text/editing.dart';
import '../text/text_frame.dart';
import 'deck.dart';
import 'edits.dart';
import 'slide_canvas.dart';
import 'slide_painter.dart';
import 'slideshow.dart';
import 'table.dart';
import 'table_edits.dart';
import 'table_gallery.dart';

/// A PowerPoint editor on a session whose document is a presentation: the
/// ribbon, the slides at the left, the slide being edited with its notes,
/// the status bar.
class PresentationEditor extends StatefulWidget {
  const PresentationEditor({
    super.key,
    required this.session,
    required this.media,
    this.title = '',
    this.onClose,
    this.strings = const LofficeStrings(),
    this.fonts,
  });

  final DocSession session;

  /// Fetches the pictures of the document by their name.
  final MediaFetcher media;
  final String title;
  final VoidCallback? onClose;
  final LofficeStrings strings;

  /// The package bundling the free fonts standing in for Office's.
  final String? fonts;

  @override
  State<PresentationEditor> createState() => _PresentationEditorState();
}

class _PresentationEditorState extends State<PresentationEditor> {
  late final MediaCache _media = MediaCache(widget.media)..addListener(_repaint);
  final _selection = SlideSelection();
  final _canvasFocus = FocusNode();
  StreamSubscription<String>? _rejections;
  StreamSubscription<Edit>? _changes;
  String? _slideId;
  var _notes = true;
  var _sorter = false;
  var _backstage = false;
  List<Node> _clipboard = const [];
  var _pastes = 0;

  Deck? _deck;
  SlidePainter? _painter;
  Tree? _painted;

  DocSession get _session => widget.session;
  LofficeStrings get _s => widget.strings;

  @override
  void initState() {
    super.initState();
    _session.addListener(_repaint);
    _selection.addListener(_repaint);
    _changes = _session.changes.listen(_changed);
    _rejections = _session.rejections.listen((reason) {
      if (!mounted) return;
      ScaffoldMessenger.maybeOf(context)?.showSnackBar(SnackBar(content: Text(_s.refused(reason))));
    });
  }

  @override
  void dispose() {
    _session.removeListener(_repaint);
    _selection.removeListener(_repaint);
    unawaited(_rejections?.cancel());
    unawaited(_changes?.cancel());
    _media
      ..removeListener(_repaint)
      ..dispose();
    _selection.dispose();
    _canvasFocus.dispose();
    super.dispose();
  }

  void _repaint() {
    if (mounted) setState(() {});
  }

  /// The deck of the document, which keeps what it resolved and laid out
  /// for each node: a node changed is a new one.
  Deck get _current {
    final tree = _session.document;
    if (_painted != tree || _deck == null) {
      _deck = Deck(tree);
      _painter = SlidePainter(_deck!, images: (m) => _media[m], fonts: widget.fonts);
      _painted = tree;
    }
    return _deck!;
  }

  SlidePainter get _paint {
    _current;
    return _painter!;
  }

  /// Forgets what was resolved when a slide changes: its shapes may inherit
  /// from another layout.
  void _changed(Edit edit) {
    if (edit.changes.any((c) => c.type == 'slide' || _session.document[c.id]?.type == 'slide')) _deck = null;
  }

  bool _edit(Edit edit) {
    if (edit.isEmpty) return false;
    return _session.edit(edit);
  }

  Node? get _slide {
    final slides = _current.slides;
    if (slides.isEmpty) return null;
    return slides.firstWhere((s) => s.id == _slideId, orElse: () => slides.first);
  }

  List<Node> get _selectedShapes => [for (final id in _selection.shapes) ?_session.document[id]];

  // formatting

  /// The node whose text formatting applies to, and the range: the text
  /// being edited, or the whole text of the first shape selected.
  (Node, int, int)? get _textTarget {
    final editing = _selection.editing;
    if (editing != null) {
      final n = _session.document[editing];
      if (n != null) return (n, _selection.start, _selection.end);
    }
    for (final n in _selectedShapes) {
      if (_hasText(n)) return (n, 0, n.text!.length - 1);
    }
    return null;
  }

  /// Whether the formatting of a shape selected goes to its whole text.
  bool _hasText(Node n) => (n.type == 'sp' || n.type == 'tc') && n.text != null;

  /// The properties a paragraph of a level of a shape or cell starts from.
  Props _levelOf(Node node, int lvl) => node.type == 'tc' ? _paint.cellOf(node)?.level(lvl) ?? const {} : _current.styleOf(node).level(lvl);

  /// The run formatting in effect where the selection starts, what is
  /// inherited included.
  Props get _runProps {
    final target = _textTarget;
    if (target == null) return const {};
    final (node, start, _) = target;
    final editor = FlowEditing(node.text!);
    final lvl = int.tryParse(editor.markAt(start)['lvl'] ?? '') ?? 0;
    final level = _levelOf(node, lvl);
    final own = _selection.editing == node.id && _selection.collapsed
        ? (_selection.typing ?? editor.typingAttributes(start))
        : (editor.attributesAt(start) ?? const <String, String>{});
    return {
      for (final e in {...level, ...own}.entries)
        if (!paraKeys.contains(e.key)) e.key: e.value,
    };
  }

  Props get _paraProps {
    final target = _textTarget;
    if (target == null) return const {};
    final (node, start, _) = target;
    final mark = FlowEditing(node.text!).markAt(start);
    final lvl = int.tryParse(mark['lvl'] ?? '') ?? 0;
    return {..._levelOf(node, lvl), ...mark};
  }

  /// Sets run attributes ("" removes) on the selection, or on the text of
  /// the shapes selected; at a caret, on what is typed next.
  void _formatRuns(Attributes attributes) {
    final editing = _selection.editing;
    if (editing != null) {
      final node = _session.document[editing];
      if (node == null) return;
      final editor = FlowEditing(node.text!);
      if (_selection.collapsed) {
        _selection.typing = {...(_selection.typing ?? editor.typingAttributes(_selection.start)), ...attributes}
          ..removeWhere((_, v) => v.isEmpty);
        _repaint();
        return;
      }
      final d = editor.formatRuns(_selection.start, _selection.end, attributes);
      if (d != null) _edit(Edit([Change.text(node.id, d)]));
      return;
    }
    _edit(Edit([
      for (final n in _selectedShapes)
        if (_hasText(n)) Change.text(n.id, FlowEditing(n.text!).formatRuns(0, n.text!.length, attributes)!),
    ]));
  }

  void _formatParagraphs(Attributes attributes) {
    final editing = _selection.editing;
    if (editing != null) {
      final node = _session.document[editing];
      if (node != null) {
        _edit(Edit([Change.text(node.id, FlowEditing(node.text!).formatParagraphs(_selection.start, _selection.end, attributes))]));
      }
      return;
    }
    _edit(Edit([
      for (final n in _selectedShapes)
        if (_hasText(n)) Change.text(n.id, FlowEditing(n.text!).formatParagraphs(0, n.text!.length - 1, attributes)),
    ]));
  }

  void _toggle(String key, String on, String off) => _formatRuns({key: _runProps[key] == on ? off : on});

  static const _sizes = [8, 9, 10, 10.5, 11, 12, 14, 16, 18, 20, 24, 28, 32, 36, 40, 44, 48, 54, 60, 66, 72, 80, 88, 96];

  double get _size => (double.tryParse(_runProps['sz'] ?? '') ?? 1800) / 100;

  void _setSize(double pt) => _formatRuns({'sz': '${(pt * 100).round()}'});

  void _growFont(bool up) {
    final now = _size;
    final next = up
        ? _sizes.firstWhere((s) => s > now + 0.01, orElse: () => now + 12)
        : _sizes.lastWhere((s) => s < now - 0.01, orElse: () => now > 1 ? now - 1 : now);
    _setSize(next.toDouble());
  }

  /// Fills the shapes selected, or draws their outline, in [color]; a
  /// table selected, its cells.
  void _fillShapes(String key, Map<String, Object?>? color) {
    const fills = {'sp', 'pic', 'grp', 'tc'}, lines = {'sp', 'pic', 'cxn', 'tc'};
    final shapes = [
      for (final n in _selectedShapes)
        if (n.attributes['frame'] == 'table')
          for (final c in _paint.tableOf(n)?.cells.values ?? const <CellLayout>[]) c.node
        else if ((key == 'fill' ? fills : lines).contains(n.type))
          n,
    ];
    Map<String, Object?> line(Object? old) => color == null
        ? {'fill': {'none': true}}
        : {...?(old as Map<String, Object?>?), 'fill': {'solid': color}};
    _edit(Edit([
      for (final n in shapes)
        if (key == 'fill')
          Change.set(n.id, attributes: {'fill': color == null ? {'none': true} : {'solid': color}})
        else if (n.type == 'tc')
          Change.set(n.id, attributes: {for (final k in ['lnL', 'lnR', 'lnT', 'lnB']) k: line(n.attributes[k])})
        else
          Change.set(n.id, attributes: {'line': line(n.attributes['line'])}),
    ]));
  }

  // slides and shapes

  void _view({required bool sorter}) => setState(() => _sorter = sorter);

  void _closeBackstage() => setState(() => _backstage = false);

  void _goTo(Node slide) {
    if (_slideId == slide.id) return;
    _selection.clear();
    setState(() => _slideId = slide.id);
  }

  void _step(int by) {
    final slides = _current.slides;
    final i = slides.indexWhere((s) => s.id == _slide?.id);
    final to = (i + by).clamp(0, slides.length - 1);
    if (slides.isNotEmpty) _goTo(slides[to]);
  }

  List<Node> get _layouts => [
    for (final m in _session.document.children(''))
      if (m.type == 'master') ..._session.document.children(m.id).where((l) => l.type == 'layout'),
  ];

  Node? get _defaultLayout {
    final layouts = _layouts;
    return layouts.where((l) => l.attributes['type'] == 'obj').firstOrNull ??
        (_slide == null ? null : _current.layoutOf(_slide!)) ??
        layouts.firstOrNull;
  }

  void _newSlide([Node? layout]) {
    layout ??= _defaultLayout;
    if (layout == null) return;
    final edit = DeckEdits(_current).newSlide(layout, after: _slide);
    if (_edit(edit)) setState(() => _slideId = edit.changes.first.id);
  }

  void _duplicateSlide() {
    final slide = _slide;
    if (slide == null) return;
    final edit = DeckEdits(_current).duplicateSlide(slide);
    if (_edit(edit)) setState(() => _slideId = edit.changes.first.id);
  }

  void _deleteSlide() {
    final slide = _slide;
    if (slide == null) return;
    final slides = _current.slides;
    final i = slides.indexWhere((s) => s.id == slide.id);
    if (_edit(DeckEdits(_current).delete([slide]))) {
      final rest = _current.slides;
      setState(() => _slideId = rest.isEmpty ? null : rest[i.clamp(0, rest.length - 1)].id);
    }
  }

  void _hideSlide() {
    final slide = _slide;
    if (slide == null) return;
    _edit(Edit([Change.set(slide.id, attributes: {'hidden': slide.attributes['hidden'] == true ? null : true})]));
  }

  /// The number of the transition preview playing over the slide, 0 for
  /// none.
  var _previews = 0;

  void _preview() => setState(() => _previews++);

  /// The transition of the slide shown, empty for none.
  Map<String, Object?> get _transition => _slide?.attributes['transition'] as Map<String, Object?>? ?? const {};

  /// Changes the transition of the slide shown, or of all of them; a null
  /// value removes a key.
  void _retransition(Map<String, Object?> changes, {bool all = false}) {
    final slide = _slide;
    if (slide == null) return;
    final t = {'dur': 500, ..._transition, ...changes}..removeWhere((_, v) => v == null);
    final keep = t.keys.any((k) => k != 'dur');
    _edit(Edit([
      for (final s in all ? _current.slides : [slide]) Change.set(s.id, attributes: {'transition': keep ? t : null}),
    ]));
  }

  /// The options of a transition effect, the first PowerPoint's default:
  /// how [LofficeStrings.effectOption] names them, and the keys they set.
  static List<(String, Map<String, Object?>)> _effectOptions(Object? effect) => switch (effect) {
    'push' => [for (final d in ['u', 'l', 'r', 'd']) (d, {'dir': d})],
    'wipe' => [for (final d in ['l', 'u', 'r', 'd']) (d, {'dir': d})],
    'cover' || 'pull' => [for (final d in ['l', 'u', 'r', 'd', 'lu', 'ru', 'ld', 'rd']) (d, {'dir': d})],
    'split' => [for (final o in ['vert', 'horz']) for (final d in ['out', 'in']) ('$o $d', {'orient': o, 'dir': d})],
    'zoom' => [('in', {'dir': 'in'}), ('out', {'dir': 'out'})],
    'fade' => [('smooth', {'thruBlk': null}), ('thruBlk', {'thruBlk': true})],
    _ => [],
  };

  static const _transitionDurations = {'cut': 0, 'fade': 700, 'push': 1000, 'wipe': 1000, 'split': 1500, 'pull': 1000, 'cover': 1000, 'zoom': 500};

  void _pickTransition(String effect) {
    final options = _effectOptions(effect);
    _retransition({
      'effect': effect.isEmpty ? null : effect,
      'dir': null,
      'orient': null,
      'thruBlk': null,
      'dur': _transitionDurations[effect] ?? 500,
      if (options.isNotEmpty) ...options.first.$2,
    });
    if (effect.isNotEmpty && effect != 'cut') _preview();
  }

  Rect get _center {
    final size = _current.size;
    return Rect.fromCenter(center: size.center(Offset.zero), width: 144, height: 108);
  }

  void _insertShape(String preset) {
    final slide = _slide;
    if (slide == null) return;
    final edit = DeckEdits(_current).newShape(slide, preset, _center, name: '${_s.shapeName} ${_current.shapesOf(slide).length + 1}');
    if (_edit(edit)) {
      _selection.selectShapes([edit.changes.single.id]);
      _canvasFocus.requestFocus();
    }
  }

  void _insertTextBox() {
    final slide = _slide;
    if (slide == null) return;
    final size = _current.size;
    final box = Rect.fromCenter(center: size.center(Offset.zero), width: 216, height: 28);
    final edit = DeckEdits(_current).newTextBox(slide, box, name: '${_s.textBoxName} ${_current.shapesOf(slide).length + 1}');
    if (_edit(edit)) {
      _selection.edit(edit.changes.single.id, 0);
      _canvasFocus.requestFocus();
    }
  }

  // tables

  /// The table selected, or the one whose cells are selected, with the
  /// first of them.
  (Node, CellLayout?)? get _table {
    final shapes = _selectedShapes;
    if (shapes.isEmpty) return null;
    final n = shapes.first;
    if (n.type == 'tc') {
      final frame = _paint.frameOf(n);
      if (frame == null || shapes.any((c) => c.type != 'tc' || _paint.frameOf(c)?.id != frame.id)) return null;
      return (frame, _paint.cellOf(n));
    }
    return shapes.length == 1 && n.attributes['frame'] == 'table' ? (n, null) : null;
  }

  /// The cells selected, of the table [_table] gives.
  List<CellLayout> get _cells => _table == null ? const [] : [for (final n in _selectedShapes) ?_paint.cellOf(n)];

  /// The cells selected, or all those of the table selected.
  List<CellLayout> get _tableCells {
    final table = _table;
    if (table == null) return const [];
    return table.$2 == null ? [...?_paint.tableOf(table.$1)?.cells.values] : _cells;
  }

  /// The line the Borders menu draws.
  var _pen = <String, Object?>{'w': 12700, 'fill': {'solid': {'rgb': '000000'}}};

  static const _penWeights = [3175, 6350, 9525, 12700, 19050, 28575, 38100, 57150, 76200];

  /// A width in EMU as PowerPoint writes it in points: "¼", "1½"…
  static String _points(int emu) {
    final quarters = (emu / 3175).round();
    const fractions = ['', '¼', '½', '¾'];
    final whole = quarters ~/ 4;
    return '${whole == 0 && quarters % 4 != 0 ? '' : whole}${fractions[quarters % 4]}';
  }

  /// The shapes and cells the direction of text goes to: the one being
  /// edited, or those selected.
  List<Node> get _textBoxes => [
    for (final n in _selectedShapes)
      if (n.attributes['frame'] == 'table') ...[for (final c in _paint.tableOf(n)?.cells.values ?? const <CellLayout>[]) c.node] else if (_hasText(n)) n,
  ];

  /// The direction of the text of the first of them: "horz", "vert"…
  String get _direction {
    final n = _textBoxes.firstOrNull;
    if (n == null) return 'horz';
    final vert = n.type == 'tc' ? n.attributes['vert'] : _current.styleOf(n).body['vert'];
    return vert is String ? vert : 'horz';
  }

  /// Sets the direction of the text; a placeholder is set horizontal
  /// explicitly, against what its layout may give it.
  void _setDirection(String vert) {
    _edit(Edit([
      for (final n in _textBoxes)
        if (n.type == 'tc')
          Change.set(n.id, attributes: {'vert': vert == 'horz' ? null : vert})
        else
          Change.set(n.id, attributes: {
            'body': {
              ...?(n.attributes['body'] as Map<String, Object?>?),
              'vert': vert,
            }..removeWhere((k, v) => k == 'vert' && v == 'horz' && n.attributes['ph'] == null),
          }),
    ]));
  }

  /// Anchors the text of the cells selected, or of all the table's.
  void _anchorCells(String anchor) {
    _edit(Edit([for (final c in _tableCells) Change.set(c.node.id, attributes: {'anchor': anchor})]));
  }

  void _merge() {
    final cells = _cells;
    _tableEdit((e, _) => e.merge(cells));
    if (cells.isNotEmpty) _selection.selectShapes([cells.first.node.id]);
  }

  /// Splits a merged cell back, or one that is not into the columns and
  /// rows asked for.
  Future<void> _splitCells() async {
    final cell = _table?.$2;
    if (cell == null) return;
    if (cell.rowSpan > 1 || cell.colSpan > 1) return _tableEdit((e, c) => e.split(c!));
    final size = await showDialog<(int, int)>(context: context, builder: (_) => _SplitDialog(strings: _s));
    if (size != null) _tableEdit((e, c) => e.splitInto(c!, size.$1, size.$2));
  }

  /// A new table in the middle of the slide, two thirds of its width, its
  /// first cell ready for typing.
  void _insertTable(int rows, int columns) {
    final slide = _slide;
    if (slide == null) return;
    final size = _current.size;
    final width = size.width * 2 / 3, height = rows * 370840 / emuPerPoint;
    final box = Rect.fromLTWH((size.width - width) / 2, (size.height - height) / 2, width, height);
    final name = '${_s.table} ${_current.shapesOf(slide).length + 1}';
    final edit = newTable(_current, slide, rows, columns, box, name: name, key: DeckEdits(_current).keyAfter(slide.id));
    if (_edit(edit)) {
      _selection.edit(edit.changes.firstWhere((c) => c.type == 'tc').id, 0);
      _canvasFocus.requestFocus();
    }
  }

  void _tableEdit(Edit Function(TableEdits edits, CellLayout? cell) make) {
    final table = _table;
    final layout = table == null ? null : _paint.tableOf(table.$1);
    if (table == null || layout == null) return;
    _edit(make(TableEdits(_session.document, table.$1, layout), table.$2));
    if (_session.document[table.$1.id] == null) _selection.clear();
  }

  void _copy({bool cut = false}) {
    final shapes = _selectedShapes.where((n) => n.parent == _slide?.id).toList();
    if (shapes.isEmpty) return;
    _clipboard = shapes;
    _pastes = 0;
    if (cut) {
      _edit(DeckEdits(_current).delete(shapes));
      _selection.clear();
    }
  }

  void _paste() {
    final slide = _slide;
    if (slide == null || _clipboard.isEmpty) return;
    _pastes++;
    final edits = DeckEdits(_current);
    final changes = edits.copies(_clipboard, slide.id, shift: Offset(9.0 * _pastes, 9.0 * _pastes), last: true);
    if (_edit(Edit(changes))) {
      _selection.selectShapes([for (final c in changes) if (c.parent == slide.id) c.id]);
    }
  }

  void _duplicateShapes() {
    _copy();
    _pastes = 0;
    _paste();
  }

  void _arrange(bool forward, {bool all = false}) {
    final shapes = _selectedShapes.where((n) => n.parent == _slide?.id || _session.document[n.parent]?.type == 'grp').toList();
    if (shapes.length != 1) return;
    final n = shapes.single;
    final siblings = _current.shapesOf(_session.document[n.parent]!);
    final i = siblings.indexWhere((s) => s.id == n.id);
    final to = all ? (forward ? siblings.length : 0) : (forward ? i + 1 : i - 1);
    _edit(DeckEdits(_current).reorder(n, to));
  }

  Future<void> _slideshow({bool fromCurrent = false, bool presenter = false}) async {
    final slides = _current.slides;
    final start = fromCurrent ? slides.indexWhere((s) => s.id == _slide?.id) : 0;
    _selection.clear();
    await Slideshow.show(
      context,
      Slideshow(deck: _current, painter: _paint, start: start < 0 ? 0 : start, strings: _s, images: _media, presenter: presenter),
    );
  }

  /// The keys of Office the editor answers wherever the focus is.
  bool _shortcut(KeyEvent e) {
    if (e is KeyUpEvent) return false;
    final ctrl = HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed;
    final shift = HardwareKeyboard.instance.isShiftPressed;
    final key = e.logicalKey;
    final editing = _selection.editing != null;
    if (key == LogicalKeyboardKey.f5) {
      final alt = HardwareKeyboard.instance.isAltPressed;
      unawaited(_slideshow(fromCurrent: shift || alt, presenter: alt));
      return true;
    }
    if (!ctrl) {
      if (!editing && _selection.shapes.isEmpty) {
        if (key == LogicalKeyboardKey.pageDown || key == LogicalKeyboardKey.arrowDown) {
          _step(1);
          return true;
        }
        if (key == LogicalKeyboardKey.pageUp || key == LogicalKeyboardKey.arrowUp) {
          _step(-1);
          return true;
        }
      }
      return false;
    }
    switch (key) {
      case LogicalKeyboardKey.keyZ:
        _session.undo();
        return true;
      case LogicalKeyboardKey.keyY:
        _session.redo();
        return true;
      case LogicalKeyboardKey.keyM:
        _newSlide();
        return true;
      // Gras is Ctrl+G in the French version, Ctrl+B elsewhere
      case LogicalKeyboardKey.keyB || LogicalKeyboardKey.keyG:
        _toggle('b', '1', '0');
        return true;
      case LogicalKeyboardKey.keyI:
        _toggle('i', '1', '0');
        return true;
      case LogicalKeyboardKey.keyU:
        _toggle('u', 'sng', 'none');
        return true;
      case LogicalKeyboardKey.keyE:
        _formatParagraphs({'algn': 'ctr'});
        return true;
      case LogicalKeyboardKey.keyL:
        _formatParagraphs({'algn': 'l'});
        return true;
      case LogicalKeyboardKey.keyR:
        _formatParagraphs({'algn': 'r'});
        return true;
      case LogicalKeyboardKey.keyJ:
        _formatParagraphs({'algn': 'just'});
        return true;
      case LogicalKeyboardKey.bracketRight || LogicalKeyboardKey.greater:
        _growFont(true);
        return true;
      case LogicalKeyboardKey.bracketLeft || LogicalKeyboardKey.less:
        _growFont(false);
        return true;
    }
    if (editing) return false;
    switch (key) {
      case LogicalKeyboardKey.keyC:
        _copy();
        return true;
      case LogicalKeyboardKey.keyX:
        _copy(cut: true);
        return true;
      case LogicalKeyboardKey.keyV:
        _paste();
        return true;
      case LogicalKeyboardKey.keyD:
        _duplicateShapes();
        return true;
    }
    return false;
  }

  // building

  @override
  Widget build(BuildContext context) {
    final status = _session.status;
    if (!_session.loaded) {
      final failure = _session.failure;
      return Center(
        child: status == DocStatus.closed
            ? _Failure(message: '$failure', retry: _s.retry, onRetry: _session.retry)
            : const CircularProgressIndicator(),
      );
    }
    final deck = _current;
    final slide = _slide;
    _slideId = slide?.id;
    final slides = deck.slides;
    final at = slide == null ? -1 : slides.indexWhere((s) => s.id == slide.id);
    final canvas = ColoredBox(
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      child: slide == null
          ? const SizedBox.expand()
          : Stack(children: [
              Positioned.fill(
                child: SlideCanvas(
                  session: _session,
                  deck: deck,
                  slide: slide,
                  painter: _paint,
                  selection: _selection,
                  focusNode: _canvasFocus,
                  strings: _s,
                  onShortcut: _shortcut,
                ),
              ),
              if (_previews > 0)
                Positioned.fill(
                  child: TransitionPreview(
                    key: ValueKey(_previews),
                    deck: deck,
                    painter: _paint,
                    slide: slide,
                    from: at > 0 ? slides[at - 1] : null,
                    onDone: () => setState(() => _previews = 0),
                  ),
                ),
            ]),
    );
    // a phone gives the slide its whole width, the thumbnails in a row below
    final phone = MediaQuery.sizeOf(context).width < phoneWidth;
    return Focus(
      autofocus: true,
      onKeyEvent: (_, e) => _shortcut(e) ? KeyEventResult.handled : KeyEventResult.ignored,
      child: Stack(
        children: [
          Column(
            children: [
              _ribbon(context, deck),
              ..._banners(context),
              Expanded(
                child: _sorter
                    ? _Sorter(editor: this)
                    : phone
                    ? Column(
                        children: [
                          Expanded(child: canvas),
                          const Divider(height: 1),
                          SizedBox(height: 88, child: _Thumbnails(editor: this, horizontal: true)),
                        ],
                      )
                    : Row(
                        children: [
                          SizedBox(width: 168, child: _Thumbnails(editor: this)),
                          const VerticalDivider(width: 1),
                          Expanded(
                            child: Column(
                              children: [
                                Expanded(child: canvas),
                                if (_notes && slide != null) _notesPane(context, slide),
                              ],
                            ),
                          ),
                        ],
                      ),
              ),
              _statusBar(context),
            ],
          ),
          if (_backstage) _Backstage(editor: this),
        ],
      ),
    );
  }

  List<Widget> _banners(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Widget banner(IconData icon, String text, Color color, {VoidCallback? action, String? label}) => Material(
      color: color,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        child: Row(children: [
          Icon(icon, size: 18),
          const SizedBox(width: 8),
          Expanded(child: Text(text)),
          if (action != null) TextButton(onPressed: action, child: Text(label!)),
        ]),
      ),
    );
    final error = _session.saveError;
    return [
      if (error != null) banner(Icons.error_outline, _s.saveFailed(error), scheme.errorContainer),
      if (_session.status == DocStatus.offline)
        banner(Icons.cloud_off, _s.offline, scheme.tertiaryContainer, action: _session.retry, label: _s.retry),
      if (_session.status == DocStatus.closed)
        banner(Icons.block, '${_session.failure ?? ''}', scheme.errorContainer),
      if (_session.readOnly) banner(Icons.visibility_outlined, _s.readOnly, scheme.secondaryContainer),
    ];
  }

  Widget _ribbon(BuildContext context, Deck deck) {
    final editable = !_session.readOnly;
    final hasText = editable && _textTarget != null;
    final shapes = editable && _selection.shapes.isNotEmpty;
    final table = _table;
    final cell = editable ? table?.$2 : null;
    final cells = editable ? _cells : const <CellLayout>[];
    final options = table?.$1.attributes['tbl'] as Map<String, Object?>? ?? const {};
    Widget option(String label, String key) => RibbonCheck(
      label: label,
      value: options[key] == true,
      onChanged: editable ? (_) => _tableEdit((e, _) => e.toggle(key)) : null,
    );
    final run = _runProps;
    final para = _paraProps;
    final colors = _slide == null ? const <String, Color>{} : deck.themeOf(_slide!).colors;
    final theme = _slide == null ? null : deck.themeOf(_slide!);
    final fonts = <String>{
      '+mj-lt', '+mn-lt', 'Calibri', 'Calibri Light', 'Cambria', 'Arial', 'Times New Roman', 'Courier New', //
      if (run['font'] != null) run['font']!,
    }.toList();
    String fontName(String f) => f == '+mj-lt'
        ? '${theme?.typeface(f) ?? 'Calibri Light'} (Titres)'
        : f == '+mn-lt'
        ? '${theme?.typeface(f) ?? 'Calibri'} (Corps)'
        : f;
    PopupMenuEntry<Map<String, Object?>?> palette(ValueChanged<Map<String, Object?>?> pick, {String? none}) =>
        PopupMenuItem(
          enabled: false,
          padding: EdgeInsets.zero,
          child: Builder(
            builder: (context) => ColorPalette(
              theme: colors,
              themeLabel: _s.themeColors,
              standardLabel: _s.standardColors,
              noneLabel: none,
              onSelected: (c) {
                Navigator.of(context).pop();
                pick(c);
              },
            ),
          ),
        );

    Widget direction({bool large = false}) => RibbonMenu<String>(
      icon: const Icon(Icons.text_rotate_vertical),
      label: _s.textDirection,
      large: large,
      enabled: editable && _textBoxes.isNotEmpty,
      items: [
        for (final v in ['horz', 'vert', 'vert270'])
          CheckedPopupMenuItem(value: v, checked: _direction == v, child: Text(_s.direction(v))),
      ],
      onSelected: _setDirection,
    );

    final layouts = _layouts;
    return Ribbon(
      fileLabel: _s.file,
      onFile: () => setState(() => _backstage = true),
      leading: [
        IconButton(
          tooltip: '${_s.undo} (Ctrl+Z)',
          iconSize: 18,
          onPressed: _session.canUndo ? () => setState(() => _session.undo()) : null,
          icon: const Icon(Icons.undo),
        ),
        IconButton(
          tooltip: '${_s.redo} (Ctrl+Y)',
          iconSize: 18,
          onPressed: _session.canRedo ? () => setState(() => _session.redo()) : null,
          icon: const Icon(Icons.redo),
        ),
      ],
      trailing: [
        for (final p in _session.peers.take(5))
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 2),
            child: Tooltip(message: p.name, child: CircleAvatar(radius: 12, child: Text(p.name.isEmpty ? '?' : p.name.characters.first.toUpperCase(), style: const TextStyle(fontSize: 11)))),
          ),
        const SizedBox(width: 8),
      ],
      initialTab: 0,
      tabs: [
        RibbonTab(_s.home, [
          RibbonGroup(_s.clipboard, [
            RibbonButton(icon: const Icon(Icons.content_paste), label: _s.paste, large: true, shortcut: 'Ctrl+V', onPressed: editable && _clipboard.isNotEmpty ? _paste : null),
            RibbonButton(icon: const Icon(Icons.content_cut), label: _s.cut, shortcut: 'Ctrl+X', onPressed: shapes ? () => _copy(cut: true) : null),
            RibbonButton(icon: const Icon(Icons.content_copy), label: _s.copy, shortcut: 'Ctrl+C', onPressed: shapes ? _copy : null),
          ]),
          RibbonGroup(_s.slides, [
            RibbonMenu<Node>(
              icon: const Icon(Icons.add_box_outlined),
              label: _s.newSlide,
              large: true,
              enabled: editable && layouts.isNotEmpty,
              items: [for (final l in layouts) PopupMenuItem(value: l, child: Text('${l.attributes['name'] ?? l.id}'))],
              onSelected: _newSlide,
            ),
            RibbonButton(icon: const Icon(Icons.copy_all_outlined), label: _s.duplicateSlide, onPressed: editable ? _duplicateSlide : null),
            RibbonButton(icon: const Icon(Icons.delete_outline), label: _s.deleteSlide, onPressed: editable ? _deleteSlide : null),
          ]),
          RibbonGroup(_s.font, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonDropdown(label: _s.font, value: run['font'] ?? '+mn-lt', values: fonts, display: fontName, onChanged: hasText ? (f) => _formatRuns({'font': f}) : null),
              const SizedBox(width: 4),
              RibbonDropdown(
                label: _s.fontSize,
                width: 56,
                value: _size.toString().replaceAll(RegExp(r'\.0$'), ''),
                values: [for (final s in _sizes) s.toString().replaceAll(RegExp(r'\.0$'), '')],
                onChanged: hasText ? (v) => _setSize(double.parse(v)) : null,
              ),
              RibbonButton(icon: const Icon(Icons.text_increase), label: _s.growFont, shortcut: 'Ctrl+>', onPressed: hasText ? () => _growFont(true) : null),
              RibbonButton(icon: const Icon(Icons.text_decrease), label: _s.shrinkFont, shortcut: 'Ctrl+<', onPressed: hasText ? () => _growFont(false) : null),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.format_bold), label: _s.bold, shortcut: 'Ctrl+G', selected: run['b'] == '1', onPressed: hasText ? () => _toggle('b', '1', '0') : null),
              RibbonButton(icon: const Icon(Icons.format_italic), label: _s.italic, shortcut: 'Ctrl+I', selected: run['i'] == '1', onPressed: hasText ? () => _toggle('i', '1', '0') : null),
              RibbonButton(icon: const Icon(Icons.format_underline), label: _s.underline, shortcut: 'Ctrl+U', selected: (run['u'] ?? 'none') != 'none', onPressed: hasText ? () => _toggle('u', 'sng', 'none') : null),
              RibbonButton(icon: const Icon(Icons.format_strikethrough), label: _s.strikethrough, selected: (run['strike'] ?? 'noStrike') != 'noStrike', onPressed: hasText ? () => _toggle('strike', 'sngStrike', 'noStrike') : null),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.format_color_text),
                label: _s.fontColor,
                enabled: hasText,
                items: [palette((c) => _formatRuns({'fill': c == null ? '' : jsonEncode({'solid': c})}), none: _s.automatic)],
                onSelected: (_) {},
              ),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.border_color),
                label: _s.highlight,
                enabled: hasText,
                items: [palette((c) => _formatRuns({'hl': c == null ? '' : jsonEncode(c)}), none: _s.noFill)],
                onSelected: (_) {},
              ),
            ]),
          ]),
          RibbonGroup(_s.paragraph, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(
                icon: const Icon(Icons.format_list_bulleted),
                label: _s.bullets,
                selected: (para['bu'] ?? '').startsWith('char:'),
                onPressed: hasText ? () => _formatParagraphs({'bu': (para['bu'] ?? '').startsWith('char:') ? 'none' : 'char:•'}) : null,
              ),
              RibbonButton(
                icon: const Icon(Icons.format_list_numbered),
                label: _s.numbering,
                selected: (para['bu'] ?? '').startsWith('auto:'),
                onPressed: hasText ? () => _formatParagraphs({'bu': (para['bu'] ?? '').startsWith('auto:') ? 'none' : 'auto:arabicPeriod'}) : null,
              ),
              RibbonButton(
                icon: const Icon(Icons.format_indent_decrease),
                label: _s.decreaseLevel,
                onPressed: hasText ? () => _level(-1) : null,
              ),
              RibbonButton(
                icon: const Icon(Icons.format_indent_increase),
                label: _s.increaseLevel,
                onPressed: hasText ? () => _level(1) : null,
              ),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              for (final (algn, icon, label, key) in [
                ('l', Icons.format_align_left, _s.alignLeft, 'Ctrl+L'),
                ('ctr', Icons.format_align_center, _s.center, 'Ctrl+E'),
                ('r', Icons.format_align_right, _s.alignRight, 'Ctrl+R'),
                ('just', Icons.format_align_justify, _s.justify, 'Ctrl+J'),
              ])
                RibbonButton(icon: Icon(icon), label: label, shortcut: key, selected: (para['algn'] ?? 'l') == algn, onPressed: hasText ? () => _formatParagraphs({'algn': algn}) : null),
              direction(),
            ]),
          ]),
          RibbonGroup(_s.drawing, [
            _shapesMenu(editable, large: true),
            RibbonMenu<Map<String, Object?>?>(
              icon: const Icon(Icons.format_color_fill),
              label: _s.shapeFill,
              enabled: shapes,
              items: [palette((c) => _fillShapes('fill', c), none: _s.noFill)],
              onSelected: (_) {},
            ),
            RibbonMenu<Map<String, Object?>?>(
              icon: const Icon(Icons.border_style),
              label: _s.shapeOutline,
              enabled: shapes,
              items: [palette((c) => _fillShapes('line', c), none: _s.noOutline)],
              onSelected: (_) {},
            ),
          ]),
          RibbonGroup(_s.arrange, [
            RibbonButton(icon: const Icon(Icons.flip_to_front), label: _s.bringToFront, onPressed: shapes ? () => _arrange(true, all: true) : null),
            RibbonButton(icon: const Icon(Icons.flip_to_back), label: _s.sendToBack, onPressed: shapes ? () => _arrange(false, all: true) : null),
            RibbonButton(icon: const Icon(Icons.arrow_upward), label: _s.bringForward, onPressed: shapes ? () => _arrange(true) : null),
            RibbonButton(icon: const Icon(Icons.arrow_downward), label: _s.sendBackward, onPressed: shapes ? () => _arrange(false) : null),
          ]),
        ]),
        RibbonTab(_s.insert, [
          RibbonGroup(_s.slides, [
            RibbonMenu<Node>(
              icon: const Icon(Icons.add_box_outlined),
              label: _s.newSlide,
              large: true,
              enabled: editable && layouts.isNotEmpty,
              items: [for (final l in layouts) PopupMenuItem(value: l, child: Text('${l.attributes['name'] ?? l.id}'))],
              onSelected: _newSlide,
            ),
          ]),
          RibbonGroup(_s.tables, [
            RibbonMenu<(int, int)>(
              icon: const Icon(Icons.table_chart_outlined),
              label: _s.table,
              large: true,
              enabled: editable,
              items: [
                PopupMenuItem(
                  enabled: false,
                  child: Builder(builder: (context) => TableSizeGrid(
                    strings: _s,
                    onSelected: (cols, rows) {
                      Navigator.of(context).pop();
                      _insertTable(rows, cols);
                    },
                  )),
                ),
              ],
              onSelected: (_) {},
            ),
          ]),
          RibbonGroup(_s.drawing, [
            _shapesMenu(editable, large: true),
            RibbonButton(icon: const Icon(Icons.text_fields), label: _s.textBox, large: true, onPressed: editable ? _insertTextBox : null),
          ]),
        ]),
        if (table != null)
          RibbonTab(_s.tableDesign, [
            RibbonGroup(_s.tableStyleOptions, [
              Row(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
                Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
                  option(_s.headerRow, 'firstRow'),
                  option(_s.totalRow, 'lastRow'),
                  option(_s.bandedRows, 'bandRow'),
                ]),
                Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
                  option(_s.firstColumn, 'firstCol'),
                  option(_s.lastColumn, 'lastCol'),
                  option(_s.bandedColumns, 'bandCol'),
                ]),
              ]),
            ]),
            RibbonGroup(_s.tableStyles, [
              RibbonMenu<String>(
                icon: const Icon(Icons.table_view),
                label: _s.styles,
                large: true,
                enabled: editable,
                constraints: const BoxConstraints(maxWidth: TableStyleGallery.width + 32),
                items: [
                  PopupMenuItem(
                    enabled: false,
                    child: Builder(builder: (context) => TableStyleGallery(
                      deck: deck,
                      table: table.$1,
                      strings: _s,
                      onSelected: (id) {
                        Navigator.of(context).pop();
                        _tableEdit((e, _) => e.restyle(id));
                      },
                    )),
                  ),
                ],
                onSelected: (_) {},
              ),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.format_color_fill),
                label: _s.shading,
                large: true,
                enabled: editable,
                items: [palette((c) => _fillShapes('fill', c), none: _s.noFill)],
                onSelected: (_) {},
              ),
              RibbonMenu<String>(
                icon: const Icon(Icons.border_all),
                label: _s.borders,
                large: true,
                enabled: editable,
                items: [
                  for (final which in ['bottom', 'top', 'left', 'right', 'none', 'all', 'outside', 'inside', 'insideH', 'insideV', 'tl2br', 'tr2bl'])
                    PopupMenuItem(value: which, child: Text(_s.border(which))),
                ],
                onSelected: (which) {
                  final cells = _tableCells;
                  _tableEdit((e, _) => e.borders(cells, which, _pen));
                },
              ),
            ]),
            RibbonGroup(_s.drawBorders, [
              RibbonMenu<int>(
                icon: const Icon(Icons.line_weight),
                label: _s.penWeight,
                enabled: editable,
                items: [
                  for (final w in _penWeights)
                    CheckedPopupMenuItem(value: w, checked: _pen['w'] == w, child: Text('${_points(w)} pt')),
                ],
                onSelected: (w) => setState(() => _pen = {..._pen, 'w': w}),
              ),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.border_color),
                label: _s.penColor,
                enabled: editable,
                items: [palette((c) => setState(() => _pen = {..._pen, 'fill': {'solid': c ?? {'rgb': '000000'}}}))],
                onSelected: (_) {},
              ),
            ]),
          ]),
        if (table != null)
          RibbonTab(_s.layout, [
            RibbonGroup(_s.rowsAndColumns, [
              RibbonMenu<String>(
                icon: const Icon(Icons.delete_outline),
                label: _s.delete,
                large: true,
                enabled: editable,
                items: [
                  if (cell != null) ...[
                    PopupMenuItem(value: 'row', child: Text(_s.deleteRow)),
                    PopupMenuItem(value: 'column', child: Text(_s.deleteColumn)),
                  ],
                  PopupMenuItem(value: 'table', child: Text(_s.deleteTable)),
                ],
                onSelected: (what) => _tableEdit((e, c) => switch (what) {
                  'row' => e.deleteRow(c!.row),
                  'column' => e.deleteColumn(c!.col),
                  _ => e.deleteTable(),
                }),
              ),
              RibbonButton(icon: const Icon(Icons.border_top), label: _s.insertAbove, large: true, onPressed: cell == null ? null : () => _tableEdit((e, c) => e.insertRow(c!.row))),
              RibbonButton(icon: const Icon(Icons.border_bottom), label: _s.insertBelow, large: true, onPressed: cell == null ? null : () => _tableEdit((e, c) => e.insertRow(c!.row + c.rowSpan))),
              RibbonButton(icon: const Icon(Icons.border_left), label: _s.insertLeft, large: true, onPressed: cell == null ? null : () => _tableEdit((e, c) => e.insertColumn(c!.col))),
              RibbonButton(icon: const Icon(Icons.border_right), label: _s.insertRight, large: true, onPressed: cell == null ? null : () => _tableEdit((e, c) => e.insertColumn(c!.col + c.colSpan))),
            ]),
            RibbonGroup(_s.alignment, [
              Row(mainAxisSize: MainAxisSize.min, children: [
                for (final (algn, icon, label) in [
                  ('l', Icons.format_align_left, _s.alignLeft),
                  ('ctr', Icons.format_align_center, _s.center),
                  ('r', Icons.format_align_right, _s.alignRight),
                ])
                  RibbonButton(icon: Icon(icon), label: label, selected: (para['algn'] ?? 'l') == algn, onPressed: hasText ? () => _formatParagraphs({'algn': algn}) : null),
              ]),
              Row(mainAxisSize: MainAxisSize.min, children: [
                for (final (anchor, icon, label) in [
                  ('t', Icons.vertical_align_top, _s.alignTop),
                  ('ctr', Icons.vertical_align_center, _s.centerVertically),
                  ('b', Icons.vertical_align_bottom, _s.alignBottom),
                ])
                  RibbonButton(
                    icon: Icon(icon),
                    label: label,
                    selected: (table.$2?.node.attributes['anchor'] ?? 't') == anchor,
                    onPressed: editable ? () => _anchorCells(anchor) : null,
                  ),
              ]),
              direction(large: true),
            ]),
            RibbonGroup(_s.cellSize, [
              RibbonMeasure(
                label: _s.height,
                icon: Icons.height,
                value: table.$2?.rect.height,
                onChanged: cell == null ? null : (h) => _tableEdit((e, c) => e.resizeRow(c!.row + c.rowSpan, h - c.rect.height)),
              ),
              RibbonMeasure(
                label: _s.width,
                icon: Icons.width_normal_outlined,
                value: table.$2?.rect.width,
                onChanged: cell == null ? null : (w) => _tableEdit((e, c) => e.resizeColumn(c!.col + c.colSpan, w - c.rect.width, widen: true)),
              ),
            ]),
            RibbonGroup(_s.merge, [
              RibbonButton(icon: const Icon(Icons.call_merge), label: _s.mergeCells, large: true, onPressed: cells.length > 1 ? _merge : null),
              RibbonButton(
                icon: const Icon(Icons.call_split),
                label: _s.splitCells,
                large: true,
                onPressed: cells.length == 1 ? _splitCells : null,
              ),
            ]),
          ]),
        RibbonTab(_s.transitions, [
          RibbonGroup(_s.preview, [
            RibbonButton(icon: const Icon(Icons.play_circle_outline), label: _s.preview, large: true, onPressed: _transition['effect'] != null ? _preview : null),
          ]),
          RibbonGroup(_s.transitionToThisSlide, [
            for (final (effect, icon) in [
              ('', Icons.block),
              ('cut', Icons.flash_on_outlined),
              ('fade', Icons.gradient),
              ('push', Icons.keyboard_double_arrow_up),
              ('wipe', Icons.swipe_left_outlined),
              ('split', Icons.vertical_split_outlined),
              ('pull', Icons.open_in_new),
              ('cover', Icons.layers_outlined),
              ('zoom', Icons.zoom_in),
            ])
              RibbonButton(
                icon: Icon(icon),
                label: _s.transition(effect),
                large: true,
                selected: (_transition['effect'] ?? '') == effect,
                onPressed: editable && _slide != null ? () => _pickTransition(effect) : null,
              ),
            RibbonMenu<Map<String, Object?>>(
              icon: const Icon(Icons.tune),
              label: _s.effectOptions,
              large: true,
              enabled: editable && _effectOptions(_transition['effect']).isNotEmpty,
              items: [
                for (final (name, keys) in _effectOptions(_transition['effect']))
                  CheckedPopupMenuItem(
                    value: keys,
                    checked: keys.entries.every((e) => _transition[e.key] == e.value),
                    child: Text(_s.effectOption(name)),
                  ),
              ],
              onSelected: (keys) {
                _retransition(keys);
                _preview();
              },
            ),
          ]),
          RibbonGroup(_s.timing, [
            RibbonButton(icon: const Icon(Icons.done_all), label: _s.applyToAll, large: true, onPressed: editable && _slide != null ? () => _retransition({}, all: true) : null),
            RibbonMeasure(
              label: _s.duration,
              icon: Icons.timer_outlined,
              unit: 's',
              scale: 1000,
              value: (_transition['dur'] as num?)?.toDouble(),
              onChanged: editable && _transition['effect'] != null ? (ms) => _retransition({'dur': ms.round()}) : null,
            ),
            RibbonCheck(
              label: _s.onMouseClick,
              value: _transition['noClick'] != true,
              onChanged: editable && _slide != null ? (click) => _retransition({'noClick': click ? null : true}) : null,
            ),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonCheck(
                label: _s.advanceAfter,
                value: _transition['after'] != null,
                onChanged: editable && _slide != null ? (after) => _retransition({'after': after ? 0 : null}) : null,
              ),
              RibbonMeasure(
                label: _s.advanceAfter,
                icon: Icons.schedule,
                unit: 's',
                scale: 1000,
                value: (_transition['after'] as num?)?.toDouble() ?? 0,
                onChanged: editable && _slide != null ? (ms) => _retransition({'after': ms.round()}) : null,
              ),
            ]),
          ]),
        ]),
        RibbonTab(_s.slideShow, [
          RibbonGroup(_s.startSlideShow, [
            RibbonButton(icon: const Icon(Icons.slideshow), label: _s.fromBeginning, large: true, shortcut: 'F5', onPressed: _slideshow),
            RibbonButton(icon: const Icon(Icons.play_arrow), label: _s.fromCurrentSlide, large: true, shortcut: 'Maj+F5', onPressed: () => _slideshow(fromCurrent: true)),
            RibbonButton(
              icon: const Icon(Icons.co_present_outlined),
              label: _s.presenterView,
              large: true,
              shortcut: 'Alt+F5',
              onPressed: () => _slideshow(fromCurrent: true, presenter: true),
            ),
            RibbonButton(
              icon: const Icon(Icons.visibility_off_outlined),
              label: _s.hideSlide,
              large: true,
              selected: _slide?.attributes['hidden'] == true,
              onPressed: editable ? _hideSlide : null,
            ),
          ]),
        ]),
        RibbonTab(_s.view, [
          RibbonGroup(_s.presentationViews, [
            RibbonButton(icon: const Icon(Icons.view_sidebar_outlined), label: _s.normal, large: true, selected: !_sorter, onPressed: () => setState(() => _sorter = false)),
            RibbonButton(icon: const Icon(Icons.grid_view), label: _s.slideSorter, large: true, selected: _sorter, onPressed: () => setState(() => _sorter = true)),
          ]),
          RibbonGroup(_s.show, [
            RibbonButton(icon: const Icon(Icons.notes), label: _s.notes, large: true, selected: _notes, onPressed: () => setState(() => _notes = !_notes)),
          ]),
        ]),
      ],
    );
  }

  void _level(int by) {
    final lvl = int.tryParse(_paraProps['lvl'] ?? '') ?? 0;
    final next = (lvl + by).clamp(0, 8);
    _formatParagraphs({'lvl': next == 0 ? '' : '$next'});
  }

  static const _presets = [
    'rect', 'roundRect', 'ellipse', 'triangle', 'rtTriangle', 'parallelogram', 'trapezoid', 'diamond', //
    'pentagon', 'hexagon', 'octagon', 'star5', 'star6', 'rightArrow', 'leftArrow', 'upArrow', 'downArrow',
    'leftRightArrow', 'chevron', 'homePlate', 'heart', 'lightningBolt', 'cloud', 'smileyFace', 'donut', 'plus',
    'flowChartProcess', 'flowChartDecision', 'flowChartTerminator', 'wedgeRectCallout', 'wedgeEllipseCallout', 'blockArc',
  ];

  Widget _shapesMenu(bool enabled, {bool large = false}) => RibbonMenu<String>(
    icon: const Icon(Icons.category_outlined),
    label: _s.shapes,
    large: large,
    enabled: enabled,
    items: [
      PopupMenuItem(
        enabled: false,
        padding: const EdgeInsets.all(4),
        child: Builder(
          builder: (context) => SizedBox(
            width: 8 * 34,
            child: Wrap(children: [
              for (final p in _presets)
                InkWell(
                  onTap: () {
                    Navigator.of(context).pop();
                    _insertShape(p);
                  },
                  child: Padding(
                    padding: const EdgeInsets.all(5),
                    child: CustomPaint(size: const Size(24, 24), painter: _PresetIcon(p, Theme.of(context).colorScheme.onSurface)),
                  ),
                ),
            ]),
          ),
        ),
      ),
    ],
    onSelected: (_) {},
  );

  Widget _notesPane(BuildContext context, Node slide) {
    final notes = _session.document['${slide.id}-notes'];
    return Container(
      height: 110,
      decoration: BoxDecoration(border: Border(top: BorderSide(color: Theme.of(context).dividerColor))),
      child: notes == null
          ? Padding(
              padding: const EdgeInsets.all(16),
              child: Text(_s.notesPrompt, style: TextStyle(color: Theme.of(context).hintColor)),
            )
          : PlainTextEditor(key: ValueKey(notes.id), session: _session, node: notes.id, style: const TextStyle(fontSize: 13)),
    );
  }

  Widget _statusBar(BuildContext context) {
    final slides = _current.slides;
    final i = slides.indexWhere((s) => s.id == _slide?.id);
    final state = _session.saveError != null
        ? _s.saveFailed(_session.saveError!)
        : _session.status == DocStatus.connecting
        ? _s.connecting
        : _session.saved
        ? _s.saved
        : _s.saving;
    final style = Theme.of(context).textTheme.labelSmall;
    final phone = MediaQuery.sizeOf(context).width < phoneWidth;
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainer,
      child: SizedBox(
        height: 26,
        child: Row(children: [
          const SizedBox(width: 12),
          Text(slides.isEmpty ? '' : _s.slideOf(i + 1, slides.length), style: style),
          const SizedBox(width: 16),
          if (!phone) ...[Text(_s.language, style: style), const SizedBox(width: 16)],
          Icon(
            _session.saveError != null ? Icons.error_outline : (_session.saved ? Icons.cloud_done_outlined : Icons.cloud_upload_outlined),
            size: 14,
          ),
          const SizedBox(width: 4),
          Expanded(child: Text(state, style: style, overflow: TextOverflow.ellipsis)),
          if (!phone) ...[
            IconButton(iconSize: 16, tooltip: _s.notes, onPressed: () => setState(() => _notes = !_notes), icon: const Icon(Icons.notes)),
            IconButton(iconSize: 16, tooltip: _s.normal, onPressed: () => setState(() => _sorter = false), icon: const Icon(Icons.view_sidebar_outlined)),
            IconButton(iconSize: 16, tooltip: _s.slideSorter, onPressed: () => setState(() => _sorter = true), icon: const Icon(Icons.grid_view)),
          ],
          IconButton(iconSize: 16, tooltip: _s.startSlideShow, onPressed: () => _slideshow(fromCurrent: true), icon: const Icon(Icons.slideshow)),
          const SizedBox(width: 8),
        ]),
      ),
    );
  }
}

/// A preset shape drawn as its own icon.
class _PresetIcon extends CustomPainter {
  _PresetIcon(this.preset, this.color);

  final String preset;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final g = Geometry.preset(preset);
    if (g == null) return;
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.2
      ..color = color;
    for (final p in g.paths(size)) {
      canvas.drawPath(p.path, paint);
    }
  }

  @override
  bool shouldRepaint(_PresetIcon old) => old.preset != preset || old.color != color;
}

/// A slide drawn small.
class _Thumbnail extends StatelessWidget {
  const _Thumbnail({required this.editor, required this.slide, required this.number, required this.selected});

  final _PresentationEditorState editor;
  final Node slide;
  final int number;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final size = editor._current.size;
    final hidden = slide.attributes['hidden'] == true;
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 6, 8, 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 22,
            child: Text('$number', textAlign: TextAlign.right, style: TextStyle(fontSize: 11, decoration: hidden ? TextDecoration.lineThrough : null)),
          ),
          const SizedBox(width: 4),
          Expanded(
            child: AspectRatio(
              aspectRatio: size.width / size.height,
              child: Container(
                decoration: BoxDecoration(
                  border: Border.all(color: selected ? scheme.primary : Theme.of(context).dividerColor, width: selected ? 2.5 : 1),
                ),
                child: Opacity(
                  opacity: hidden ? 0.5 : 1,
                  child: CustomPaint(painter: _SlideThumbnailPainter(editor._paint, editor._current, slide)),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SlideThumbnailPainter extends CustomPainter {
  _SlideThumbnailPainter(this.painter, this.deck, this.slide);

  final SlidePainter painter;
  final Deck deck;
  final Node slide;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.scale(size.width / deck.size.width);
    painter.paint(canvas, slide);
  }

  @override
  bool shouldRepaint(_SlideThumbnailPainter old) => true;
}

/// The slides at the left, in order: click to edit one, drag to move it.
class _Thumbnails extends StatelessWidget {
  const _Thumbnails({required this.editor, this.horizontal = false});

  final _PresentationEditorState editor;

  /// Whether the thumbnails go in a row, as on a phone.
  final bool horizontal;

  @override
  Widget build(BuildContext context) {
    final slides = editor._current.slides;
    final editable = !editor._session.readOnly;
    return ReorderableListView.builder(
      scrollDirection: horizontal ? Axis.horizontal : Axis.vertical,
      buildDefaultDragHandles: editable,
      itemCount: slides.length,
      onReorderItem: (from, to) => editor._edit(DeckEdits(editor._current).reorder(slides[from], to)),
      itemBuilder: (context, i) {
        final slide = slides[i];
        return GestureDetector(
          key: ValueKey(slide.id),
          onTap: () => editor._goTo(slide),
          onSecondaryTapDown: editable ? (d) => _menu(context, d.globalPosition, slide) : null,
          onLongPressStart: editable ? (d) => _menu(context, d.globalPosition, slide) : null,
          child: SizedBox(
            width: horizontal ? 132 : null,
            child: _Thumbnail(editor: editor, slide: slide, number: i + 1, selected: slide.id == editor._slide?.id),
          ),
        );
      },
    );
  }

  Future<void> _menu(BuildContext context, Offset at, Node slide) async {
    editor._goTo(slide);
    final s = editor._s;
    final choice = await showMenu<String>(
      context: context,
      position: RelativeRect.fromLTRB(at.dx, at.dy, at.dx, at.dy),
      items: [
        PopupMenuItem(value: 'new', child: Text(s.newSlide)),
        PopupMenuItem(value: 'duplicate', child: Text(s.duplicateSlide)),
        PopupMenuItem(value: 'delete', child: Text(s.deleteSlide)),
        PopupMenuItem(value: 'hide', child: Text(s.hideSlide)),
      ],
    );
    switch (choice) {
      case 'new':
        editor._newSlide();
      case 'duplicate':
        editor._duplicateSlide();
      case 'delete':
        editor._deleteSlide();
      case 'hide':
        editor._hideSlide();
    }
  }
}

/// The slide sorter: every slide in a grid.
class _Sorter extends StatelessWidget {
  const _Sorter({required this.editor});

  final _PresentationEditorState editor;

  @override
  Widget build(BuildContext context) {
    final slides = editor._current.slides;
    return GridView.builder(
      padding: const EdgeInsets.all(16),
      gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(maxCrossAxisExtent: 260, childAspectRatio: 1.4),
      itemCount: slides.length,
      itemBuilder: (context, i) => GestureDetector(
        onTap: () => editor._goTo(slides[i]),
        onDoubleTap: () {
          editor._goTo(slides[i]);
          editor._view(sorter: false);
        },
        child: _Thumbnail(editor: editor, slide: slides[i], number: i + 1, selected: slides[i].id == editor._slide?.id),
      ),
    );
  }
}

/// The backstage of the File tab: what the document is, and closing it.
class _Backstage extends StatelessWidget {
  const _Backstage({required this.editor});

  final _PresentationEditorState editor;

  @override
  Widget build(BuildContext context) {
    final s = editor._s;
    final session = editor._session;
    final scheme = Theme.of(context).colorScheme;
    return Material(
      color: scheme.surface,
      child: Row(
        children: [
          Container(
            width: 220,
            color: scheme.primary,
            child: ListTileTheme(
              textColor: scheme.onPrimary,
              iconColor: scheme.onPrimary,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  ListTile(leading: const Icon(Icons.arrow_back), title: Text(s.back), onTap: editor._closeBackstage),
                  ListTile(leading: const Icon(Icons.info_outline), title: Text(s.info), selected: true, selectedColor: scheme.onPrimary),
                  if (editor.widget.onClose != null)
                    ListTile(leading: const Icon(Icons.close), title: Text(s.close), onTap: editor.widget.onClose),
                ],
              ),
            ),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(32),
              children: [
                Text(s.info, style: Theme.of(context).textTheme.headlineMedium),
                const SizedBox(height: 16),
                Text(editor.widget.title, style: Theme.of(context).textTheme.titleLarge),
                const SizedBox(height: 16),
                Text(session.saveError != null ? s.saveFailed(session.saveError!) : (session.saved ? s.saved : s.saving)),
                if (session.readOnly) Text(s.readOnly),
                const SizedBox(height: 16),
                for (final p in session.peers) ListTile(leading: const Icon(Icons.person_outline), title: Text(p.name)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// The columns and rows PowerPoint asks a cell to be split into.
class _SplitDialog extends StatefulWidget {
  const _SplitDialog({required this.strings});

  final LofficeStrings strings;

  @override
  State<_SplitDialog> createState() => _SplitDialogState();
}

class _SplitDialogState extends State<_SplitDialog> {
  final _columns = TextEditingController(text: '2');
  final _rows = TextEditingController(text: '1');

  @override
  void dispose() {
    _columns.dispose();
    _rows.dispose();
    super.dispose();
  }

  void _done() {
    final columns = int.tryParse(_columns.text) ?? 0, rows = int.tryParse(_rows.text) ?? 0;
    if (columns >= 1 && rows >= 1 && columns <= 75 && rows <= 75) Navigator.of(context).pop((columns, rows));
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    Widget field(TextEditingController controller, String label) => TextField(
      controller: controller,
      keyboardType: TextInputType.number,
      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
      decoration: InputDecoration(labelText: label),
      onSubmitted: (_) => _done(),
    );
    return AlertDialog(
      title: Text(s.splitCells),
      content: Column(mainAxisSize: MainAxisSize.min, children: [
        field(_columns, s.numberOfColumns),
        field(_rows, s.numberOfRows),
      ]),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: _done, child: Text(s.ok)),
      ],
    );
  }
}

class _Failure extends StatelessWidget {
  const _Failure({required this.message, required this.retry, required this.onRetry});

  final String message;
  final String retry;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      Icon(Icons.error_outline, size: 40, color: Theme.of(context).colorScheme.error),
      const SizedBox(height: 12),
      Text(message, textAlign: TextAlign.center),
      const SizedBox(height: 12),
      FilledButton(onPressed: onRetry, child: Text(retry)),
    ],
  );
}
