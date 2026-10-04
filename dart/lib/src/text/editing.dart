import 'package:characters/characters.dart';
import 'package:trame/trame.dart';

import 'text_frame.dart';

/// The keys of a kind of flow: those of paragraphs, those a mark keeps to
/// itself when its paragraph is split or joined, and those that describe an
/// object, which text typed after it does not take.
class FlowKeys {
  const FlowKeys({required this.paragraph, this.own = const {}, this.objects = const {'fld', 'o'}});

  final Set<String> paragraph, own, objects;

  /// The keys of the text bodies of DrawingML.
  static const drawing = FlowKeys(paragraph: paraKeys);
}

/// Edits of a flow as PowerPoint and Word make them, each a delta to send.
/// Offsets count UTF-16 units; an edit never splits a character.
class FlowEditing {
  FlowEditing(this.flow, {this.keys = FlowKeys.drawing}) : text = flow.text;

  final Delta flow;
  final FlowKeys keys;

  /// The flow as text: paragraph marks are "\n", line breaks "\v".
  final String text;

  /// The run attributes of the character at [offset], null past the end.
  Attributes? attributesAt(int offset) {
    var at = 0;
    for (final op in flow.ops) {
      final n = op.insert!.length;
      if (offset < at + n) return op.attributes;
      at += n;
    }
    return null;
  }

  /// The attributes text typed at [offset] takes: those of the character
  /// before it in its paragraph, or of the first one, or of the paragraph
  /// mark in an empty paragraph; paragraph and field keys left out.
  Attributes typingAttributes(int offset) {
    final (start, end) = paragraphAt(offset);
    final source = offset > start
        ? attributesAt(offset - 1)
        : (end > start ? attributesAt(start) : attributesAt(end));
    return {
      for (final e in (source ?? const <String, String>{}).entries)
        if (!keys.paragraph.contains(e.key) && !keys.objects.contains(e.key)) e.key: e.value,
    };
  }

  /// The paragraph holding [offset]: its start, and the offset of its mark.
  (int, int) paragraphAt(int offset) {
    final start = offset == 0 ? 0 : text.lastIndexOf('\n', offset - 1) + 1;
    final end = text.indexOf('\n', offset);
    return (start, end < 0 ? text.length - 1 : end);
  }

  /// The attributes of the mark of the paragraph holding [offset].
  Attributes markAt(int offset) => attributesAt(paragraphAt(offset).$2) ?? const {};

  /// Replaces [start]…[end] by [inserted], typed with [attributes]; each
  /// "\n" in it ends a paragraph like the one it is typed in.
  Delta replace(int start, int end, String inserted, Attributes attributes) {
    final d = Delta()..retain(start);
    if (end > start) d.delete(end - start);
    final mark = {...attributes, ..._paragraphKeys(markAt(start))}..removeWhere((k, _) => keys.own.contains(k));
    final parts = inserted.split('\n');
    for (var i = 0; i < parts.length; i++) {
      if (parts[i].isNotEmpty) d.insert(parts[i], attributes.isEmpty ? null : attributes);
      if (i < parts.length - 1) d.insert('\n', mark.isEmpty ? null : mark);
    }
    return d.chop();
  }

  /// Enter: a new paragraph, like the one the caret is in.
  Delta newParagraph(int start, int end) => replace(start, end, '\n', typingAttributes(start));

  /// Backspace: the selection, or the character before the caret. At the
  /// start of a paragraph, it joins the one before, which keeps its
  /// formatting. Null when there is nothing to delete.
  Delta? deleteBackward(int start, int end) {
    if (end > start) return _delete(start, end);
    if (start == 0) return null;
    final before = text.substring(0, start).characters;
    return _delete(start - before.last.length, start);
  }

  /// Delete: the selection, or the character after the caret; never the
  /// last paragraph mark.
  Delta? deleteForward(int start, int end) {
    if (end > start) return _delete(start, end);
    if (start >= text.length - 1) return null;
    final after = text.substring(start).characters;
    return _delete(start, start + after.first.length);
  }

  /// Deletes a range; when it takes paragraph marks, the paragraph left
  /// keeps the formatting of the first one.
  Delta delete(int start, int end) => _delete(start, end);

  Delta _delete(int start, int end) {
    end = end.clamp(start, text.length - 1);
    final d = Delta()
      ..retain(start)
      ..delete(end - start);
    if (!text.substring(start, end).contains('\n')) return d.chop();
    final first = _paragraphKeys(markAt(start))..removeWhere((k, _) => keys.own.contains(k));
    final (_, mark) = paragraphAt(end);
    final last = _paragraphKeys(attributesAt(mark) ?? const {})..removeWhere((k, _) => keys.own.contains(k));
    final change = <String, String>{
      for (final k in {...first.keys, ...last.keys})
        if (first[k] != last[k]) k: first[k] ?? '',
    };
    if (change.isNotEmpty) {
      d
        ..retain(mark - end)
        ..retain(1, change);
    }
    return d.chop();
  }

  /// Sets or removes ("" value) run attributes over a range; at a caret,
  /// null: the caller keeps them for what is typed next.
  Delta? formatRuns(int start, int end, Attributes attributes) {
    if (end <= start) return null;
    final d = Delta()..retain(start);
    // marks keep their run attributes too, for empty paragraphs
    d.retain(end - start, attributes);
    return d.chop();
  }

  /// Sets or removes paragraph attributes on every paragraph a range
  /// touches.
  Delta formatParagraphs(int start, int end, Attributes attributes) {
    final d = Delta();
    var at = 0;
    var offset = start;
    while (true) {
      final (_, mark) = paragraphAt(offset);
      d
        ..retain(mark - at)
        ..retain(1, attributes);
      at = mark + 1;
      if (mark >= end || mark >= text.length - 1) break;
      offset = mark + 1;
    }
    return d.chop();
  }

  /// The paragraph attributes of each paragraph a range touches.
  List<Attributes> paragraphsIn(int start, int end) {
    final out = <Attributes>[];
    var offset = start;
    while (true) {
      final (_, mark) = paragraphAt(offset);
      out.add(attributesAt(mark) ?? const {});
      if (mark >= end || mark >= text.length - 1) break;
      offset = mark + 1;
    }
    return out;
  }

  /// The next caret position after [offset], by character or by word.
  int next(int offset, {bool word = false}) {
    if (offset >= text.length - 1) return text.length - 1;
    if (!word) return offset + text.substring(offset).characters.first.length;
    final match = RegExp(r'[\s\v]*[^\s\v]+|[\s\v]+').matchAsPrefix(text, offset);
    return (match?.end ?? offset + 1).clamp(0, text.length - 1);
  }

  /// The previous caret position before [offset].
  int previous(int offset, {bool word = false}) {
    if (offset <= 0) return 0;
    if (!word) return offset - text.substring(0, offset).characters.last.length;
    var i = offset;
    while (i > 0 && _space(text.codeUnitAt(i - 1))) {
      i--;
    }
    while (i > 0 && !_space(text.codeUnitAt(i - 1))) {
      i--;
    }
    return i;
  }

  /// The word around [offset], for a double click.
  (int, int) wordAt(int offset) {
    var start = offset, end = offset;
    while (start > 0 && !_space(text.codeUnitAt(start - 1))) {
      start--;
    }
    while (end < text.length - 1 && !_space(text.codeUnitAt(end))) {
      end++;
    }
    return (start, end);
  }

  static bool _space(int c) => c == 0x20 || c == 0x0A || c == 0x0B || c == 0x09 || c == 0xA0;

  Attributes _paragraphKeys(Attributes attrs) => {
    for (final e in attrs.entries)
      if (keys.paragraph.contains(e.key)) e.key: e.value,
  };
}
