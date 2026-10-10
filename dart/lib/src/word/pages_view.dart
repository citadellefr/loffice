import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/commands.dart';
import '../chrome/link_card.dart';
import '../chrome/strings.dart';
import '../clipboard.dart';
import 'document.dart';
import 'edits.dart';
import 'layout.dart';
import 'page_painter.dart';
import 'revisions.dart';

/// The pages of a document one under the other, where the text is edited:
/// the caret and selection, those of the others, the keys of Word and the
/// input of the platform.
class WordPagesView extends StatefulWidget {
  const WordPagesView({
    super.key,
    required this.session,
    required this.layout,
    required this.selection,
    required this.painter,
    required this.scale,
    required this.focusNode,
    this.onShortcut,
    this.onContextMenu,
    this.marks = const [],
    this.highlights = const [],
    this.changed = const [],
    this.trackAs,
    this.commands = const [],
    this.onOpenLink,
    this.linkCard,
    this.strings = const LofficeStrings(),
  });

  final DocSession session;
  final WordLayout layout;
  final WordSelection selection;
  final PagePainter painter;

  /// Logical pixels by point.
  final double scale;
  final FocusNode focusNode;

  /// The keys of the editor, tried first.
  final bool Function(KeyEvent e)? onShortcut;

  /// A right click or long press on the text, at a global position.
  final void Function(Offset global)? onContextMenu;

  /// Ranges of flows drawn in a color under the text: those of comments.
  final List<(String, int, int, Color)> marks;

  /// Ranges of flows shown highlighted: what a search found.
  final List<(String, int, int)> highlights;

  /// Ranges of flows changed, marked by a bar in the margin as Word does.
  final List<(String, int, int)> changed;

  /// The author changes are tracked as, null when they are not tracked.
  final String? trackAs;

  /// The keywords an `@` typed in the text starts.
  final List<Command> commands;

  /// Opens a link of the text: clicked with Ctrl, or alone in a document
  /// only read.
  final void Function(Uri uri)? onOpenLink;

  /// The card of the host over a link pointed at, or tapped on a touch
  /// screen in a document only read: what it leads to. Null shows nothing.
  final LinkCard? linkCard;
  final LofficeStrings strings;

  @override
  State<WordPagesView> createState() => WordPagesViewState();
}

const _gap = 16.0;

class WordPagesViewState extends State<WordPagesView> implements DeltaTextInputClient, CommandField {
  late final _commands = Commands(this, commands: () => widget.commands, strings: () => widget.strings);
  late final _cards = LinkCards(() => context, () => widget.linkCard);
  final _vertical = ScrollController();
  final _horizontal = ScrollController();
  TextInputConnection? _input;
  StreamSubscription<Edit>? _changes;
  void Function()? _unwatchPaste;
  var _local = false;
  Timer? _blink;
  var _caretOn = true;
  double? _goalX;
  DateTime _lastDown = DateTime(0);
  Offset _lastDownAt = Offset.zero;
  var _clicks = 0;
  var _dragging = false;
  var _viewport = Size.zero;

  WordSelection get _selection => widget.selection;
  DocSession get _session => widget.session;
  WordLayout get _layout => widget.layout;
  double get _scale => widget.scale;

  @override
  void initState() {
    super.initState();
    _selection.addListener(_selectionChanged);
    _changes = _session.changes.listen(_documentChanged);
    widget.focusNode.addListener(_focusChanged);
    _unwatchPaste = watchPaste(_pasted);
  }

  @override
  void didUpdateWidget(WordPagesView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selection != widget.selection) {
      oldWidget.selection.removeListener(_selectionChanged);
      widget.selection.addListener(_selectionChanged);
    }
    if (oldWidget.focusNode != widget.focusNode) {
      oldWidget.focusNode.removeListener(_focusChanged);
      widget.focusNode.addListener(_focusChanged);
    }
  }

  @override
  void dispose() {
    _selection.removeListener(_selectionChanged);
    widget.focusNode.removeListener(_focusChanged);
    _unwatchPaste?.call();
    unawaited(_changes?.cancel());
    _commands.dispose();
    _cards.hide();
    _blink?.cancel();
    _input?.close();
    _vertical.dispose();
    _horizontal.dispose();
    super.dispose();
  }

  void _focusChanged() {
    if (widget.focusNode.hasFocus && _selection.flow != null) {
      _openInput();
    } else if (!widget.focusNode.hasFocus) {
      _input?.close();
      _input = null;
      _commands.close();
    }
    if (mounted) setState(() {});
  }

  void _selectionChanged() {
    final flow = _selection.flow;
    if (flow == null) {
      _session.select(null);
    } else {
      _session.select(DocSelection(flow, _selection.base, _selection.extent));
      if (widget.focusNode.hasFocus) _input?.setEditingState(currentTextEditingValue);
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

  /// Follows the edits of others in the flow being edited.
  void _documentChanged(Edit edit) {
    final flow = _selection.flow;
    if (flow == null || _local) return;
    if (_session.document[flow] == null) {
      _selection.clear();
      return;
    }
    var base = _selection.base, extent = _selection.extent;
    for (final c in edit.changes) {
      if (c.kind == ChangeKind.text && c.id == flow) {
        base = c.text!.transformPosition(base, thisFirst: false);
        extent = c.text!.transformPosition(extent, thisFirst: false);
      }
    }
    _selection.set(flow, base, extent);
  }

  Node? get _flow {
    final id = _selection.flow;
    return id == null ? null : _session.document[id];
  }

  // keywords

  @override
  ({String text, int caret})? get typing {
    final flow = _flow;
    final s = _selection;
    if (flow == null || !s.collapsed || _session.readOnly || !widget.focusNode.hasFocus) return null;
    final editing = wordEditing(flow);
    final (start, end) = editing.paragraphAt(s.extent);
    return (text: editing.text.substring(start, end), caret: s.extent - start);
  }

  @override
  ({String before, String after}) get around {
    final flow = _flow;
    if (flow == null) return (before: '', after: '');
    final (start, end) = wordEditing(flow).paragraphAt(_selection.extent);
    final before = StringBuffer(), after = StringBuffer();
    var passed = false;
    for (final f in _flows) {
      final text = f.text!.text;
      if (f.id != flow.id) {
        (passed ? after : before).write(text.replaceAll('￼', ''));
        continue;
      }
      passed = true;
      before.write(text.substring(0, start).replaceAll('￼', ''));
      after.write(text.substring(end).replaceAll('￼', ''));
    }
    return (before: before.toString(), after: after.toString());
  }

  @override
  Rect? rectAt(int offset) {
    final flow = _flow;
    final box = context.findRenderObject();
    if (flow == null || box is! RenderBox || !_vertical.hasClients) return null;
    final (start, _) = wordEditing(flow).paragraphAt(_selection.extent);
    final at = _layout.caret(flow.id, start + offset, page: _selection.page);
    final (tops, _) = _tops;
    if (at == null || at.$1 >= tops.length) return null;
    final (page, rect) = at;
    final width = math.max(box.size.width, _width);
    final origin = Offset(
      (width - _layout.pages[page].size.width * _scale) / 2 - (_horizontal.hasClients ? _horizontal.offset : 0),
      tops[page] - _vertical.offset,
    );
    return Rect.fromPoints(
      box.localToGlobal(origin + rect.topLeft * _scale),
      box.localToGlobal(origin + rect.bottomRight * _scale),
    );
  }

  @override
  void write(int start, int end, String text, {Uri? link}) {
    final flow = _flow;
    if (flow == null) return;
    final editing = wordEditing(flow);
    final (paragraph, _) = editing.paragraphAt(_selection.extent);
    start += paragraph;
    end += paragraph;
    if (link == null) {
      replace(flow, start, end, text);
      return;
    }
    final attributes = _selection.typing ?? editing.typingAttributes(start);
    final d = Delta()..retain(start);
    if (end > start) d.delete(end - start);
    d
      ..insert(text, {...attributes, ...linkAttributes(link)})
      ..insert(' ', attributes.isEmpty ? null : attributes);
    final caret = start + text.length + 1;
    _text(flow, d.chop(), caret, caret);
  }

  /// The link under a click that opens it, if any.
  Uri? _linkAt(String flow, int offset) =>
      widget.onOpenLink == null || !(_ctrl || _session.readOnly) ? null : _linkOf(flow, offset);

  Uri? _linkOf(String flow, int offset) {
    final node = _session.document[flow];
    final link = node == null ? null : wordEditing(node).attributesAt(offset)?['link'];
    final uri = link == null ? null : Uri.tryParse(link);
    return uri != null && uri.hasScheme ? uri : null;
  }

  /// Tells the card which link the pointer is over, on a page.
  void _hover(int page, PointerHoverEvent e) {
    final p = e.localPosition / _scale;
    final hit = _layout.hit(page, p, area: _selection.area);
    Uri? uri;
    if (hit case (final flow, final offset)) {
      // the nearest character is not always under the pointer: a margin, the
      // end of a line
      for (final o in [offset, offset - 1]) {
        if (o < 0 || !_layout.selection(flow, o, o + 1).any((r) => r.$1 == page && r.$2.contains(p))) continue;
        uri = _linkOf(flow, o);
        break;
      }
    }
    _cards.point(uri, e.position);
  }

  // geometry

  /// The top of each page in the scrolled column, and the column's height.
  (List<double>, double) get _tops {
    final tops = <double>[];
    var y = _gap;
    for (final p in _layout.pages) {
      tops.add(y);
      y += p.size.height * _scale + _gap;
    }
    return (tops, y);
  }

  double get _width => _layout.pages.fold(0.0, (w, p) => math.max(w, p.size.width * _scale)) + 2 * _gap;

  /// Brings the caret into view.
  void reveal() => _reveal();

  void _reveal() {
    final flow = _selection.flow;
    if (flow == null) return;
    final at = _layout.caret(flow, _selection.extent, page: _selection.page);
    if (at == null || !_vertical.hasClients) return;
    final (page, rect) = at;
    final (tops, _) = _tops;
    if (page >= tops.length) return;
    final top = tops[page] + rect.top * _scale, bottom = tops[page] + rect.bottom * _scale;
    final view = _vertical.position;
    const margin = 40.0;
    if (top < view.pixels + margin) {
      _vertical.jumpTo(math.max(top - margin, 0));
    } else if (bottom > view.pixels + view.viewportDimension - margin) {
      _vertical.jumpTo(math.min(bottom - view.viewportDimension + margin, view.maxScrollExtent));
    }
  }

  // pointer

  void _down(int page, PointerDownEvent e, Offset local) {
    if (_commands.answering) return;
    _cards.hide();
    widget.focusNode.requestFocus();
    if (e.buttons == kSecondaryMouseButton) {
      widget.onContextMenu?.call(e.position);
      return;
    }
    final now = DateTime.now();
    final again = now.difference(_lastDown) < kDoubleTapTimeout && (e.position - _lastDownAt).distance < kDoubleTapSlop;
    _clicks = again ? _clicks + 1 : 1;
    _lastDown = now;
    _lastDownAt = e.position;
    _goalX = null;
    final p = local / _scale;
    final pageLayout = _layout.pages[page];
    // a double click in the margin of a header or footer edits it, a double
    // click in the body leaves it
    var area = _selection.area;
    if (_clicks == 2) {
      final inHeader = p.dy < pageLayout.body.top && pageLayout.header != null;
      final inFooter = p.dy > pageLayout.body.bottom && pageLayout.footer != null;
      area = inHeader ? PageArea.header : (inFooter ? PageArea.footer : PageArea.body);
      if (area != _selection.area) _clicks = 1;
    }
    final hit = _layout.hit(page, p, area: area);
    if (hit == null) return;
    final (flow, offset) = hit;
    final node = _session.document[flow];
    if (node == null) return;
    if (_linkAt(flow, offset) case final uri?) {
      if (_ctrl || e.kind == PointerDeviceKind.mouse || !_cards.tap(uri, e.position)) widget.onOpenLink!(uri);
      return;
    }
    final editing = wordEditing(node);
    if (_clicks == 2) {
      final (s, t) = editing.wordAt(offset);
      _selection.set(flow, s, t, page, area);
    } else if (_clicks >= 3) {
      final (s, t) = editing.paragraphAt(offset);
      _selection.set(flow, s, t + 1 > node.text!.length - 1 ? t : t + 1, page, area);
    } else if (HardwareKeyboard.instance.isShiftPressed && _selection.flow != null && area == _selection.area) {
      _selection.extendTo(flow, offset, page);
    } else {
      _selection.set(flow, offset, null, page, area);
    }
    _dragging = true;
  }

  void _move(int page, Offset local) {
    if (!_dragging) return;
    final flow = _selection.flow;
    if (flow == null) return;
    final hit = _layout.hit(page, local / _scale, area: _selection.area);
    if (hit == null) return;
    _selection.extendTo(hit.$1, hit.$2, page);
  }

  // keys

  bool get _ctrl => HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed;
  bool get _shift => HardwareKeyboard.instance.isShiftPressed;

  KeyEventResult _key(FocusNode node, KeyEvent e) {
    if (_commands.key(e)) return KeyEventResult.handled;
    if (e is KeyUpEvent) return KeyEventResult.ignored;
    if (widget.onShortcut?.call(e) ?? false) return KeyEventResult.handled;
    final flow = _flow;
    if (flow == null) return KeyEventResult.ignored;
    return _textKey(flow, e) ? KeyEventResult.handled : KeyEventResult.ignored;
  }

  /// The flows the caret moves through with the arrows: those of the body,
  /// or of the header or footer being edited.
  List<Node> get _flows {
    if (_selection.area == PageArea.body) return flowsOf(_session.document);
    final root = headerNodeOf(_session.document, _selection.flow ?? '');
    return root == null ? const [] : flowsOf(_session.document, root);
  }

  bool _textKey(Node flow, KeyEvent e) {
    final key = e.logicalKey;
    final editing = wordEditing(flow);
    final last = flow.text!.length - 1;
    final s = _selection;
    void caret(String id, int to, {bool keepGoal = false}) {
      if (!keepGoal) _goalX = null;
      if (_shift) {
        s.extendTo(id, to);
      } else {
        s.set(id, to);
      }
      _reveal();
    }

    void across(int by) {
      final flows = _flows;
      final i = flows.indexWhere((f) => f.id == flow.id);
      final j = i + by;
      if (i < 0 || j < 0 || j >= flows.length) return;
      final next = flows[j];
      caret(next.id, by > 0 ? 0 : next.text!.length - 1);
    }

    switch (key) {
      case LogicalKeyboardKey.escape:
        if (s.area != PageArea.body) {
          final body = flowsOf(_session.document).firstOrNull;
          if (body != null) s.set(body.id, 0, null, null, PageArea.body);
        }
        return true;
      case LogicalKeyboardKey.arrowLeft:
        if (!_shift && !s.collapsed) {
          caret(flow.id, s.start);
        } else if (s.extent == 0) {
          across(-1);
        } else {
          caret(flow.id, editing.previous(s.extent, word: _ctrl));
        }
        return true;
      case LogicalKeyboardKey.arrowRight:
        if (!_shift && !s.collapsed) {
          caret(flow.id, s.end);
        } else if (s.extent >= last) {
          across(1);
        } else {
          caret(flow.id, editing.next(s.extent, word: _ctrl));
        }
        return true;
      case LogicalKeyboardKey.arrowUp || LogicalKeyboardKey.arrowDown:
        _moveLines(flow, key == LogicalKeyboardKey.arrowUp ? -1 : 1, caret);
        return true;
      case LogicalKeyboardKey.pageUp || LogicalKeyboardKey.pageDown:
        final lines = (_viewport.height / _scale / 14).floor();
        _moveLines(flow, (key == LogicalKeyboardKey.pageUp ? -1 : 1) * math.max(lines, 1), caret);
        return true;
      case LogicalKeyboardKey.home:
        if (_ctrl) {
          final first = _flows.firstOrNull;
          if (first != null) caret(first.id, 0);
        } else {
          caret(flow.id, _layout.lineRange(flow.id, s.extent)?.$1 ?? editing.paragraphAt(s.extent).$1);
        }
        return true;
      case LogicalKeyboardKey.end:
        if (_ctrl) {
          final lastFlow = _flows.lastOrNull;
          if (lastFlow != null) caret(lastFlow.id, lastFlow.text!.length - 1);
        } else {
          caret(flow.id, _layout.lineRange(flow.id, s.extent)?.$2 ?? editing.paragraphAt(s.extent).$2);
        }
        return true;
    }
    if (_session.readOnly) {
      if (_ctrl && (key == LogicalKeyboardKey.keyC || key == LogicalKeyboardKey.keyA)) return clipboard(flow, key);
      return false;
    }
    if (s.spans && (key == LogicalKeyboardKey.backspace || key == LogicalKeyboardKey.delete)) {
      _deleteSpan();
      return true;
    }
    if (_ctrl) {
      // left to the browser, which then tells the page to paste
      if (key == LogicalKeyboardKey.keyV && _unwatchPaste != null) return false;
      switch (key) {
        case LogicalKeyboardKey.keyA || LogicalKeyboardKey.keyC || LogicalKeyboardKey.keyX || LogicalKeyboardKey.keyV:
          return clipboard(flow, key);
        case LogicalKeyboardKey.backspace:
          final from = editing.previous(s.start, word: true);
          _delete(flow, editing.deleteBackward(from, s.collapsed ? s.start : s.end), from, back: true);
          return true;
        case LogicalKeyboardKey.delete:
          _delete(flow, editing.deleteForward(s.start, s.collapsed ? editing.next(s.start, word: true) : s.end), s.start);
          return true;
        case LogicalKeyboardKey.enter || LogicalKeyboardKey.numpadEnter:
          replaceWith(flow, s.start, s.end, '￼', {'br': 'page'});
          return true;
      }
      return false;
    }
    switch (key) {
      case LogicalKeyboardKey.backspace:
        _delete(flow, editing.deleteBackward(s.start, s.end), s.collapsed ? editing.previous(s.start) : s.start, back: true);
        return true;
      case LogicalKeyboardKey.delete:
        _delete(flow, editing.deleteForward(s.start, s.end), s.start);
        return true;
      case LogicalKeyboardKey.enter || LogicalKeyboardKey.numpadEnter:
        replace(flow, s.start, s.end, _shift ? '\v' : '\n');
        return true;
      case LogicalKeyboardKey.tab:
        return _tab(flow);
    }
    return false;
  }

  /// Tab: the next cell of a table, a level of a list at the start of a
  /// list paragraph, or a tab.
  bool _tab(Node flow) {
    final s = _selection;
    final cell = _session.document[flow.parent];
    if (cell != null && cell.type == 'tc') {
      final flows = _flows;
      final i = flows.indexWhere((f) => f.id == flow.id);
      final step = _shift ? -1 : 1;
      for (var j = i + step; j >= 0 && j < flows.length; j += step) {
        if (flows[j].parent != flow.parent && _session.document[flows[j].parent]?.type == 'tc') {
          s.set(flows[j].id, 0, flows[j].text!.length - 1);
          _reveal();
          return true;
        }
      }
      return true;
    }
    final mark = wordEditing(flow).markAt(s.start);
    final (start, _) = wordEditing(flow).paragraphAt(s.start);
    if (mark['num'] != null && mark['num'] != '0' && (s.start == start || !s.collapsed)) {
      final lvl = (int.tryParse(mark['lvl'] ?? '') ?? 0) + (_shift ? -1 : 1);
      _text(flow, wordEditing(flow).formatParagraphs(s.start, s.end, {'lvl': '${lvl.clamp(0, 8)}'}), s.base, s.extent);
      return true;
    }
    replace(flow, s.start, s.end, '\t');
    return true;
  }

  void _moveLines(Node flow, int lines, void Function(String id, int to, {bool keepGoal}) caret) {
    final s = _selection;
    final at = _layout.caret(flow.id, s.extent, page: s.page);
    if (at == null) return;
    var (page, rect) = at;
    _goalX ??= rect.left;
    var y = rect.center.dy;
    for (var n = 0; n < lines.abs(); n++) {
      y += lines.sign * math.max(rect.height, 8);
      final p = _layout.pages[page];
      if (y < p.body.top && lines < 0 && s.area == PageArea.body && page > 0) {
        page--;
        y = _layout.pages[page].body.bottom - 1;
      } else if (y > p.body.bottom && lines > 0 && s.area == PageArea.body && page + 1 < _layout.pages.length) {
        page++;
        y = _layout.pages[page].body.top + 1;
      }
    }
    final hit = _layout.hit(page, Offset(_goalX!, y), area: s.area);
    if (hit == null) return;
    s.page = page;
    caret(hit.$1, hit.$2, keepGoal: true);
  }

  /// Selects all, copies, cuts or pastes, as the key with Ctrl does.
  bool clipboard(Node flow, LogicalKeyboardKey key) {
    final s = _selection;
    final editing = wordEditing(flow);
    switch (key) {
      case LogicalKeyboardKey.keyA:
        final flows = _flows;
        if (flows.isEmpty) return true;
        s.set(flows.first.id, 0);
        s.extendTo(flows.last.id, flows.last.text!.length - 1);
      case LogicalKeyboardKey.keyC || LogicalKeyboardKey.keyX when s.spans:
        _copied = null;
        final text = [
          for (final (f, a, b) in s.ranges(_session.document, _flows)) wordEditing(f).text.substring(a, b),
        ].join('\n');
        unawaited(Clipboard.setData(ClipboardData(text: text.replaceAll('\v', '\n').replaceAll('￼', ''))));
        if (key == LogicalKeyboardKey.keyX && !_session.readOnly) _deleteSpan();
      case LogicalKeyboardKey.keyC || LogicalKeyboardKey.keyX:
        if (!s.collapsed) {
          _copied = _sliceOf(flow.text!, s.start, s.end);
          unawaited(Clipboard.setData(ClipboardData(text: editing.text.substring(s.start, s.end).replaceAll('\v', '\n').replaceAll('￼', ''))));
          if (key == LogicalKeyboardKey.keyX) replace(flow, s.start, s.end, '');
        }
      case LogicalKeyboardKey.keyV:
        unawaited(Clipboard.getData(Clipboard.kTextPlain).then((data) {
          final text = data?.text;
          if (text != null) _pasteText(text);
        }));
    }
    return true;
  }

  void _pasteText(String text) {
    final current = _flow;
    if (current == null) return;
    final own = _copied;
    final plain = own?.text.replaceAll('\v', '\n').replaceAll('￼', '');
    if (own != null && plain == text.replaceAll('\r\n', '\n')) {
      _pasteRich(current, own);
    } else {
      replace(current, _selection.start, _selection.end, text.replaceAll('\r\n', '\n').replaceAll('\r', '\n'));
    }
  }

  bool _pasted(String text) {
    if (_session.readOnly || !widget.focusNode.hasFocus || _flow == null) return false;
    _pasteText(text);
    return true;
  }

  Delta _sliceOf(Delta flow, int start, int end) {
    final out = Delta();
    var at = 0;
    for (final op in flow.ops) {
      final s = op.insert!;
      final a = (start - at).clamp(0, s.length), b = (end - at).clamp(0, s.length);
      // the anchors of comments stay with their comment
      final anchor = op.attributes?.keys.any(const {'cs', 'ce', 'comment'}.contains) ?? false;
      if (b > a && !anchor) {
        final attrs = op.attributes == null ? null : {for (final e in op.attributes!.entries) if (e.key != 'sect' && e.key != 'sx') e.key: e.value};
        out.insert(s.substring(a, b), attrs == null || attrs.isEmpty ? null : attrs);
      }
      at += s.length;
    }
    return out;
  }

  void _pasteRich(Node flow, Delta content) {
    if (_selection.spans) flow = _deleteSpan() ?? flow;
    final s = _selection;
    final d = Delta()..retain(s.start);
    if (s.end > s.start) d.delete(s.end - s.start);
    for (final op in content.ops) {
      d.insert(op.insert!, op.attributes);
    }
    final caret = s.start + content.length;
    _text(flow, d.chop(), caret, caret);
  }

  /// Deletes what a selection across flows covers, the caret left where it
  /// started; the flow of the caret, null when there is none.
  Node? _deleteSpan() {
    final s = _selection;
    final ranges = s.ranges(_session.document, _flows);
    if (ranges.isEmpty) return null;
    final (first, at, _) = ranges.first;
    final author = widget.trackAs;
    _local = true;
    _session.edit(author == null ? deleteRanges(_session.document, ranges) : trackDeletion(ranges, author, revisionDate(DateTime.now())));
    _local = false;
    final node = _session.document[first.id];
    if (node != null) s.set(node.id, at.clamp(0, node.text!.length - 1));
    _input?.setEditingState(currentTextEditingValue);
    return node;
  }

  /// Replaces a range of a flow by what was typed, or the selection across
  /// flows.
  void replace(Node flow, int start, int end, String text) {
    if (_selection.spans) {
      final node = _deleteSpan();
      if (node == null || text.isEmpty) return;
      (flow, start, end) = (node, _selection.start, _selection.start);
    }
    final editing = wordEditing(flow);
    final attributes = _selection.typing ?? editing.typingAttributes(start);
    final d = text.isEmpty ? editing.delete(start, end) : editing.replace(start, end, text, attributes);
    final caret = start + text.length;
    _text(flow, d, caret, caret);
  }

  /// Replaces a range by an object the writer makes: a page break.
  void replaceWith(Node flow, int start, int end, String char, Attributes attributes) {
    if (_selection.spans) {
      final node = _deleteSpan();
      if (node == null) return;
      (flow, start, end) = (node, _selection.start, _selection.start);
    }
    final d = Delta()..retain(start);
    if (end > start) d.delete(end - start);
    d.insert(char, attributes);
    _text(flow, d.chop(), start + 1, start + 1);
  }

  /// Deletes, the caret put at [caret]: before the text struck there when
  /// changes are tracked and the deletion goes [back], after it otherwise.
  void _delete(Node flow, Delta? d, int caret, {bool back = false}) {
    if (d == null) return;
    _text(flow, d, caret, caret, after: !back);
  }

  /// Makes a change of a flow, then puts the selection at [base] and
  /// [extent], offsets of the flow changed; tracked, what it deletes stays
  /// and they move over it, [after] it or not.
  void _text(Node flow, Delta d, int base, int extent, {bool after = true}) {
    final author = widget.trackAs;
    if (author != null) {
      final tracked = track(flow.text!, d, author, revisionDate(DateTime.now()));
      (d, base, extent) = (tracked.delta, tracked.offset(base, after: after), tracked.offset(extent, after: after));
    }
    _local = true;
    final ok = d.isEmpty || _session.edit(Edit([Change.text(flow.id, d)]));
    _local = false;
    final node = _session.document[flow.id];
    if (node == null) return;
    final last = node.text!.length - 1;
    if (ok) _selection.set(flow.id, base.clamp(0, last), extent.clamp(0, last));
    _input?.setEditingState(currentTextEditingValue);
    _commands.follow();
    _restartBlink();
    WidgetsBinding.instance.addPostFrameCallback((_) => _reveal());
  }

  // text input

  void _openInput() {
    if (_session.readOnly) return;
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
    final node = _flow;
    if (node == null) return TextEditingValue.empty;
    final text = node.text!.text;
    final s = _selection;
    return TextEditingValue(
      text: text.substring(0, text.length - 1),
      selection: s.spans ? TextSelection.collapsed(offset: s.extent) : TextSelection(baseOffset: s.base, extentOffset: s.extent),
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
      final node = _flow;
      if (node == null) return;
      switch (delta) {
        case TextEditingDeltaInsertion(textInserted: '\n') when _commands.take():
          _input?.setEditingState(currentTextEditingValue);
          continue;
        case TextEditingDeltaInsertion(:final insertionOffset, :final textInserted):
          replace(node, insertionOffset, insertionOffset, textInserted);
        case TextEditingDeltaDeletion(:final deletedRange):
          replace(node, deletedRange.start, deletedRange.end, '');
        case TextEditingDeltaReplacement(:final replacedRange, :final replacementText):
          replace(node, replacedRange.start, replacedRange.end, replacementText);
        case TextEditingDeltaNonTextUpdate():
      }
      // tracked, the text struck stays: the platform's selection is off
      final selection = delta.selection;
      if (selection.isValid && (widget.trackAs == null || delta is TextEditingDeltaNonTextUpdate)) {
        final last = (_flow?.text?.length ?? 1) - 1;
        _selection.set(node.id, selection.baseOffset.clamp(0, last), selection.extentOffset.clamp(0, last));
      }
    }
  }

  /// The whole value, from platforms that do not send deltas: what changed
  /// is found by comparing it with the text.
  @override
  void updateEditingValue(TextEditingValue value) {
    final node = _flow;
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
      replace(node, prefix, old.length - suffix, now.substring(prefix, now.length - suffix));
    }
    final last = (_flow?.text?.length ?? 1) - 1;
    if (value.selection.isValid && (widget.trackAs == null || old == now)) {
      _selection.set(node.id, value.selection.baseOffset.clamp(0, last), value.selection.extentOffset.clamp(0, last));
    }
  }

  @override
  void performAction(TextInputAction action) {
    final node = _flow;
    if (node == null || action != TextInputAction.newline || _held || _commands.take()) return;
    replace(node, _selection.start, _selection.end, '\n');
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
    final theme = Theme.of(context);
    final pages = _layout.pages;
    return LayoutBuilder(builder: (context, box) {
      _viewport = box.biggest;
      final width = math.max(box.maxWidth, _width);
      final pagesView = Focus(
        focusNode: widget.focusNode,
        onKeyEvent: _key,
        child: MouseRegion(
          cursor: SystemMouseCursors.text,
          onExit: (_) => _cards.hide(),
          child: Scrollbar(
            controller: _horizontal,
            child: SingleChildScrollView(
              controller: _horizontal,
              scrollDirection: Axis.horizontal,
              child: SizedBox(
                width: width,
                height: box.maxHeight,
                child: Scrollbar(
                  controller: _vertical,
                  child: ListView.builder(
                    controller: _vertical,
                    padding: const EdgeInsets.only(top: _gap),
                    itemCount: pages.length,
                    itemExtentBuilder: (i, _) => i < pages.length ? pages[i].size.height * _scale + _gap : 0,
                    itemBuilder: (context, i) {
                      final page = pages[i];
                      final size = page.size * _scale;
                      return Align(
                        alignment: Alignment.topCenter,
                        child: Listener(
                          onPointerDown: (e) => _down(i, e, e.localPosition),
                          onPointerHover: (e) => _hover(i, e),
                          onPointerMove: (e) => _move(i, e.localPosition),
                          onPointerUp: (_) => _dragging = false,
                          child: Container(
                            decoration: BoxDecoration(boxShadow: [BoxShadow(color: theme.shadowColor.withValues(alpha: 0.25), blurRadius: 4, offset: const Offset(0, 1))]),
                            child: CustomPaint(size: size, painter: _PagePainter(this, i, theme.colorScheme)),
                          ),
                        ),
                      );
                    },
                  ),
                ),
              ),
            ),
          ),
        ),
      );
      return CommandsMenu(commands: _commands, child: pagesView);
    });
  }
}

/// Draws a page and, over it, the selections and carets on it.
class _PagePainter extends CustomPainter {
  _PagePainter(this.state, this.index, this.colors);

  final WordPagesViewState state;
  final int index;
  final ColorScheme colors;

  @override
  void paint(Canvas canvas, Size size) {
    final layout = state._layout;
    if (index >= layout.pages.length) return;
    final page = layout.pages[index];
    final s = state._selection;
    canvas.save();
    canvas.scale(state._scale);
    state.widget.painter.paint(canvas, page, dimHeaders: s.area == PageArea.body);
    if (s.area != PageArea.body) {
      final line = Paint()
        ..color = colors.primary
        ..strokeWidth = 0.75;
      final y = s.area == PageArea.header ? page.body.top : page.body.bottom;
      canvas.drawLine(Offset(0, y), Offset(page.size.width, y), line);
    }
    for (final (flow, a, b, color) in state.widget.marks) {
      for (final (p, r) in layout.selection(flow, a, b)) {
        if (p == index) canvas.drawRect(r, Paint()..color = color);
      }
    }
    final bar = Paint()
      ..color = const Color(0xFF7F7F7F)
      ..strokeWidth = 1;
    for (final (flow, a, b) in state.widget.changed) {
      for (final (p, r) in layout.selection(flow, a, b)) {
        final x = page.body.left - 14;
        if (p == index) canvas.drawLine(Offset(x, r.top), Offset(x, r.bottom), bar);
      }
    }
    for (final (flow, a, b) in state.widget.highlights) {
      for (final (p, r) in layout.selection(flow, a, b)) {
        if (p == index) canvas.drawRect(r, Paint()..color = const Color(0x66FFD600));
      }
    }
    for (final peer in state._session.peers) {
      final sel = peer.selection;
      if (sel == null) continue;
      final color = _peerColor(peer.sid);
      for (final (p, r) in layout.selection(sel.node, sel.start, sel.end)) {
        if (p == index) canvas.drawRect(r, Paint()..color = color.withValues(alpha: 0.25));
      }
      final caret = layout.caret(sel.node, sel.extent);
      if (caret != null && caret.$1 == index) {
        canvas.drawRect(Rect.fromLTWH(caret.$2.left - 0.5, caret.$2.top, 1.5, caret.$2.height), Paint()..color = color);
      }
    }
    final flow = s.flow;
    if (flow != null) {
      if (!s.collapsed) {
        for (final (node, a, b) in s.ranges(state._session.document, state._flows)) {
          for (final (p, r) in layout.selection(node.id, a, b)) {
            if (p == index) canvas.drawRect(r, Paint()..color = colors.primary.withValues(alpha: 0.28));
          }
        }
      }
      final caret = layout.caret(flow, s.extent, page: s.page);
      if (caret != null && caret.$1 == index && s.collapsed && state._caretOn && state.widget.focusNode.hasFocus) {
        canvas.drawRect(Rect.fromLTWH(caret.$2.left, caret.$2.top, 1 / state._scale * 1.5, caret.$2.height), Paint()..color = const Color(0xFF000000));
      }
    }
    canvas.restore();
  }

  @override
  bool shouldRepaint(_PagePainter old) => true;
}

/// What was last copied in a Word editor, with its formatting: pasted as
/// such when the clipboard still holds its text.
Delta? _copied;

const _peerColors = [Color(0xFFE67E22), Color(0xFF8E44AD), Color(0xFF16A085), Color(0xFFC0392B), Color(0xFF2980B9)];

Color _peerColor(int sid) => _peerColors[sid % _peerColors.length];
