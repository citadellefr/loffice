import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/commands.dart';
import '../chrome/comments.dart';
import '../chrome/comments_pane.dart' show authorColor;
import '../chrome/link_card.dart';
import '../chrome/strings.dart';
import '../clipboard.dart';
import '../text/editing.dart';
import '../text/text_frame.dart' show linkOf, linkProps;
import 'comments.dart';
import 'deck.dart';
import 'edits.dart';
import 'slide_painter.dart';
import 'table.dart';
import 'table_edits.dart';

/// What is selected on a slide: shapes, or a range of the text of one.
class SlideSelection extends ChangeNotifier {
  Set<String> _shapes = const {};
  String? _editing;
  int _base = 0;
  int _extent = 0;

  /// The formatting a caret was given, for what is typed next.
  Attributes? typing;

  Set<String> get shapes => _shapes;

  /// The shape whose text is being edited.
  String? get editing => _editing;

  int get base => _base;
  int get extent => _extent;
  int get start => math.min(_base, _extent);
  int get end => math.max(_base, _extent);
  bool get collapsed => _base == _extent;

  void selectShapes(Iterable<String> ids) {
    _shapes = Set.unmodifiable(ids);
    _editing = null;
    typing = null;
    notifyListeners();
  }

  /// Edits the text of a shape, selecting [base] to [extent].
  void edit(String id, int base, [int? extent]) {
    if (_editing != id || _base != base || _extent != (extent ?? base)) typing = null;
    _shapes = {id};
    _editing = id;
    _base = base;
    _extent = extent ?? base;
    notifyListeners();
  }

  void clear() => selectShapes(const []);
}

/// A slide being edited: its shapes selected, moved, resized and rotated
/// with the mouse or a finger, their text typed in.
class SlideCanvas extends StatefulWidget {
  const SlideCanvas({
    super.key,
    required this.session,
    required this.deck,
    required this.slide,
    required this.painter,
    required this.selection,
    this.focusNode,
    this.strings = const LofficeStrings(),
    this.onShortcut,
    this.comments = const [],
    this.activeComment,
    this.onComment,
    this.commands = const [],
    this.onOpenLink,
    this.linkCard,
  });

  /// The keywords an `@` typed in a text starts.
  final List<Command> commands;

  /// Opens a link of a text: clicked with Ctrl, or alone in a document
  /// only read.
  final void Function(Uri uri)? onOpenLink;

  /// The card of the host over a link pointed at, or tapped on a touch
  /// screen in a document only read: what it leads to. Null shows nothing.
  final LinkCard? linkCard;

  final DocSession session;
  final Deck deck;
  final Node slide;
  final SlidePainter painter;
  final SlideSelection selection;
  final FocusNode? focusNode;
  final LofficeStrings strings;

  /// Keys the canvas leaves to the editor around it; true when handled.
  final bool Function(KeyEvent event)? onShortcut;

  /// The comment threads of the slide, drawn as balloons where they sit,
  /// the active one raised; [onComment] hears the one that is clicked.
  final List<CommentThread> comments;
  final String? activeComment;
  final ValueChanged<String>? onComment;

  @override
  State<SlideCanvas> createState() => SlideCanvasState();
}

enum _Drag { none, move, resize, rotate, text, cells, column, row }

/// Handles of a selected shape: the corners, the middles of the sides and
/// the rotation handle above.
enum _Handle { topLeft, top, topRight, right, bottomRight, bottom, bottomLeft, left, rotate }

class SlideCanvasState extends State<SlideCanvas> implements DeltaTextInputClient, CommandField {
  late final _commands = Commands(this, commands: () => widget.commands, strings: () => widget.strings);
  late final _cards = LinkCards(() => context, () => widget.linkCard);
  late FocusNode _focus = (widget.focusNode ?? FocusNode())..addListener(_focusChanged);
  TextInputConnection? _input;
  StreamSubscription<Edit>? _changes;
  void Function()? _unwatchPaste;
  var _local = false;

  var _scale = 1.0;
  var _origin = Offset.zero;

  var _drag = _Drag.none;
  _Handle? _handle;
  Offset _dragFrom = Offset.zero;
  Offset _dragTo = Offset.zero;
  Node? _pressed;
  var _moved = false;
  DateTime _lastDown = DateTime(0);
  Offset _lastDownAt = Offset.zero;
  var _clicks = 0;

  /// The cell a range of cells goes from, and where its text was being
  /// selected from before the range took over.
  String? _anchor;
  var _anchorOffset = 0;

  /// The table whose edge is under the mouse or dragged, and that edge.
  (Node, _Drag, int)? _edge;
  MouseCursor _cursor = MouseCursor.defer;

  Timer? _blink;
  var _caretOn = true;
  double? _goalX;

  SlideSelection get _selection => widget.selection;
  DocSession get _session => widget.session;
  Deck get _deck => widget.deck;

  @override
  void initState() {
    super.initState();
    _selection.addListener(_selectionChanged);
    _changes = _session.changes.listen(_documentChanged);
    _unwatchPaste = watchPaste(_pasted);
  }

  @override
  void didUpdateWidget(SlideCanvas oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selection != widget.selection) {
      oldWidget.selection.removeListener(_selectionChanged);
      widget.selection.addListener(_selectionChanged);
    }
    if (oldWidget.focusNode != widget.focusNode) {
      _focus.removeListener(_focusChanged);
      if (oldWidget.focusNode == null) _focus.dispose();
      _focus = (widget.focusNode ?? FocusNode())..addListener(_focusChanged);
    }
    if (oldWidget.slide.id != widget.slide.id && _selection.shapes.isNotEmpty) _selection.clear();
  }

  @override
  void dispose() {
    _selection.removeListener(_selectionChanged);
    _unwatchPaste?.call();
    unawaited(_changes?.cancel());
    _commands.dispose();
    _cards.hide();
    _blink?.cancel();
    _input?.close();
    _focus.removeListener(_focusChanged);
    if (widget.focusNode == null) _focus.dispose();
    super.dispose();
  }

  void _focusChanged() {
    if (!_focus.hasFocus) _commands.close();
  }

  void _selectionChanged() {
    final editing = _selection.editing;
    if (editing == null) {
      _input?.close();
      _input = null;
      _blink?.cancel();
      _session.select(null);
    } else {
      _openInput();
      _session.select(DocSelection(editing, _selection.base, _selection.extent));
      _restartBlink();
    }
    _commands.follow();
    if (mounted) setState(() {});
  }

  void _restartBlink() {
    _caretOn = true;
    _blink?.cancel();
    _blink = Timer.periodic(const Duration(milliseconds: 530), (_) {
      if (mounted) setState(() => _caretOn = !_caretOn);
    });
  }

  /// Follows the edits of others in the text being edited.
  void _documentChanged(Edit edit) {
    final editing = _selection.editing;
    if (editing == null || _local) {
      if (mounted) setState(() {});
      return;
    }
    if (_session.document[editing] == null) {
      _selection.clear();
      return;
    }
    var base = _selection.base, extent = _selection.extent;
    for (final c in edit.changes) {
      if (c.kind == ChangeKind.text && c.id == editing) {
        base = c.text!.transformPosition(base, thisFirst: false);
        extent = c.text!.transformPosition(extent, thisFirst: false);
      }
    }
    _selection.edit(editing, base, extent);
    _input?.setEditingState(currentTextEditingValue);
  }

  // coordinates

  Offset _toSlide(Offset view) => (view - _origin) / _scale;

  Node? get _editingNode {
    final id = _selection.editing;
    return id == null ? null : _session.document[id];
  }

  // keywords

  @override
  ({String text, int caret})? get typing {
    final node = _editingNode;
    final s = _selection;
    if (node == null || !s.collapsed || _session.readOnly || !_focus.hasFocus) return null;
    final editing = FlowEditing(node.text!);
    final (start, end) = editing.paragraphAt(s.extent);
    return (text: editing.text.substring(start, end), caret: s.extent - start);
  }

  /// The texts of the slide, the one being edited around its paragraph.
  @override
  ({String before, String after}) get around {
    final node = _editingNode;
    if (node == null) return (before: '', after: '');
    final (start, end) = FlowEditing(node.text!).paragraphAt(_selection.extent);
    final before = StringBuffer(), after = StringBuffer();
    var passed = false;
    void walk(String parent) {
      for (final n in _session.document.children(parent)) {
        final text = n.text?.text.replaceAll('￼', '');
        if (n.id == node.id) {
          passed = true;
          before.write(text!.substring(0, start));
          after.write(text.substring(end));
        } else if (text != null && n.type != 'notes') {
          (passed ? after : before).write(text);
        }
        walk(n.id);
      }
    }

    walk(widget.slide.id);
    return (before: before.toString(), after: after.toString());
  }

  @override
  Rect? rectAt(int offset) {
    final node = _editingNode;
    final frame = node == null ? null : widget.painter.textOf(node);
    final box = context.findRenderObject();
    if (frame == null || box is! RenderBox) return null;
    final (start, _) = FlowEditing(node!.text!).paragraphAt(_selection.extent);
    final caret = frame.caretAt(start + offset);
    final transform = widget.painter.transformOf(node);
    Offset global(Offset p) => box.localToGlobal(_origin + transform.apply(p) * _scale);
    return Rect.fromPoints(global(caret.topLeft), global(caret.bottomRight));
  }

  @override
  void write(int start, int end, String text, {Uri? link}) {
    final node = _editingNode;
    if (node == null) return;
    final editing = FlowEditing(node.text!);
    final (paragraph, _) = editing.paragraphAt(_selection.extent);
    start += paragraph;
    end += paragraph;
    if (link == null) {
      _replace(node, start, end, text);
      return;
    }
    final attributes = _selection.typing ?? editing.typingAttributes(start);
    final d = Delta()..retain(start);
    if (end > start) d.delete(end - start);
    d
      ..insert(text, {...attributes, ...linkProps(link)})
      ..insert(' ', attributes.isEmpty ? null : attributes);
    final caret = start + text.length + 1;
    _text(node, d.chop(), caret, caret);
  }

  /// The link under a click that opens it, if any: in the text at a point
  /// of the slide.
  Uri? _linkAt(Offset p) => widget.onOpenLink == null || !(_ctrl || _session.readOnly) ? null : _linkUnder(p);

  Uri? _linkUnder(Offset p) {
    var node = _editingNode;
    if (node == null || !_contains(node, p)) {
      final hit = _shapeAt(p);
      node = hit == null ? null : _cellAt(hit, p) ?? hit;
    }
    final frame = node?.text == null ? null : widget.painter.textOf(node!);
    if (frame == null) return null;
    final at = widget.painter.transformOf(node!).invert().apply(p);
    final offset = frame.offsetAt(at);
    final caret = frame.caretAt(offset);
    if (at.dy < caret.top || at.dy > caret.bottom || (at.dx - caret.left).abs() > caret.height) return null;
    final url = linkOf(FlowEditing(node.text!).attributesAt(offset) ?? const {});
    final uri = url == null ? null : Uri.tryParse(url);
    return uri != null && uri.hasScheme ? uri : null;
  }

  /// The shape at a point of the slide, the topmost; a shape of a group
  /// gives the group, unless it or one of its shapes is selected, as in
  /// PowerPoint, where a second click goes into the group.
  Node? _shapeAt(Offset p) {
    Node? top(Node parent) => _deck.shapesOf(parent).reversed.where((s) => _contains(s, p)).firstOrNull;
    var hit = top(widget.slide);
    while (hit != null && hit.type == 'grp' && _selection.shapes.any((id) => id == hit!.id || _isUnder(id, hit.id))) {
      final child = top(hit);
      if (child == null) break;
      hit = child;
    }
    return hit;
  }

  bool _isUnder(String id, String ancestor) {
    for (var n = _session.document[id]; n != null && n.type != 'slide'; n = _session.document[n.parent]) {
      if (n.parent == ancestor) return true;
    }
    return false;
  }

  /// Whether a shape can be moved and resized: on the slide or in a group.
  bool _placeable(Node shape) => shape.parent == widget.slide.id || _session.document[shape.parent]?.type == 'grp';

  /// The transform from the coordinates a shape is placed in, its group's
  /// or the slide's, to the slide.
  Matrix4Like _space(Node shape) {
    final parent = _session.document[shape.parent];
    return parent != null && parent.type == 'grp' ? widget.painter.transformOf(parent) : Matrix4Like.identity();
  }

  /// How far the drag went, in the coordinates [shape] is placed in.
  Offset _dragIn(Node shape) {
    final inverse = _space(shape).invert();
    return inverse.apply(_dragTo) - inverse.apply(_dragFrom);
  }

  bool _contains(Node shape, Offset p) {
    if (shape.type == 'grp' || shape.type == 'alt') {
      return _deck.shapesOf(shape).any((c) => _contains(c, p));
    }
    final size = widget.painter.sizeOf(shape);
    if (size == null) return false;
    final local = widget.painter.transformOf(shape).invert().apply(p);
    final slack = 4 / _scale;
    return Rect.fromLTWH(-slack, -slack, size.width + 2 * slack, size.height + 2 * slack).contains(local);
  }

  /// The cell of a table at a point, null on the edge of the table, where
  /// a click takes the table itself.
  Node? _cellAt(Node frame, Offset p) {
    final table = widget.painter.tableOf(frame);
    if (table == null) return null;
    final local = widget.painter.transformOf(frame).invert().apply(p);
    if (!(Offset.zero & table.size).deflate(4 / _scale).contains(local)) return null;
    final cell = table.cellAt(local);
    return cell == null ? null : _session.document[cell.node.id];
  }

  /// The cells selected, when they are.
  List<CellLayout> get _selectedCells => [
    for (final id in _selection.shapes)
      if (_session.document[id] case final n? when n.type == 'tc') ?widget.painter.cellOf(n),
  ];

  /// Selects the cells from [anchor] to [to], or the text of [anchor] when
  /// they are the same cell.
  void _selectRange(Node anchor, Node to) {
    final frame = widget.painter.frameOf(anchor);
    final table = frame == null ? null : widget.painter.tableOf(frame);
    final a = widget.painter.cellOf(anchor), b = widget.painter.cellOf(to);
    if (table == null || a == null || b == null || widget.painter.frameOf(to)?.id != frame!.id) return;
    _anchor = anchor.id;
    if (a == b) {
      if (_selection.editing != anchor.id) _selection.edit(anchor.id, _anchorOffset);
      return;
    }
    final ids = [for (final c in table.range(a, b)) c.node.id];
    if (_selection.editing != null || !_selection.shapes.containsAll(ids) || _selection.shapes.length != ids.length) _selection.selectShapes(ids);
  }

  /// The edge of a table at a point that a drag moves: a column's right or
  /// a row's bottom, where no merged cell covers it.
  (Node, _Drag, int)? _edgeAt(Offset p) {
    if (_session.readOnly) return null;
    final slack = 4 / _scale;
    for (final shape in _deck.shapesOf(widget.slide).reversed) {
      final table = widget.painter.tableOf(shape);
      if (table == null) {
        if (_contains(shape, p)) return null;
        continue;
      }
      final local = widget.painter.transformOf(shape).invert().apply(p);
      if (!(Offset.zero & table.size).inflate(slack).contains(local)) continue;
      final rtl = (shape.attributes['tbl'] as Map<String, Object?>?)?['rtl'] == true;
      final cell = table.cellAt(Offset(local.dx.clamp(0, table.size.width - 0.01), local.dy.clamp(0, table.size.height - 0.01)));
      for (var i = 1; i < table.columns.length && !rtl; i++) {
        final inside = cell != null && cell.col < i && i < cell.col + cell.colSpan;
        if ((local.dx - table.columns[i]).abs() < slack && !inside) return (shape, _Drag.column, i);
      }
      for (var i = 1; i < table.rows.length; i++) {
        final inside = cell != null && cell.row < i && i < cell.row + cell.rowSpan;
        if ((local.dy - table.rows[i]).abs() < slack && !inside) return (shape, _Drag.row, i);
      }
      return null;
    }
    return null;
  }

  /// How far the edge dragged moves, in points of its table.
  double get _edgeShift {
    final edge = _edge;
    if (edge == null) return 0;
    final inverse = widget.painter.transformOf(edge.$1).invert();
    final d = inverse.apply(_dragTo) - inverse.apply(_dragFrom);
    return edge.$2 == _Drag.column ? d.dx : d.dy;
  }

  /// Where the handles of a shape are, on the slide.
  Map<_Handle, Offset> _handles(Node shape) {
    final box = _deck.styleOf(shape).bounds;
    if (box == null || shape.attributes['frame'] == 'table') return const {};
    final t = widget.painter.transformOf(shape);
    final w = box.width, h = box.height;
    return {
      _Handle.topLeft: t.apply(Offset.zero),
      _Handle.top: t.apply(Offset(w / 2, 0)),
      _Handle.topRight: t.apply(Offset(w, 0)),
      _Handle.right: t.apply(Offset(w, h / 2)),
      _Handle.bottomRight: t.apply(Offset(w, h)),
      _Handle.bottom: t.apply(Offset(w / 2, h)),
      _Handle.bottomLeft: t.apply(Offset(0, h)),
      _Handle.left: t.apply(Offset(0, h / 2)),
      _Handle.rotate: t.apply(Offset(w / 2, -20 / _scale)),
    };
  }

  Node? get _single {
    final shapes = _selection.shapes;
    if (shapes.length != 1) return null;
    return _session.document[shapes.first];
  }

  // pointer

  /// The box of the balloon of a comment thread, in points.
  Rect _balloon(CommentThread t) {
    final side = 22 / _scale;
    return threadPlace(t) & Size(side, side);
  }

  void _down(PointerDownEvent e) {
    if (_commands.answering) return;
    _focus.requestFocus();
    final p = _toSlide(e.localPosition);
    _cards.hide();
    if (_linkAt(p) case final uri?) {
      if (_ctrl || e.kind == PointerDeviceKind.mouse || !_cards.tap(uri, e.position)) widget.onOpenLink!(uri);
      return;
    }
    final comment = widget.comments.where((t) => _balloon(t).contains(p)).lastOrNull;
    if (comment != null) {
      widget.onComment?.call(comment.id);
      return;
    }
    final now = DateTime.now();
    final again = now.difference(_lastDown) < kDoubleTapTimeout && (e.localPosition - _lastDownAt).distance < kDoubleTapSlop;
    _clicks = again ? _clicks + 1 : 1;
    _lastDown = now;
    _lastDownAt = e.localPosition;
    _dragFrom = _dragTo = p;
    _moved = false;
    _goalX = null;
    final shift = HardwareKeyboard.instance.isShiftPressed;

    final edge = _edgeAt(p);
    if (edge != null) {
      _edge = edge;
      _drag = edge.$2;
      _pressed = edge.$1;
      return;
    }

    final editing = _editingNode;
    if (editing != null && _contains(editing, p)) {
      final frame = widget.painter.textOf(editing);
      if (frame == null) return;
      final offset = frame.offsetAt(widget.painter.transformOf(editing).invert().apply(p));
      final editor = FlowEditing(editing.text!);
      if (_clicks == 2) {
        final (s, t) = editor.wordAt(offset);
        _selection.edit(editing.id, s, t);
      } else if (_clicks >= 3) {
        final (s, t) = editor.paragraphAt(offset);
        _selection.edit(editing.id, s, t);
      } else {
        _selection.edit(editing.id, shift ? _selection.base : offset, offset);
      }
      _anchor = editing.id;
      _anchorOffset = _selection.base;
      _drag = editing.type == 'tc' ? _Drag.cells : _Drag.text;
      return;
    }

    final single = _single;
    if (single != null && _placeable(single) && !_session.readOnly) {
      for (final h in _handles(single).entries) {
        if ((h.value - p).distance * _scale < 8) {
          _drag = h.key == _Handle.rotate ? _Drag.rotate : _Drag.resize;
          _handle = h.key;
          _pressed = single;
          return;
        }
      }
    }

    final hit = _shapeAt(p);
    _pressed = hit;
    _wasSelected = hit != null && _selection.shapes.length == 1 && _selection.shapes.contains(hit.id);
    if (hit == null) {
      _drag = _Drag.none;
      _selection.clear();
      return;
    }
    if (_clicks >= 2 && hit.type == 'sp' && hit.text != null) {
      _startEditing(hit, at: p, word: true);
      _drag = _Drag.text;
      return;
    }
    final cell = _session.readOnly ? null : _cellAt(hit, p);
    final anchor = _anchor == null ? null : _session.document[_anchor!];
    final inRange = _selection.editing != null || _selectedCells.isNotEmpty;
    if (cell != null && shift && anchor != null && inRange && widget.painter.frameOf(anchor)?.id == hit.id) {
      _selectRange(anchor, cell);
      _drag = _Drag.cells;
      return;
    }
    if (cell != null && !shift) {
      _startEditing(cell, at: p, word: _clicks >= 2);
      _anchor = cell.id;
      _anchorOffset = _selection.base;
      _drag = _Drag.cells;
      return;
    }
    if (shift) {
      final ids = {..._selection.shapes};
      if (!ids.remove(hit.id)) ids.add(hit.id);
      _selection.selectShapes(ids);
    } else if (!_selection.shapes.contains(hit.id)) {
      _selection.selectShapes([hit.id]);
    }
    _drag = _session.readOnly ? _Drag.none : _Drag.move;
  }

  void _move(PointerMoveEvent e) {
    final p = _toSlide(e.localPosition);
    if ((p - _dragFrom).distance * _scale > 3) _moved = true;
    _dragTo = p;
    switch (_drag) {
      case _Drag.text:
        final editing = _editingNode;
        final frame = editing == null ? null : widget.painter.textOf(editing);
        if (editing != null && frame != null) {
          final offset = frame.offsetAt(widget.painter.transformOf(editing).invert().apply(p));
          _selection.edit(editing.id, _selection.base, offset);
        }
      case _Drag.cells:
        final anchor = _anchor == null ? null : _session.document[_anchor!];
        final frame = anchor == null ? null : widget.painter.frameOf(anchor);
        final over = frame == null ? null : _cellAt(frame, p);
        if (anchor != null && over != null) _selectRange(anchor, over);
        final editing = _editingNode;
        final text = editing == null ? null : widget.painter.textOf(editing);
        if (editing != null && text != null) {
          _selection.edit(editing.id, _selection.base, text.offsetAt(widget.painter.transformOf(editing).invert().apply(p)));
        }
      case _Drag.move || _Drag.resize || _Drag.rotate || _Drag.column || _Drag.row:
        setState(() {});
      case _Drag.none:
    }
  }

  void _hover(PointerHoverEvent e) {
    final edge = _edgeAt(_toSlide(e.localPosition));
    final cursor = switch (edge?.$2) {
      _Drag.column => SystemMouseCursors.resizeColumn,
      _Drag.row => SystemMouseCursors.resizeRow,
      _ => MouseCursor.defer,
    };
    if (cursor != _cursor) setState(() => _cursor = cursor);
    _cards.point(_linkUnder(_toSlide(e.localPosition)), e.position);
  }

  void _up(PointerUpEvent e) {
    final drag = _drag;
    if (!_moved) {
      _drag = _Drag.none;
      _edge = null;
      // a click on a shape already selected goes into its text
      final pressed = _pressed;
      if (drag == _Drag.move && pressed != null && _selection.shapes.length == 1 && pressed.type == 'sp' && _clicks == 1 && _wasSelected) {
        _startEditing(pressed, at: _dragFrom);
      }
      setState(() {});
      return;
    }
    final edit = switch (drag) {
      _Drag.move => DeckEdits(_deck).place(_previews()),
      _Drag.resize => DeckEdits(_deck).place(_previews()),
      _Drag.rotate => _single == null ? Edit() : DeckEdits(_deck).rotate(_single!, _rotation(_single!)),
      _Drag.column || _Drag.row => _edgeEdit(),
      _ => Edit(),
    };
    _drag = _Drag.none;
    _edge = null;
    if (!edit.isEmpty) _session.edit(edit);
    setState(() {});
  }

  /// Whether the shape pressed was the one selected, which a click then
  /// goes into the text of.
  var _wasSelected = false;

  Edit _edgeEdit() {
    final edge = _edge;
    final frame = edge == null ? null : _session.document[edge.$1.id];
    final table = frame == null ? null : widget.painter.tableOf(frame);
    if (edge == null || frame == null || table == null) return Edit();
    final edits = TableEdits(_session.document, frame, table);
    return edge.$2 == _Drag.column ? edits.resizeColumn(edge.$3, _edgeShift) : edits.resizeRow(edge.$3, _edgeShift);
  }

  void _startEditing(Node shape, {Offset? at, bool word = false}) {
    if (_session.readOnly) return;
    final frame = widget.painter.textOf(shape);
    final text = shape.text!;
    var offset = text.length - 1;
    if (frame != null && at != null) {
      offset = frame.offsetAt(widget.painter.transformOf(shape).invert().apply(at));
    }
    if (word) {
      final (s, t) = FlowEditing(text).wordAt(offset);
      _selection.edit(shape.id, s, t);
    } else {
      _selection.edit(shape.id, offset);
    }
  }

  /// Where the shapes being dragged would go, in the coordinates each is
  /// placed in.
  Map<Node, Rect> _previews() {
    final out = <Node, Rect>{};
    if (_drag == _Drag.move) {
      for (final id in _selection.shapes) {
        final n = _session.document[id];
        final box = n == null || !_placeable(n) ? null : _deck.styleOf(n).bounds;
        if (box != null) out[n!] = box.shift(_dragIn(n));
      }
    } else if (_drag == _Drag.resize) {
      final n = _single;
      final box = n == null ? null : _deck.styleOf(n).bounds;
      if (n != null && box != null) out[n] = _resized(n, box);
    }
    return out;
  }

  /// The bounds of a shape resized by the handle dragged, where it is
  /// placed: the opposite side stays where it is, rotated or not.
  Rect _resized(Node shape, Rect box) {
    final t = widget.painter.transformOf(shape);
    final inverse = t.invert();
    final from = inverse.apply(_dragFrom), to = inverse.apply(_dragTo);
    final d = to - from;
    var l = 0.0, top = 0.0, r = box.width, b = box.height;
    switch (_handle) {
      case _Handle.topLeft:
        l += d.dx;
        top += d.dy;
      case _Handle.top:
        top += d.dy;
      case _Handle.topRight:
        r += d.dx;
        top += d.dy;
      case _Handle.right:
        r += d.dx;
      case _Handle.bottomRight:
        r += d.dx;
        b += d.dy;
      case _Handle.bottom:
        b += d.dy;
      case _Handle.bottomLeft:
        l += d.dx;
        b += d.dy;
      case _Handle.left:
        l += d.dx;
      default:
    }
    if (HardwareKeyboard.instance.isShiftPressed && box.width > 0 && box.height > 0) {
      // keeping the proportions, from the corners
      final ratio = box.width / box.height;
      if ((r - l) / (b - top) > ratio) {
        r = l + (b - top) * ratio;
      } else {
        b = top + (r - l) / ratio;
      }
    }
    final local = Rect.fromLTRB(math.min(l, r), math.min(top, b), math.max(l, r), math.max(top, b));
    final center = _space(shape).invert().apply(t.apply(local.center));
    return Rect.fromCenter(center: center, width: math.max(local.width, 1), height: math.max(local.height, 1));
  }

  double _rotation(Node shape) {
    final c = _deck.styleOf(shape).bounds!.center;
    final to = _space(shape).invert().apply(_dragTo);
    var degrees = math.atan2(to.dy - c.dy, to.dx - c.dx) * 180 / math.pi + 90;
    if (HardwareKeyboard.instance.isShiftPressed) degrees = (degrees / 15).round() * 15.0;
    return (degrees % 360 + 360) % 360;
  }

  // keys

  bool _pasted(String text) {
    final current = _editingNode;
    if (current == null || _session.readOnly || !_focus.hasFocus) return false;
    _replace(current, _selection.start, _selection.end, text.replaceAll('\r\n', '\n'));
    return true;
  }

  KeyEventResult _key(FocusNode node, KeyEvent e) {
    if (_commands.key(e)) return KeyEventResult.handled;
    if (e is KeyUpEvent) return KeyEventResult.ignored;
    if (widget.onShortcut?.call(e) ?? false) return KeyEventResult.handled;
    final editing = _editingNode;
    if (editing != null) return _textKey(editing, e) ? KeyEventResult.handled : KeyEventResult.ignored;
    return _shapeKey(e) ? KeyEventResult.handled : KeyEventResult.ignored;
  }

  bool get _ctrl => HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed;
  bool get _shift => HardwareKeyboard.instance.isShiftPressed;

  bool _shapeKey(KeyEvent e) {
    final shapes = [for (final id in _selection.shapes) ?_session.document[id]];
    if (shapes.isEmpty) return false;
    final key = e.logicalKey;
    if (key == LogicalKeyboardKey.escape) {
      _selection.clear();
      return true;
    }
    if (_session.readOnly) return false;
    final cells = _selectedCells;
    if (cells.isNotEmpty) {
      final frame = widget.painter.frameOf(cells.first.node);
      final table = frame == null ? null : widget.painter.tableOf(frame);
      if ((key == LogicalKeyboardKey.delete || key == LogicalKeyboardKey.backspace) && table != null) {
        _session.edit(TableEdits(_session.document, frame!, table).clear(cells));
        return true;
      }
      return false;
    }
    if (key == LogicalKeyboardKey.delete || key == LogicalKeyboardKey.backspace) {
      _session.edit(DeckEdits(_deck).delete(shapes));
      _selection.clear();
      return true;
    }
    final nudge = switch (key) {
      LogicalKeyboardKey.arrowLeft => const Offset(-1, 0),
      LogicalKeyboardKey.arrowRight => const Offset(1, 0),
      LogicalKeyboardKey.arrowUp => const Offset(0, -1),
      LogicalKeyboardKey.arrowDown => const Offset(0, 1),
      _ => null,
    };
    if (nudge != null) {
      final step = _ctrl ? 0.75 : 7.2;
      _session.edit(DeckEdits(_deck).place({
        for (final s in shapes)
          if (_placeable(s))
            if (_deck.styleOf(s).bounds case final b?) s: b.shift(_nudgeIn(s, nudge * step)),
      }));
      return true;
    }
    final single = shapes.length == 1 ? shapes.single : null;
    if (single != null && single.type == 'sp' && single.text != null) {
      if (key == LogicalKeyboardKey.enter || key == LogicalKeyboardKey.f2) {
        _selection.edit(single.id, 0, single.text!.length - 1);
        return true;
      }
      // typing on a selected shape replaces its text, as in PowerPoint
      final char = e.character;
      if (char != null && char.isNotEmpty && char.codeUnitAt(0) >= 0x20 && !_ctrl) {
        _selection.edit(single.id, 0, single.text!.length - 1);
        _replace(single, 0, single.text!.length - 1, char);
        return true;
      }
    }
    return false;
  }

  /// A nudge on the slide, in the coordinates [shape] is placed in.
  Offset _nudgeIn(Node shape, Offset by) {
    final inverse = _space(shape).invert();
    return inverse.apply(by) - inverse.apply(Offset.zero);
  }

  bool _textKey(Node shape, KeyEvent e) {
    final key = e.logicalKey;
    final editor = FlowEditing(shape.text!);
    final last = shape.text!.length - 1;
    final s = _selection;
    void caret(int to, {bool keepGoal = false}) {
      if (!keepGoal) _goalX = null;
      to = to.clamp(0, last);
      if (_shift) {
        s.edit(shape.id, s.base, to);
      } else {
        s.edit(shape.id, to);
      }
      _input?.setEditingState(currentTextEditingValue);
    }

    final frame = widget.painter.textOf(shape);
    switch (key) {
      case LogicalKeyboardKey.escape:
        _selection.selectShapes([shape.type == 'tc' ? widget.painter.frameOf(shape)?.id ?? shape.id : shape.id]);
        return true;
      case LogicalKeyboardKey.arrowLeft:
        caret(!_shift && !s.collapsed ? s.start : editor.previous(s.extent, word: _ctrl));
        return true;
      case LogicalKeyboardKey.arrowRight:
        caret(!_shift && !s.collapsed ? s.end : editor.next(s.extent, word: _ctrl));
        return true;
      case LogicalKeyboardKey.arrowUp || LogicalKeyboardKey.arrowDown:
        if (frame == null) return true;
        _goalX ??= frame.lineX(s.extent);
        caret(frame.verticalMove(s.extent, key == LogicalKeyboardKey.arrowUp ? -1 : 1, x: _goalX), keepGoal: true);
        return true;
      case LogicalKeyboardKey.home:
        caret(_ctrl ? 0 : (frame?.lineAt(s.extent).$1 ?? editor.paragraphAt(s.extent).$1));
        return true;
      case LogicalKeyboardKey.end:
        caret(_ctrl ? last : (frame?.lineAt(s.extent).$2 ?? editor.paragraphAt(s.extent).$2));
        return true;
    }
    if (_ctrl) {
      switch (key) {
        case LogicalKeyboardKey.keyA:
          s.edit(shape.id, 0, last);
          return true;
        case LogicalKeyboardKey.keyC || LogicalKeyboardKey.keyX:
          if (!s.collapsed) {
            unawaited(Clipboard.setData(ClipboardData(text: editor.text.substring(s.start, s.end).replaceAll('\v', '\n'))));
            if (key == LogicalKeyboardKey.keyX) _replace(shape, s.start, s.end, '');
          }
          return true;
        case LogicalKeyboardKey.keyV:
          // left to the browser, which then tells the page to paste
          if (_unwatchPaste != null) return false;
          unawaited(Clipboard.getData(Clipboard.kTextPlain).then((data) {
            final text = data?.text;
            if (text != null) _pasted(text);
          }));
          return true;
        case LogicalKeyboardKey.backspace:
          _delete(shape, editor.deleteBackward(editor.previous(s.start, word: true), s.collapsed ? s.start : s.end), editor.previous(s.start, word: true));
          return true;
        case LogicalKeyboardKey.delete:
          _delete(shape, editor.deleteForward(s.start, s.collapsed ? editor.next(s.start, word: true) : s.end), s.start);
          return true;
      }
      return false;
    }
    switch (key) {
      case LogicalKeyboardKey.backspace:
        final d = editor.deleteBackward(s.start, s.end);
        _delete(shape, d, s.collapsed ? editor.previous(s.start) : s.start);
        return true;
      case LogicalKeyboardKey.delete:
        _delete(shape, editor.deleteForward(s.start, s.end), s.start);
        return true;
      case LogicalKeyboardKey.enter || LogicalKeyboardKey.numpadEnter:
        _replace(shape, s.start, s.end, _shift ? '\v' : '\n');
        return true;
      case LogicalKeyboardKey.tab when shape.type == 'tc':
        _nextCell(shape, back: _shift);
        return true;
      case LogicalKeyboardKey.tab:
        final paragraphs = editor.paragraphsIn(s.start, s.end);
        final atStart = s.collapsed && editor.paragraphAt(s.start).$1 == s.start;
        if (!s.collapsed || atStart || (paragraphs.first['bu'] ?? '').isNotEmpty) {
          final lvl = int.tryParse(paragraphs.first['lvl'] ?? '') ?? 0;
          final next = (lvl + (_shift ? -1 : 1)).clamp(0, 8);
          _text(shape, editor.formatParagraphs(s.start, s.end, {'lvl': next == 0 ? '' : '$next'}), s.base, s.extent);
        } else {
          _replace(shape, s.start, s.end, '\t');
        }
        return true;
    }
    return false;
  }

  /// Selects the text of the cell after [cell], or before it; after the
  /// last, as PowerPoint, a new row.
  void _nextCell(Node cell, {required bool back}) {
    final frame = widget.painter.frameOf(cell);
    final table = frame == null ? null : widget.painter.tableOf(frame);
    if (frame == null || table == null) return;
    final cells = table.cells.values.toList();
    final i = cells.indexWhere((c) => c.node.id == cell.id) + (back ? -1 : 1);
    if (i == cells.length && !_session.readOnly) {
      final edit = TableEdits(_session.document, frame, table).insertRow(table.rows.length - 1);
      if (_session.edit(edit)) _selection.edit(edit.changes.firstWhere((c) => c.type == 'tc').id, 0);
    } else if (i >= 0 && i < cells.length) {
      final next = _session.document[cells[i].node.id];
      if (next != null) _selection.edit(next.id, 0, next.text!.length - 1);
    }
    _input?.setEditingState(currentTextEditingValue);
  }

  /// Replaces a range of a shape's text by what was typed.
  void _replace(Node shape, int start, int end, String text) {
    final editor = FlowEditing(shape.text!);
    final attributes = _selection.typing ?? editor.typingAttributes(start);
    final d = text.isEmpty ? editor.delete(start, end) : editor.replace(start, end, text, attributes);
    final caret = start + text.length;
    _text(shape, d, caret, caret);
  }

  void _delete(Node shape, Delta? d, int caret) {
    if (d == null) return;
    _text(shape, d, caret, caret);
  }

  void _text(Node shape, Delta d, int base, int extent) {
    _local = true;
    final ok = d.isEmpty || _session.edit(Edit([Change.text(shape.id, d)]));
    _local = false;
    final node = _session.document[shape.id];
    if (node == null) return;
    final last = node.text!.length - 1;
    if (ok) _selection.edit(shape.id, base.clamp(0, last), extent.clamp(0, last));
    _input?.setEditingState(currentTextEditingValue);
    _commands.follow();
    _restartBlink();
  }

  // text input

  void _openInput() {
    if (_input?.attached ?? false) {
      _input!.setEditingState(currentTextEditingValue);
      return;
    }
    _input = TextInput.attach(
      this,
      const TextInputConfiguration(
        inputType: TextInputType.multiline,
        inputAction: TextInputAction.newline,
        enableDeltaModel: true,
        autocorrect: false,
        enableSuggestions: false,
      ),
    )
      ..setEditingState(currentTextEditingValue)
      ..show();
  }

  @override
  TextEditingValue get currentTextEditingValue {
    final node = _editingNode;
    if (node == null) return TextEditingValue.empty;
    final text = node.text!.text;
    return TextEditingValue(
      text: text.substring(0, text.length - 1),
      selection: TextSelection(baseOffset: _selection.base, extentOffset: _selection.extent),
    );
  }

  /// Whether what the platform typed is not for the text: an answer is on
  /// its way, and the platform is given back what the text holds.
  bool get _held {
    if (_commands.answering) _input?.setEditingState(currentTextEditingValue);
    return _commands.answering;
  }

  @override
  void updateEditingValueWithDeltas(List<TextEditingDelta> deltas) {
    if (_held) return;
    for (final delta in deltas) {
      final node = _editingNode;
      if (node == null) return;
      switch (delta) {
        case TextEditingDeltaInsertion(textInserted: '\n') when _commands.take():
          _input?.setEditingState(currentTextEditingValue);
          continue;
        case TextEditingDeltaInsertion(:final insertionOffset, :final textInserted):
          _replace(node, insertionOffset, insertionOffset, textInserted);
        case TextEditingDeltaDeletion(:final deletedRange):
          _replace(node, deletedRange.start, deletedRange.end, '');
        case TextEditingDeltaReplacement(:final replacedRange, :final replacementText):
          _replace(node, replacedRange.start, replacedRange.end, replacementText);
        case TextEditingDeltaNonTextUpdate():
      }
      final selection = delta.selection;
      if (selection.isValid) {
        final last = (_editingNode?.text?.length ?? 1) - 1;
        _selection.edit(node.id, selection.baseOffset.clamp(0, last), selection.extentOffset.clamp(0, last));
      }
    }
  }

  /// The whole value, from platforms that do not send deltas: what changed
  /// is found by comparing it with the text.
  @override
  void updateEditingValue(TextEditingValue value) {
    final node = _editingNode;
    if (node == null || _held) return;
    final old = currentTextEditingValue.text, now = value.text;
    if (old != now) {
      var prefix = 0;
      while (prefix < old.length && prefix < now.length && old.codeUnitAt(prefix) == now.codeUnitAt(prefix)) {
        prefix++;
      }
      var suffix = 0;
      while (suffix < old.length - prefix &&
          suffix < now.length - prefix &&
          old.codeUnitAt(old.length - 1 - suffix) == now.codeUnitAt(now.length - 1 - suffix)) {
        suffix++;
      }
      _replace(node, prefix, old.length - suffix, now.substring(prefix, now.length - suffix));
    }
    final last = (_editingNode?.text?.length ?? 1) - 1;
    if (value.selection.isValid) {
      _selection.edit(node.id, value.selection.baseOffset.clamp(0, last), value.selection.extentOffset.clamp(0, last));
    }
  }

  @override
  void performAction(TextInputAction action) {
    final node = _editingNode;
    if (node == null || action != TextInputAction.newline || _held || _commands.take()) return;
    _replace(node, _selection.start, _selection.end, '\n');
  }

  @override
  void connectionClosed() {
    _input = null;
  }

  @override
  AutofillScope? get currentAutofillScope => null;

  @override
  void performPrivateCommand(String action, Map<String, dynamic> data) {}

  @override
  void updateFloatingCursor(RawFloatingCursorPoint point) {}

  @override
  void showAutocorrectionPromptRect(int start, int end) {}

  @override
  void insertTextPlaceholder(Size size) {}

  @override
  void removeTextPlaceholder() {}

  @override
  void showToolbar() {}

  @override
  void didChangeInputControl(TextInputControl? oldControl, TextInputControl? newControl) {}

  @override
  void performSelector(String selectorName) {}

  @override
  void insertContent(KeyboardInsertedContent content) {}

  @override
  bool onFocusReceived() => false;

  // drawing

  @override
  Widget build(BuildContext context) {
    final size = _deck.size;
    return LayoutBuilder(builder: (context, constraints) {
      const margin = 24.0;
      _scale = math.max(
        math.min((constraints.maxWidth - 2 * margin) / size.width, (constraints.maxHeight - 2 * margin) / size.height),
        0.05,
      );
      _origin = Offset((constraints.maxWidth - size.width * _scale) / 2, (constraints.maxHeight - size.height * _scale) / 2);
      final theme = Theme.of(context);
      final canvas = Focus(
        focusNode: _focus,
        onKeyEvent: _key,
        child: Listener(
          onPointerDown: _down,
          onPointerMove: _move,
          onPointerUp: _up,
          onPointerHover: _hover,
          child: MouseRegion(
            onExit: (_) => _cards.hide(),
            cursor: _cursor != MouseCursor.defer
                ? _cursor
                : (_selection.editing != null ? SystemMouseCursors.text : SystemMouseCursors.basic),
            child: CustomPaint(
              size: Size(constraints.maxWidth, constraints.maxHeight),
              painter: _CanvasPainter(this, theme.colorScheme),
            ),
          ),
        ),
      );
      return CommandsMenu(commands: _commands, child: canvas);
    });
  }
}

/// Draws the slide, and over it what editing it shows: the prompts of empty
/// placeholders, selections, handles, carets, and those of the others.
class _CanvasPainter extends CustomPainter {
  _CanvasPainter(this.state, this.colors);

  final SlideCanvasState state;
  final ColorScheme colors;

  static const _peerColors = [
    Color(0xFFD83B01), Color(0xFF107C10), Color(0xFF8764B8), Color(0xFF0078D4), Color(0xFFC239B3), Color(0xFF00B7C3),
  ];

  @override
  void paint(Canvas canvas, Size size) {
    final w = state.widget;
    final deck = w.deck;
    final painter = w.painter;
    final scale = state._scale;
    canvas.save();
    canvas.translate(state._origin.dx, state._origin.dy);
    canvas.scale(scale);
    final slide = Offset.zero & deck.size;
    canvas.drawRect(slide.shift(Offset(2 / scale, 2 / scale)), Paint()..color = const Color(0x33000000));
    painter.paint(canvas, w.slide);

    final selection = w.selection;
    // the prompts of empty placeholders
    for (final shape in deck.shapesOf(w.slide)) {
      if (!painter.isEmptyPlaceholder(shape) || selection.editing == shape.id) continue;
      final kind = (shape.attributes['ph'] as Map<String, Object?>?)?['type'] as String? ?? 'obj';
      final prompt = w.strings.prompt(kind);
      canvas.save();
      canvas.transform(painter.transformOf(shape).storage);
      final box = deck.styleOf(shape).bounds;
      if (box != null) {
        _dashed(canvas, Offset.zero & box.size, const Color(0xFF8A8A8A), 1 / scale);
        if (prompt.isNotEmpty) {
          final mark = shape.text!.ops.last.attributes;
          painter.layout(shape, Delta([Op.insert('$prompt\n', mark)]), color: const Color(0xFF7F7F7F))?.paint(canvas);
        }
      }
      canvas.restore();
    }

    // the selections of others
    var i = 0;
    for (final peer in w.session.peers) {
      final color = _peerColors[i++ % _peerColors.length];
      final s = peer.selection;
      final node = s == null ? null : w.session.document[s.node];
      final frame = node == null ? null : painter.textOf(node);
      if (node == null || frame == null || deck.pageOf(node)?.id != w.slide.id) continue;
      canvas.save();
      canvas.transform(painter.transformOf(node).storage);
      for (final r in frame.selection(s!.start, s.end)) {
        canvas.drawRect(r, Paint()..color = color.withValues(alpha: 0.25));
      }
      final caret = frame.caretAt(s.extent);
      canvas.drawRect(_bar(caret, 1.5 / scale), Paint()..color = color);
      final label = TextPainter(
        text: TextSpan(text: peer.name, style: TextStyle(fontSize: 10 / scale, color: Colors.white)),
        textDirection: TextDirection.ltr,
      )..layout();
      final tag = Rect.fromLTWH(caret.left, caret.top - label.height - 2 / scale, label.width + 6 / scale, label.height + 2 / scale);
      canvas.drawRect(tag, Paint()..color = color);
      label.paint(canvas, tag.topLeft + Offset(3 / scale, 1 / scale));
      canvas.restore();
    }

    // this person's text selection and caret
    final editing = state._editingNode;
    if (editing != null) {
      final frame = painter.textOf(editing);
      if (frame != null) {
        canvas.save();
        canvas.transform(painter.transformOf(editing).storage);
        for (final r in frame.selection(selection.start, selection.end)) {
          canvas.drawRect(r, Paint()..color = colors.primary.withValues(alpha: 0.3));
        }
        if (selection.collapsed && state._caretOn) {
          final c = frame.caretAt(selection.extent);
          canvas.drawRect(_bar(c, 1.2 / scale), Paint()..color = Colors.black);
        }
        canvas.restore();
      }
    }

    // selected shapes, their handles, and where a drag takes them
    for (final id in selection.shapes) {
      final node = w.session.document[id];
      final shape = node?.type == 'tc' ? painter.frameOf(node!) : node;
      final size = shape == null ? null : painter.sizeOf(shape);
      if (shape == null || size == null) continue;
      if (node!.type == 'tc' && selection.editing != id) {
        final cell = painter.sizeOf(node);
        canvas.save();
        canvas.transform(painter.transformOf(node).storage);
        if (cell != null) canvas.drawRect(Offset.zero & cell, Paint()..color = colors.primary.withValues(alpha: 0.2));
        canvas.restore();
      }
      canvas.save();
      canvas.transform(painter.transformOf(shape).storage);
      final outline = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1 / scale
        ..color = colors.primary;
      if (selection.editing == id) {
        _dashed(canvas, Offset.zero & size, colors.primary, 1 / scale);
      } else {
        canvas.drawRect(Offset.zero & size, outline);
      }
      canvas.restore();
      final group = w.session.document[shape.parent];
      if (group != null && group.type == 'grp') {
        final size = painter.sizeOf(group);
        canvas.save();
        canvas.transform(painter.transformOf(group).storage);
        if (size != null) _dashed(canvas, Offset.zero & size, colors.primary, 1 / scale);
        canvas.restore();
      }
      if (selection.shapes.length == 1 && selection.editing == null && state._placeable(shape)) {
        for (final h in state._handles(shape).entries) {
          final fill = Paint()..color = Colors.white;
          final edge = Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = 1 / scale
            ..color = colors.primary;
          if (h.key == _Handle.rotate) {
            canvas.drawCircle(h.value, 5 / scale, fill);
            canvas.drawCircle(h.value, 5 / scale, edge);
          } else {
            final r = Rect.fromCenter(center: h.value, width: 8 / scale, height: 8 / scale);
            canvas.drawRect(r, fill);
            canvas.drawRect(r, edge);
          }
        }
      }
    }
    if (state._moved) {
      final ghost = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1 / scale
        ..color = colors.primary.withValues(alpha: 0.8);
      final edge = state._edge;
      final table = edge == null ? null : painter.tableOf(edge.$1);
      if (edge != null && table != null) {
        canvas.save();
        canvas.transform(painter.transformOf(edge.$1).storage);
        final at = (edge.$2 == _Drag.column ? table.columns : table.rows)[edge.$3] + state._edgeShift;
        final line = edge.$2 == _Drag.column ? (Offset(at, 0), Offset(at, table.size.height)) : (Offset(0, at), Offset(table.size.width, at));
        _dashed(canvas, Rect.fromPoints(line.$1, line.$2), colors.primary, 1 / scale);
        canvas.restore();
      } else if (state._drag == _Drag.rotate && state._single != null) {
        final shape = state._single!;
        final box = deck.styleOf(shape).bounds!;
        canvas.save();
        canvas.transform(state._space(shape).storage);
        canvas.translate(box.center.dx, box.center.dy);
        canvas.rotate(state._rotation(shape) * math.pi / 180);
        canvas.drawRect(Rect.fromCenter(center: Offset.zero, width: box.width, height: box.height), ghost);
        canvas.restore();
      } else {
        for (final MapEntry(key: shape, value: r) in state._previews().entries) {
          canvas.save();
          canvas.transform(state._space(shape).storage);
          canvas.drawRect(r, ghost);
          canvas.restore();
        }
      }
    }
    for (final t in w.comments) {
      final box = state._balloon(t);
      final color = authorColor(t.root.author);
      final active = t.id == w.activeComment;
      final shape = RRect.fromRectAndCorners(box, topLeft: Radius.circular(box.width / 2), topRight: Radius.circular(box.width / 2), bottomRight: Radius.circular(box.width / 2));
      canvas.drawRRect(shape, Paint()..color = t.done ? color.withValues(alpha: 0.5) : color);
      canvas.drawRRect(
        shape,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = (active ? 2.5 : 1.5) / scale
          ..color = active ? Colors.white : const Color(0x66FFFFFF),
      );
      final label = TextPainter(
        text: TextSpan(text: t.root.initials.isEmpty ? '?' : t.root.initials, style: TextStyle(fontSize: 9 / scale, color: Colors.white, fontWeight: FontWeight.w600)),
        textDirection: TextDirection.ltr,
      )..layout();
      label.paint(canvas, box.center - Offset(label.width / 2, label.height / 2));
    }
    canvas.restore();
  }

  /// A caret drawn [width] thick, across the line of the text: upright in
  /// a horizontal text, lying in a vertical one.
  Rect _bar(Rect caret, double width) => caret.width == 0
      ? Rect.fromLTWH(caret.left - width / 2, caret.top, width, caret.height)
      : Rect.fromLTWH(caret.left, caret.top - width / 2, caret.width, width);

  void _dashed(Canvas canvas, Rect r, Color color, double width) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = width;
    final dash = 4 * width;
    for (final (a, b) in [(r.topLeft, r.topRight), (r.topRight, r.bottomRight), (r.bottomRight, r.bottomLeft), (r.bottomLeft, r.topLeft)]) {
      final length = (b - a).distance;
      if (length == 0) continue;
      final dir = (b - a) / length;
      for (var t = 0.0; t < length; t += 2 * dash) {
        canvas.drawLine(a + dir * t, a + dir * math.min(t + dash, length), paint);
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => true;
}
