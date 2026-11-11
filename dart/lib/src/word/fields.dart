import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import 'blocks.dart' show formatNumber;
import 'document.dart';
import 'edits.dart';
import 'layout.dart';
import 'revisions.dart';

const _object = '￼';

/// The instructions a table of contents is inserted with.
const tocInstructions = r'TOC \o "1-3" \h';

/// The formats of dates and times the ribbon offers, as a DATE field writes
/// them.
const dateFormats = ['dd/MM/yyyy', 'd MMMM yyyy', 'dddd d MMMM yyyy', 'dd/MM/yyyy HH:mm', 'HH:mm'];

/// A field of a flow, the result it shows between [resultStart] and
/// [resultEnd]: that of the characters from a "begin" to an "end", or the
/// text a simple field wraps.
class WordField {
  const WordField(this.flow, this.instructions, this.start, this.end, this.resultStart, this.resultEnd);

  final String flow;
  final String instructions;
  final int start, end;
  final int resultStart, resultEnd;

  /// The name of the field: "TOC", "DATE"…
  String get name => instructions.split(RegExp(r'\s+')).first.toUpperCase();
}

class _Open {
  _Open(this.start);

  final int start;
  final instructions = StringBuffer();
  int? result;
}

/// The fields of a flow that are not in the result of another, in order.
List<WordField> fieldsOf(Node flow) {
  final out = <WordField>[];
  final open = <_Open>[];
  String? simple;
  var simpleStart = 0;
  var at = 0;
  void closeSimple() {
    final s = simple;
    if (s != null) out.add(WordField(flow.id, s, simpleStart, at, simpleStart, at));
    simple = null;
  }

  for (final op in flow.text!.ops) {
    final a = op.attributes ?? const <String, String>{};
    final field = open.isEmpty ? a['field']?.trim() : null;
    if (field != simple) {
      closeSimple();
      if (field != null) (simple, simpleStart) = (field, at);
    }
    if (a['del'] == null) {
      for (var i = 0; i < op.length; i++) {
        switch (a['fld']) {
          case 'begin':
            open.add(_Open(at + i));
          case 'separate':
            open.lastOrNull?.result ??= at + i + 1;
          case 'end' when open.isNotEmpty:
            final f = open.removeLast();
            if (open.isEmpty) out.add(WordField(flow.id, f.instructions.toString().trim(), f.start, at + i + 1, f.result ?? at + i, at + i));
        }
      }
      final instr = a['instr'];
      if (instr != null && open.isNotEmpty && open.last.result == null) open.last.instructions.write(instr);
    }
    at += op.length;
  }
  closeSimple();
  return out;
}

/// The field an offset of a flow is in, or at the edge of.
WordField? fieldAt(Node flow, int offset) => fieldsOf(flow).where((f) => offset >= f.start && offset <= f.end).firstOrNull;

/// The words of instructions, what is in quotes one word; switches are
/// the words that start with a backslash.
List<String> _words(String instructions) => [for (final m in RegExp(r'"([^"]*)"|(\S+)').allMatches(instructions)) m.group(1) ?? m.group(2)!];

/// The switches of instructions and their arguments.
Map<String, String> _switches(List<String> words) {
  final out = <String, String>{};
  for (var i = 0; i < words.length; i++) {
    if (!words[i].startsWith(r'\')) continue;
    final arg = i + 1 < words.length && !words[i + 1].startsWith(r'\') ? words[i + 1] : '';
    out[words[i]] = arg;
  }
  return out;
}

/// The word after the name of a field: the bookmark of a REF, the
/// identifier of a SEQ.
String _argument(List<String> words) => words.length > 1 && !words[1].startsWith(r'\') ? words[1] : '';

/// [t] written as a Word date format asks: "dd/MM/yyyy", "d MMMM yyyy",
/// "HH:mm"; text in single quotes goes as it is.
String formatDate(DateTime t, String format, LofficeStrings s) {
  final out = StringBuffer();
  var i = 0;
  while (i < format.length) {
    final c = format[i];
    if (c == "'") {
      final end = format.indexOf("'", i + 1);
      out.write(format.substring(i + 1, end < 0 ? format.length : end));
      i = end < 0 ? format.length : end + 1;
      continue;
    }
    final meridiem = const [('AM/PM', 'AM', 'PM'), ('am/pm', 'am', 'pm'), ('A/P', 'A', 'P'), ('a/p', 'a', 'p')].where((m) => format.startsWith(m.$1, i)).firstOrNull;
    if (meridiem != null) {
      out.write(t.hour < 12 ? meridiem.$2 : meridiem.$3);
      i += meridiem.$1.length;
      continue;
    }
    var n = 1;
    while (i + n < format.length && format[i + n] == c) {
      n++;
    }
    String two(int v) => v.toString().padLeft(2, '0');
    final text = switch ((c, n)) {
      ('d', 1) => '${t.day}',
      ('d', 2) => two(t.day),
      ('d', 3) => s.weekdaysShort[t.weekday - 1],
      ('d', _) => s.weekdays[t.weekday - 1],
      ('M', 1) => '${t.month}',
      ('M', 2) => two(t.month),
      ('M', 3) => s.monthsShort[t.month - 1],
      ('M', _) => s.months[t.month - 1],
      ('y', 1 || 2) => two(t.year % 100),
      ('y', _) => '${t.year}'.padLeft(4, '0'),
      ('h', 1) => '${t.hour % 12 == 0 ? 12 : t.hour % 12}',
      ('h', _) => two(t.hour % 12 == 0 ? 12 : t.hour % 12),
      ('H', 1) => '${t.hour}',
      ('H', _) => two(t.hour),
      ('m', 1) => '${t.minute}',
      ('m', _) => two(t.minute),
      ('s', 1) => '${t.second}',
      ('s', _) => two(t.second),
      _ => c * n,
    };
    out.write(text);
    i += n;
  }
  return out.toString();
}

/// A heading of the document a table of contents lists.
class _Heading {
  _Heading(this.flow, this.start, this.end, this.level, this.title);

  final String flow;
  final int start, end, level;
  final String title;

  /// The bookmark entries link to: the one the heading has, or the one made.
  String? bookmark;
}

class _Bookmark {
  const _Bookmark(this.name, this.flow, this.at);

  final String name;
  final String flow;
  final int at;
}

class _Patch {
  const _Patch(this.start, this.end, this.insert, this.order);

  final int start, end;
  final Delta insert;
  final int order;
}

/// What an update or an insertion changes in a document: the patches of
/// its flows, the bookmarks, the headings and the counters being worked
/// out as fields ask.
class _Fields {
  _Fields(this.doc, this.layout, this.strings, this.now) : flows = flowsOf(doc.tree) {
    for (final flow in flows) {
      var at = 0;
      for (final op in flow.text!.ops) {
        final a = op.attributes;
        if (a?['bm'] != null) starts.add(_Bookmark(a!['bm']!, flow.id, at));
        if (a?['bme'] != null) ends[a!['bme']!] = _Bookmark(a['bme']!, flow.id, at);
        at += op.length;
      }
    }
  }

  final WordDocument doc;
  final WordLayout layout;
  final LofficeStrings strings;
  final DateTime now;
  final List<Node> flows;
  final starts = <_Bookmark>[];
  final ends = <String, _Bookmark>{};
  final _patches = <String, List<_Patch>>{};
  final _counters = <String, int>{};
  final _made = <String, String>{};
  late int _nextToc = starts.map((b) => int.tryParse(b.name.replaceFirst('_Toc', ''))).nonNulls.fold(100000000, (a, b) => a > b ? a : b) + 1;

  /// Replaces [start] to [end] of a flow; at one offset, the patches go in
  /// the order of [order]: the ends of bookmarks first.
  void patch(String flow, int start, int end, Delta insert, {int order = 1}) => (_patches[flow] ??= []).add(_Patch(start, end, insert, order));

  Edit get edit => Edit([
    for (final e in _patches.entries)
      if (_compose(e.value) case final d when !d.isEmpty) Change.text(e.key, d),
  ]);

  Delta _compose(List<_Patch> patches) {
    final sorted = [...patches]..sort((a, b) => a.start != b.start ? a.start - b.start : (a.end != b.end ? a.end - b.end : a.order - b.order));
    final d = Delta();
    var at = 0;
    for (final p in sorted) {
      if (p.start < at) continue;
      d.retain(p.start - at);
      d.delete(p.end - p.start);
      for (final op in p.insert.ops) {
        d.push(op);
      }
      at = p.end;
    }
    return d.chop();
  }

  /// The characters of a flow from [from] to [to], without what is struck
  /// or an object.
  String plain(Node flow, int from, int to) =>
      slice(flow.text!, from, to).ops.where((op) => op.attributes?['del'] == null).map((op) => op.insert!).join().replaceAll(_object, '').replaceAll(RegExp(r'[\n\v\t]'), ' ');

  /// The ops of a result as they show: what is struck gone, and the
  /// revisions with it.
  Delta visible(Node flow, int start, int end) {
    final out = Delta();
    for (final op in slice(flow.text!, start, end).ops) {
      final a = op.attributes ?? const <String, String>{};
      if (a['del'] != null) continue;
      out.insert(op.insert!, {
        for (final e in a.entries)
          if (!revisionKeys.contains(e.key)) e.key: e.value,
      });
    }
    return out;
  }

  /// [d] without what tells the same result apart: the elements it was
  /// read from, the spaces around instructions.
  Delta same(Delta d) {
    final out = Delta();
    for (final op in d.ops) {
      out.insert(op.insert!, {
        for (final e in (op.attributes ?? const <String, String>{}).entries)
          if (!const {'o', 'wrap', 'r', 'p', 'pa'}.contains(e.key)) e.key: e.key == 'instr' ? e.value.trim() : e.value,
      });
    }
    return out;
  }

  /// The formatting a new result takes: that of the old one.
  Attributes template(WordField f, Delta shown) {
    final first = shown.ops.where((op) => op.insert != _object).firstOrNull;
    if (first != null) return first.attributes ?? const {};
    final flow = doc.tree[f.flow]!;
    final at = f.resultStart > 0 ? slice(flow.text!, f.resultStart - 1, f.resultStart).ops.firstOrNull?.attributes : null;
    return {
      for (final e in (at ?? const <String, String>{}).entries)
        if (!wordObjectKeys.contains(e.key) && !revisionKeys.contains(e.key)) e.key: e.value,
    };
  }

  String pageLabel(String flow, int offset) {
    final at = layout.placementAt(flow, offset);
    if (at == null) return '';
    final page = layout.pages[at.$1];
    return formatNumber(page.number, page.section.pageFormat);
  }

  double textWidth(String flow, int offset) {
    final at = layout.placementAt(flow, offset)?.$1 ?? 0;
    return layout.pages.isEmpty ? 450 : layout.pages[at].body.width;
  }

  /// The text of the bookmark [name]: null when it is none.
  String? bookmarkText(String name) {
    final start = starts.where((b) => b.name == name).firstOrNull;
    final end = ends[name];
    if (start == null || end == null) return null;
    final from = flows.indexWhere((f) => f.id == start.flow), to = flows.indexWhere((f) => f.id == end.flow);
    if (from < 0 || to < from) return null;
    final text = StringBuffer();
    for (var i = from; i <= to; i++) {
      final flow = flows[i];
      text.write(plain(flow, i == from ? start.at + 1 : 0, i == to ? end.at : flow.text!.length));
    }
    return text.toString().trim();
  }

  /// The headings a table of contents with [switches] lists: the styles
  /// "heading n" of its levels, the outline levels with \u, the styles
  /// named by \t.
  List<_Heading> headings(Map<String, String> switches) {
    final range = RegExp(r'^(\d+)-(\d+)$').firstMatch(switches[r'\o'] ?? '');
    final byOutline = switches.containsKey(r'\u');
    final custom = (switches[r'\t'] ?? '').split(RegExp('[,;]'));
    final byStyle = {for (var i = 0; i + 1 < custom.length; i += 2) custom[i].trim().toLowerCase(): int.tryParse(custom[i + 1].trim()) ?? 0};
    final byHeading = range != null || !byOutline && byStyle.isEmpty;
    final from = range == null ? 1 : int.parse(range.group(1)!), to = range == null ? (byOutline ? 9 : 3) : int.parse(range.group(2)!);
    int? levelOf(Attributes mark) {
      final id = doc.styleOf(mark) ?? '';
      final name = (doc.styles[id]?.name ?? '').toLowerCase();
      final named = byStyle[name] ?? byStyle[id.toLowerCase()];
      if (named != null) return named > 0 ? named : null;
      final heading = int.tryParse(RegExp(r'^heading (\d)$').firstMatch(name)?.group(1) ?? '');
      final outline = int.tryParse(doc.paragraph(mark)['outline'] ?? '');
      final level = byHeading && heading != null ? heading : (byOutline && outline != null ? outline + 1 : null);
      return level != null && level >= from && level <= to ? level : null;
    }

    final out = <_Heading>[];
    for (final flow in flows) {
      var start = 0, at = 0;
      for (final op in flow.text!.ops) {
        final s = op.insert!;
        for (var i = s.indexOf('\n'); i >= 0; i = s.indexOf('\n', i + 1)) {
          final end = at + i;
          final level = levelOf(op.attributes ?? const {});
          final title = level == null ? '' : plain(flow, start, end).trim();
          if (title.isNotEmpty) out.add(_Heading(flow.id, start, end, level!, title));
          start = end + 1;
        }
        at += s.length;
      }
    }
    return out;
  }

  /// The result of a table of contents: an entry for each of [list], with
  /// its page, a link to a bookmark of its heading when asked for.
  Delta toc(String instructions, double width) {
    final switches = _switches(_words(instructions));
    final linked = switches.containsKey(r'\h');
    final paged = !switches.containsKey(r'\n');
    final list = headings(switches);
    final result = Delta();
    if (list.isEmpty) return result..insert(strings.noTocEntries);
    final tabs = 'right:${(width * 20).round()}:dot';
    for (final h in list) {
      if (linked) h.bookmark = bookmarkOf(h);
      final link = linked ? {'link': '#${h.bookmark}'} : null;
      final page = pageLabel(h.flow, h.start);
      result.insert(h.title, link);
      if (paged && page.isNotEmpty) {
        result.insert('\t', link);
        if (linked) {
          result
            ..insert(_object, {...?link, 'fld': 'begin'})
            ..insert(_object, {...?link, 'instr': 'PAGEREF ${h.bookmark} \\h'})
            ..insert(_object, {...?link, 'fld': 'separate'})
            ..insert(page, link)
            ..insert(_object, {...?link, 'fld': 'end'});
        } else {
          result.insert(page);
        }
      }
      result.insert('\n', {'pstyle': tocStyle(h.level), 'tabs': tabs});
    }
    return result;
  }

  String tocStyle(int level) => doc.styles.entries.where((e) => e.value.name.toLowerCase() == 'toc $level').map((e) => e.key).firstOrNull ?? 'TOC$level';

  /// The bookmark a heading has for a table of contents, made when it has
  /// none.
  String bookmarkOf(_Heading h) {
    final own = starts.where((b) => b.flow == h.flow && b.at >= h.start && b.at <= h.end && b.name.startsWith('_Toc')).firstOrNull;
    if (own != null) return own.name;
    return _made.putIfAbsent('${h.flow}:${h.start}', () {
      final name = '_Toc${_nextToc++}';
      patch(h.flow, h.start, h.start, Delta()..insert(_object, {'bm': name}));
      patch(h.flow, h.end, h.end, Delta()..insert(_object, {'bme': name}), order: -1);
      return name;
    });
  }

  /// The result a field shows now, null when it is not one this updates.
  Delta? result(WordField f, Delta shown) {
    final words = _words(f.instructions);
    final switches = _switches(words);
    final attrs = template(f, shown);
    String? text;
    switch (f.name) {
      case 'DATE' || 'TIME':
        text = formatDate(now, switches[r'\@'] ?? (f.name == 'DATE' ? 'dd/MM/yyyy' : 'HH:mm'), strings);
      case 'SEQ':
        final id = _argument(words);
        final n = switches.containsKey(r'\r') ? int.tryParse(switches[r'\r']!) ?? 1 : (switches.containsKey(r'\c') ? _counters[id] ?? 1 : (_counters[id] ?? 0) + 1);
        _counters[id] = n;
        text = formatNumber(n, switch (switches[r'\*']) {
          'ROMAN' => 'upperRoman',
          'roman' => 'lowerRoman',
          'ALPHABETIC' => 'upperLetter',
          'alphabetic' => 'lowerLetter',
          _ => 'decimal',
        });
      case 'REF':
        text = bookmarkText(_argument(words)) ?? strings.referenceNotFound;
      case 'PAGEREF':
        final b = starts.where((b) => b.name == _argument(words)).firstOrNull;
        text = b == null ? strings.referenceNotFound : pageLabel(b.flow, b.at);
        if (text.isEmpty) return null;
      case 'TOC':
        return toc(f.instructions, textWidth(f.flow, f.start));
      default:
        return null;
    }
    return Delta()..insert(text, attrs);
  }
}

/// The edit that brings the fields of the body up to date: the dates and
/// times, the numbers of sequences, the texts of references, the pages and
/// the tables of contents, those [where] keeps; what is wrong is kept as
/// it was. Empty when nothing changes.
Edit updateFields(WordDocument doc, WordLayout layout, LofficeStrings strings, {DateTime? now, bool Function(WordField field)? where}) {
  final u = _Fields(doc, layout, strings, now ?? DateTime.now());
  for (final flow in u.flows) {
    for (final f in fieldsOf(flow)) {
      final wanted = where == null || where(f);
      if (f.name == 'TOC' && !wanted) continue;
      final shown = u.visible(flow, f.resultStart, f.resultEnd);
      final result = u.result(f, shown);
      if (result != null && wanted && u.same(result) != u.same(shown)) u.patch(flow.id, f.resultStart, f.resultEnd, result);
    }
  }
  return u.edit;
}

/// The edit that inserts a table of contents at [offset] of [flow], a
/// paragraph of its own, with an entry for each heading of the document.
Edit insertToc(WordDocument doc, WordLayout layout, LofficeStrings strings, Node flow, int offset) {
  final u = _Fields(doc, layout, strings, DateTime.now());
  final editing = wordEditing(flow);
  final (start, end) = editing.paragraphAt(offset);
  final like = WordEdits.paragraphLike(editing.markAt(offset));
  final insert = Delta();
  if (offset > start) insert.insert('\n', like.isEmpty ? null : like);
  insert
    ..insert(_object, {'fld': 'begin'})
    ..insert(_object, {'instr': tocInstructions})
    ..insert(_object, {'fld': 'separate'});
  for (final op in u.toc(tocInstructions, u.textWidth(flow.id, offset)).ops) {
    insert.push(op);
  }
  insert.insert(_object, {'fld': 'end'});
  if (offset < end) insert.insert('\n', like.isEmpty ? null : like);
  u.patch(flow.id, offset, offset, insert, order: 0);
  return u.edit;
}
