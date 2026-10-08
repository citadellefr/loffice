import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:trame/trame.dart';

import 'chrome/commands.dart';
import 'chrome/strings.dart';

/// A plain text editor of the text of [node] in a [DocSession], one line per
/// paragraph, where the selections of others show in the theme's accent.
///
/// Undo and redo are the session's: they revert this person's edits only.
class PlainTextEditor extends StatefulWidget {
  const PlainTextEditor({
    super.key,
    required this.session,
    this.node = 'body',
    this.style,
    this.padding = const EdgeInsets.all(16),
    this.focusNode,
    this.autofocus = false,
    this.commands = const [],
    this.strings = const LofficeStrings(),
  });

  /// The keywords of the host an `@` typed in the text starts. What they
  /// find is written by its name: a text holds no link.
  final List<Command> commands;
  final LofficeStrings strings;

  final DocSession session;
  final String node;
  final TextStyle? style;
  final EdgeInsetsGeometry padding;
  final FocusNode? focusNode;
  final bool autofocus;

  @override
  State<PlainTextEditor> createState() => _PlainTextEditorState();
}

class _PlainTextEditorState extends State<PlainTextEditor> implements CommandField {
  late final _PresenceController _controller;
  late final _commands = Commands(this, commands: () => widget.commands, strings: () => widget.strings);
  final _field = GlobalKey();
  FocusNode? _ownFocus;
  StreamSubscription<Edit>? _changes;
  var _updating = false;
  var _selection = const TextSelection.collapsed(offset: 0);

  DocSession get _session => widget.session;

  FocusNode get _focus => widget.focusNode ?? (_ownFocus ??= FocusNode());

  @override
  void initState() {
    super.initState();
    _controller = _PresenceController(_session, widget.node)..text = _text;
    _controller.addListener(_edited);
    _changes = _session.changes.listen(_changed);
    _session.addListener(_rebuild);
    _commands.addListener(_rebuild);
    _focus.addListener(_focusChanged);
  }

  @override
  void didUpdateWidget(PlainTextEditor old) {
    super.didUpdateWidget(old);
    if (old.focusNode != widget.focusNode) {
      (old.focusNode ?? _ownFocus)?.removeListener(_focusChanged);
      _focus.addListener(_focusChanged);
    }
  }

  @override
  void dispose() {
    unawaited(_changes?.cancel());
    _session.removeListener(_rebuild);
    _session.select(null);
    _focus.removeListener(_focusChanged);
    _ownFocus?.dispose();
    _commands.dispose();
    _controller.dispose();
    super.dispose();
  }

  void _focusChanged() => _focus.hasFocus ? _commands.follow() : _commands.close();

  void _rebuild() {
    if (mounted) setState(() {});
  }

  /// The text of the node, one line per paragraph.
  String get _text {
    final flow = _session.document[widget.node]?.text?.text ?? '\n';
    return flow.substring(0, flow.length - 1);
  }

  /// Turns what the field did into an edit of the document.
  void _edited() {
    if (_updating) return;
    final value = _controller.value;
    final old = _text;
    if (value.text != old) {
      final (start, end, inserted) = _replaced(old, value.text, _selection, value.selection);
      if (!_session.replaceText(widget.node, start, end, inserted)) {
        _set(TextEditingValue(text: old, selection: _selection));
        return;
      }
    }
    _selection = value.selection;
    if (value.selection.isValid) {
      _session.select(DocSelection(widget.node, value.selection.baseOffset, value.selection.extentOffset));
    }
    _commands.follow();
  }

  // keywords

  /// The line holding an offset of the text: its start and its end.
  (int, int) _lineAt(int offset) {
    final text = _controller.text;
    final end = text.indexOf('\n', offset);
    return (offset == 0 ? 0 : text.lastIndexOf('\n', offset - 1) + 1, end < 0 ? text.length : end);
  }

  @override
  ({String text, int caret})? get typing {
    final s = _controller.selection;
    if (!s.isValid || !s.isCollapsed || _session.readOnly || !_focus.hasFocus) return null;
    final (start, end) = _lineAt(s.extentOffset);
    return (text: _controller.text.substring(start, end), caret: s.extentOffset - start);
  }

  @override
  ({String before, String after}) get around {
    final (start, end) = _lineAt(_controller.selection.extentOffset);
    return (before: _controller.text.substring(0, start), after: _controller.text.substring(end));
  }

  @override
  Rect? rectAt(int offset) {
    RenderEditable? editable;
    void visit(RenderObject object) {
      if (object is RenderEditable) {
        editable = object;
      } else if (editable == null) {
        object.visitChildren(visit);
      }
    }

    _field.currentContext?.findRenderObject()?.visitChildren(visit);
    final found = editable;
    if (found == null) return null;
    final (start, _) = _lineAt(_controller.selection.extentOffset);
    final caret = found.getLocalRectForCaret(TextPosition(offset: start + offset));
    return Rect.fromPoints(found.localToGlobal(caret.topLeft), found.localToGlobal(caret.bottomRight));
  }

  @override
  void write(int start, int end, String text, {Uri? link}) {
    final (line, _) = _lineAt(_controller.selection.extentOffset);
    final written = link == null ? text : '$text ';
    _controller.value = TextEditingValue(
      text: _controller.text.replaceRange(line + start, line + end, written),
      selection: TextSelection.collapsed(offset: line + start + written.length),
    );
  }

  /// Follows a change of the document the field did not make.
  void _changed(Edit change) {
    final text = _text;
    if (_controller.text == text) return;
    int move(int offset) {
      if (offset < 0) return offset;
      for (final c in change.changes) {
        if (c.kind == ChangeKind.text && c.id == widget.node) offset = c.text!.transformPosition(offset, thisFirst: false);
      }
      return offset;
    }

    final value = _controller.value;
    final selection = value.selection.copyWith(
      baseOffset: move(value.selection.baseOffset),
      extentOffset: move(value.selection.extentOffset),
    );
    final composing = value.composing.isValid
        ? TextRange(start: move(value.composing.start), end: move(value.composing.end))
        : TextRange.empty;
    _set(TextEditingValue(text: text, selection: selection, composing: composing));
    _commands.follow();
  }

  void _set(TextEditingValue value) {
    _updating = true;
    _controller.value = value;
    _selection = value.selection;
    _updating = false;
  }

  @override
  Widget build(BuildContext context) {
    return Actions(
      actions: {
        UndoTextIntent: CallbackAction<UndoTextIntent>(onInvoke: (_) => _session.undo()),
        RedoTextIntent: CallbackAction<RedoTextIntent>(onInvoke: (_) => _session.redo()),
      },
      child: CommandsMenu(
        commands: _commands,
        child: Focus(
          canRequestFocus: false,
          skipTraversal: true,
          onKeyEvent: (_, e) => _commands.key(e) ? KeyEventResult.handled : KeyEventResult.ignored,
          child: TextField(
            key: _field,
            controller: _controller,
            focusNode: _focus,
            autofocus: widget.autofocus,
            // an answer on its way holds the text
            readOnly: _session.readOnly || !_session.loaded || _commands.answering,
            maxLines: null,
            expands: true,
            keyboardType: TextInputType.multiline,
            textAlignVertical: TextAlignVertical.top,
            style: widget.style,
            decoration: InputDecoration(border: InputBorder.none, contentPadding: widget.padding),
          ),
        ),
      ),
    );
  }
}

/// Where [now] differs from [old]: the range of [old] replaced and the text
/// put there. The selections before and after tell which of several equal
/// characters was typed or deleted, and a surrogate pair is never split.
(int, int, String) _replaced(String old, String now, TextSelection before, TextSelection after) {
  var prefix = 0;
  final shorter = math.min(old.length, now.length);
  while (prefix < shorter && old.codeUnitAt(prefix) == now.codeUnitAt(prefix)) {
    prefix++;
  }
  if (before.isValid && after.isValid) prefix = math.min(prefix, math.min(before.start, after.start));
  if (prefix > 0 && _isHigh(old.codeUnitAt(prefix - 1))) prefix--;
  var suffix = 0;
  while (suffix < old.length - prefix &&
      suffix < now.length - prefix &&
      old.codeUnitAt(old.length - 1 - suffix) == now.codeUnitAt(now.length - 1 - suffix)) {
    suffix++;
  }
  if (suffix > 0 && suffix < old.length && _isHigh(old.codeUnitAt(old.length - 1 - suffix))) suffix--;
  return (prefix, old.length - suffix, now.substring(prefix, now.length - suffix));
}

bool _isHigh(int unit) => unit >= 0xD800 && unit < 0xDC00;

/// Paints the selections of others behind the text.
class _PresenceController extends TextEditingController {
  _PresenceController(this.session, this.node) {
    session.presence.addListener(notifyListeners);
  }

  final DocSession session;
  final String node;

  @override
  void dispose() {
    session.presence.removeListener(notifyListeners);
    super.dispose();
  }

  @override
  TextSpan buildTextSpan({required BuildContext context, TextStyle? style, required bool withComposing}) {
    final marks = <(int, int)>[];
    for (final peer in session.peers) {
      final s = peer.selection;
      if (s == null || s.node != node) continue;
      var start = s.start.clamp(0, text.length);
      var end = s.end.clamp(0, text.length);
      if (start == end) {
        // a caret: the character it stands before, or after at the end
        if (text.isEmpty) continue;
        if (end < text.length) {
          end++;
        } else {
          start--;
        }
      }
      marks.add((start, end));
    }
    if (marks.isEmpty || (withComposing && value.composing.isValid && !value.composing.isCollapsed)) {
      return super.buildTextSpan(context: context, style: style, withComposing: withComposing);
    }
    final marked = TextStyle(backgroundColor: Theme.of(context).colorScheme.primary.withValues(alpha: 0.2));
    final cuts = {0, text.length, for (final m in marks) ...[m.$1, m.$2]}.toList()..sort();
    return TextSpan(
      style: style,
      children: [
        for (var i = 0; i + 1 < cuts.length; i++)
          if (cuts[i] < cuts[i + 1])
            TextSpan(
              text: text.substring(cuts[i], cuts[i + 1]),
              style: marks.any((m) => m.$1 <= cuts[i] && cuts[i + 1] <= m.$2) ? marked : null,
            ),
      ],
    );
  }
}
