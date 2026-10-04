import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import '../text/editing.dart';
import 'deck.dart';
import 'edits.dart';
import 'slide_painter.dart';
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
  });

  final DocSession session;
  final Deck deck;
  final Node slide;
  final SlidePainter painter;
  final SlideSelection selection;
  final FocusNode? focusNode;
  final LofficeStrings strings;

  /// Keys the canvas leaves to the editor around it; true when handled.
  final bool Function(KeyEvent event)? onShortcut;

  @override
  State<SlideCanvas> createState() => SlideCanvasState();
}

enum _Drag { none, move, resize, rotate, text }

/// Handles of a selected shape: the corners, the middles of the sides and
/// the rotation handle above.
enum _Handle { topLeft, top, topRight, right, bottomRight, bottom, bottomLeft, left, rotate }

class SlideCanvasState extends State<SlideCanvas> implements DeltaTextInputClient {
  late FocusNode _focus = widget.focusNode ?? FocusNode();
  TextInputConnection? _input;
  StreamSubscription<Edit>? _changes;
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
  }

  @override
  void didUpdateWidget(SlideCanvas oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selection != widget.selection) {
      oldWidget.selection.removeListener(_selectionChanged);
      widget.selection.addListener(_selectionChanged);
    }
    if (oldWidget.focusNode != widget.focusNode) {
      if (oldWidget.focusNode == null) _focus.dispose();
      _focus = widget.focusNode ?? FocusNode();
    }
    if (oldWidget.slide.id != widget.slide.id && _selection.shapes.isNotEmpty) _selection.clear();
  }

  @override
  void dispose() {
    _selection.removeListener(_selectionChanged);
    unawaited(_changes?.cancel());
    _blink?.cancel();
    _input?.close();
    if (widget.focusNode == null) _focus.dispose();
    super.dispose();
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

  /// The shape at a point of the slide, the topmost; a shape of a group
  /// gives the group.
  Node? _shapeAt(Offset p) {
    final shapes = _deck.shapesOf(widget.slide).reversed;
    for (final s in shapes) {
      if (_contains(s, p)) return s;
    }
    return null;
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

  void _down(PointerDownEvent e) {
    _focus.requestFocus();
    final p = _toSlide(e.localPosition);
    final now = DateTime.now();
    final again = now.difference(_lastDown) < kDoubleTapTimeout && (e.localPosition - _lastDownAt).distance < kDoubleTapSlop;
    _clicks = again ? _clicks + 1 : 1;
    _lastDown = now;
    _lastDownAt = e.localPosition;
    _dragFrom = _dragTo = p;
    _moved = false;
    _goalX = null;
    final shift = HardwareKeyboard.instance.isShiftPressed;

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
      _drag = _Drag.text;
      return;
    }

    final single = _single;
    if (single != null && single.parent == widget.slide.id && !_session.readOnly) {
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
    final cell = shift || _session.readOnly ? null : _cellAt(hit, p);
    if (cell != null) {
      _startEditing(cell, at: p, word: _clicks >= 2);
      _drag = _Drag.text;
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
      case _Drag.move || _Drag.resize || _Drag.rotate:
        setState(() {});
      case _Drag.none:
    }
  }

  void _up(PointerUpEvent e) {
    final drag = _drag;
    _drag = _Drag.none;
    if (!_moved) {
      // a click on a shape already selected goes into its text
      final pressed = _pressed;
      if (drag == _Drag.move && pressed != null && _selection.shapes.length == 1 && pressed.type == 'sp' && _clicks == 1 && _wasSelected) {
        _startEditing(pressed, at: _dragFrom);
      }
      _wasSelected = _selection.shapes.contains(_pressed?.id);
      setState(() {});
      return;
    }
    final edit = switch (drag) {
      _Drag.move => DeckEdits(_deck).place(_previews()),
      _Drag.resize => DeckEdits(_deck).place(_previews()),
      _Drag.rotate => _single == null ? Edit() : DeckEdits(_deck).rotate(_single!, _rotation(_single!)),
      _ => Edit(),
    };
    if (!edit.isEmpty) _session.edit(edit);
    _wasSelected = true;
    setState(() {});
  }

  var _wasSelected = false;

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

  /// Where the shapes being dragged would go, on the slide.
  Map<Node, Rect> _previews() {
    final out = <Node, Rect>{};
    final delta = _dragTo - _dragFrom;
    if (_drag == _Drag.move) {
      for (final id in _selection.shapes) {
        final n = _session.document[id];
        final box = n == null || n.parent != widget.slide.id ? null : _deck.styleOf(n).bounds;
        if (box != null) out[n!] = box.shift(delta);
      }
    } else if (_drag == _Drag.resize) {
      final n = _single;
      final box = n == null ? null : _deck.styleOf(n).bounds;
      if (n != null && box != null) out[n] = _resized(n, box);
    }
    return out;
  }

  /// The bounds of a shape resized by the handle dragged: the opposite side
  /// stays where it is, rotated or not.
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
    final center = t.apply(local.center);
    return Rect.fromCenter(center: center, width: math.max(local.width, 1), height: math.max(local.height, 1));
  }

  double _rotation(Node shape) {
    final box = _deck.styleOf(shape).bounds!;
    final c = box.center;
    var degrees = math.atan2(_dragTo.dy - c.dy, _dragTo.dx - c.dx) * 180 / math.pi + 90;
    if (HardwareKeyboard.instance.isShiftPressed) degrees = (degrees / 15).round() * 15.0;
    return (degrees % 360 + 360) % 360;
  }

  // keys

  KeyEventResult _key(FocusNode node, KeyEvent e) {
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
          if (s.parent == widget.slide.id)
            if (_deck.styleOf(s).bounds case final b?) s: b.shift(nudge * step),
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
        _goalX ??= frame.caretAt(s.extent).left;
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
          unawaited(Clipboard.getData(Clipboard.kTextPlain).then((data) {
            final text = data?.text;
            final current = _editingNode;
            if (text != null && current != null) _replace(current, _selection.start, _selection.end, text.replaceAll('\r\n', '\n'));
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

  @override
  void updateEditingValueWithDeltas(List<TextEditingDelta> deltas) {
    for (final delta in deltas) {
      final node = _editingNode;
      if (node == null) return;
      switch (delta) {
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
    if (node == null) return;
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
    if (node != null && action == TextInputAction.newline) _replace(node, _selection.start, _selection.end, '\n');
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
      return Focus(
        focusNode: _focus,
        onKeyEvent: _key,
        child: Listener(
          onPointerDown: _down,
          onPointerMove: _move,
          onPointerUp: _up,
          child: MouseRegion(
            cursor: _selection.editing != null ? SystemMouseCursors.text : SystemMouseCursors.basic,
            child: CustomPaint(
              size: Size(constraints.maxWidth, constraints.maxHeight),
              painter: _CanvasPainter(this, theme.colorScheme),
            ),
          ),
        ),
      );
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
      canvas.drawRect(Rect.fromLTWH(caret.left - 0.75 / scale, caret.top, 1.5 / scale, caret.height), Paint()..color = color);
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
          canvas.drawRect(Rect.fromLTWH(c.left - 0.5 / scale, c.top, 1.2 / scale, c.height), Paint()..color = Colors.black);
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
      if (selection.shapes.length == 1 && selection.editing == null && shape.parent == w.slide.id) {
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
      if (state._drag == _Drag.rotate && state._single != null) {
        final shape = state._single!;
        final box = deck.styleOf(shape).bounds!;
        canvas.save();
        canvas.translate(box.center.dx, box.center.dy);
        canvas.rotate(state._rotation(shape) * math.pi / 180);
        canvas.drawRect(Rect.fromCenter(center: Offset.zero, width: box.width, height: box.height), ghost);
        canvas.restore();
      } else {
        for (final r in state._previews().values) {
          canvas.drawRect(r, ghost);
        }
      }
    }
    canvas.restore();
  }

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
