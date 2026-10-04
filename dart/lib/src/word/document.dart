import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:trame/trame.dart';

import '../text/text_frame.dart' show Props;

/// The keys of paragraphs in the flows of a Word document, see the Go
/// package docx; the others are those of runs. The XML a mark was read from,
/// "p" and "pa", goes with the paragraph, and so does its section.
const wordParagraphKeys = {
  'pstyle', 'jc', 'ind.left', 'ind.right', 'ind.first', 'sp.before', 'sp.after', 'sp.line', 'sp.rule', //
  'sp.beforeAuto', 'sp.afterAuto', 'num', 'lvl', 'keepNext', 'keepLines', 'pageBreakBefore', 'widowControl',
  'contextualSpacing', 'bidi', 'outline', 'tabs', 'pshd', 'pbdr', 'p', 'pa', 'sect', 'sx',
};

/// The keys of runs the model knows, which formatting sets.
const wordRunKeys = {
  'rstyle', 'b', 'i', 'caps', 'smallCaps', 'strike', 'dstrike', 'vanish', 'u', 'sz', 'font', 'fontEa', //
  'fontCs', 'color', 'hl', 'shd', 'va', 'spc', 'pos', 'scale',
};

/// The keys of a mark that stay with it: a paragraph split in two does not
/// give both halves its section or its id.
const wordOwnKeys = {'sect', 'sx', 'pa'};

/// The keys that describe an element of a run, which text typed after it
/// does not take.
const wordObjectKeys = {'o', 'po', 'img', 'fld', 'instr', 'br', 'sym', 'note', 'comment', 'cs', 'ce', 'bm', 'math'};

/// A style of the document.
class WordStyle {
  WordStyle.fromJson(Map<String, Object?> j)
    : type = j['type'] as String? ?? 'paragraph',
      name = j['name'] as String? ?? '',
      basedOn = j['basedOn'] as String?,
      next = j['next'] as String?,
      link = j['link'] as String?,
      isDefault = j['default'] == true,
      hidden = j['hidden'] == true,
      quick = j['quick'] == true,
      priority = (j['priority'] as num?)?.toInt(),
      p = _props(j['p']),
      r = _props(j['r']),
      tbl = _map(j['tbl']),
      tr = _map(j['tr']),
      tc = _map(j['tc']),
      cond = {
        for (final e in _map(j['cond']).entries)
          if (e.value is Map<String, Object?>) e.key: WordStyle.fromJson(e.value as Map<String, Object?>),
      };

  final String type;
  final String name;
  final String? basedOn;
  final String? next;
  final String? link;
  final bool isDefault;
  final bool hidden;
  final bool quick;
  final int? priority;
  final Props p;
  final Props r;
  final Map<String, Object?> tbl;
  final Map<String, Object?> tr;
  final Map<String, Object?> tc;

  /// The formatting of parts of a table: "firstRow", "band1Horz"…
  final Map<String, WordStyle> cond;
}

/// A level of a list.
class WordLevel {
  WordLevel.fromJson(Map<String, Object?> j)
    : start = (j['start'] as num?)?.toInt() ?? 0,
      format = j['fmt'] as String? ?? 'decimal',
      text = j['text'] as String? ?? '',
      jc = j['jc'] as String? ?? 'left',
      suffix = j['suff'] as String? ?? 'tab',
      restart = (j['restart'] as num?)?.toInt(),
      legal = j['legal'] == true,
      p = _props(j['p']),
      r = _props(j['r']),
      override = j['override'] == true;

  final int start;
  final String format;
  final String text;
  final String jc;
  final String suffix;
  final int? restart;
  final bool legal;
  final Props p;
  final Props r;
  final bool override;
}

/// A list: its levels, and the abstract numbering whose counters it shares.
class WordNumbering {
  WordNumbering.fromJson(Map<String, Object?> j)
    : abstract = j['abstract'] as String? ?? '',
      levels = [
        for (final l in (j['levels'] as List<Object?>? ?? const []))
          WordLevel.fromJson(l is Map<String, Object?> ? l : const {}),
      ];

  final String abstract;
  final List<WordLevel> levels;
}

/// The layout of the pages of a section, in twentieths of a point.
class WordSection {
  WordSection.fromJson(Map<String, Object?> j)
    : width = _int(j['w'], 12240),
      height = _int(j['h'], 15840),
      landscape = j['orient'] == 'landscape',
      top = _int(j['top'], 1440),
      right = _int(j['right'], 1800),
      bottom = _int(j['bottom'], 1440),
      left = _int(j['left'], 1800),
      header = _int(j['header'], 720),
      footer = _int(j['footer'], 720),
      gutter = _int(j['gutter'], 0),
      columns = _int(j['cols'], 1).clamp(1, 45),
      columnSpace = _int(j['colSpace'], 720),
      type = j['type'] as String? ?? 'nextPage',
      titlePage = j['titlePg'] == true,
      headers = _strings(j['hdr']),
      footers = _strings(j['ftr']),
      pageStart = (j['pgStart'] as num?)?.toInt(),
      pageFormat = j['pgFmt'] as String? ?? 'decimal',
      json = j;

  static final fallback = WordSection.fromJson(const {});

  final int width, height;
  final bool landscape;
  final int top, right, bottom, left, header, footer, gutter;
  final int columns, columnSpace;
  final String type;
  final bool titlePage;

  /// The header and footer nodes by kind: "default", "first", "even".
  final Map<String, String> headers, footers;
  final int? pageStart;
  final String pageFormat;

  /// What the section was read from, for an edit to change it.
  final Map<String, Object?> json;
}

/// What settings.xml says of the layout.
class WordSettings {
  WordSettings.fromJson(Map<String, Object?> j)
    : tab = _int(j['tab'], 720),
      evenOdd = j['evenOdd'] == true,
      mirror = j['mirror'] == true,
      gutterTop = j['gutterTop'] == true,
      hyphen = j['hyphen'] == true,
      compat = _int(j['compat'], 0),
      noHtmlAuto = j['noHTMLAuto'] == true;

  final int tab;
  final bool evenOdd, mirror, gutterTop, hyphen, noHtmlAuto;
  final int compat;
}

/// A Word document as the Go package docx reads it: the styles, lists and
/// settings of its "doc" node, and the formatting in effect anywhere.
class WordDocument {
  /// Reads the document of a tree; what [previous] read of the same "doc"
  /// node is taken as it is.
  WordDocument(this.tree, {WordDocument? previous}) : _node = tree['doc'] {
    if (previous != null && identical(previous._node, _node)) {
      styles = previous.styles;
      defaultParagraph = previous.defaultParagraph;
      defaultRun = previous.defaultRun;
      numbering = previous.numbering;
      themeFonts = previous.themeFonts;
      themeColors = previous.themeColors;
      settings = previous.settings;
      lastSection = previous.lastSection;
      paragraphStyle = previous.paragraphStyle;
      tableStyle = previous.tableStyle;
      _chains = previous._chains;
      _paragraphs = previous._paragraphs;
      _runs = previous._runs;
      return;
    }
    _chains = {};
    _paragraphs = {};
    _runs = {};
    final a = _node?.attributes ?? const <String, Object?>{};
    styles = {
      for (final e in _map(a['styles']).entries)
        if (e.value is Map<String, Object?>) e.key: WordStyle.fromJson(e.value as Map<String, Object?>),
    };
    final defaults = _map(a['defaults']);
    defaultParagraph = _props(defaults['p']);
    defaultRun = _props(defaults['r']);
    numbering = {
      for (final e in _map(a['numbering']).entries)
        if (e.value is Map<String, Object?>) e.key: WordNumbering.fromJson(e.value as Map<String, Object?>),
    };
    themeFonts = _strings(a['fonts']);
    themeColors = _strings(a['colors']);
    settings = WordSettings.fromJson(_map(a['settings']));
    lastSection = a['sect'] is Map<String, Object?> ? WordSection.fromJson(a['sect'] as Map<String, Object?>) : WordSection.fallback;
    String? byDefault(String type) =>
        styles.entries.where((e) => e.value.type == type && e.value.isDefault).map((e) => e.key).firstOrNull;
    paragraphStyle = byDefault('paragraph');
    tableStyle = byDefault('table');
  }

  final Tree tree;
  final Node? _node;
  late final Map<String, WordStyle> styles;
  late final Props defaultParagraph;
  late final Props defaultRun;
  late final Map<String, WordNumbering> numbering;
  late final Map<String, String> themeFonts;
  late final Map<String, String> themeColors;
  late final WordSettings settings;

  /// The section of the last blocks of the body.
  late final WordSection lastSection;

  /// The default paragraph and table styles.
  late final String? paragraphStyle;
  late final String? tableStyle;

  late final Map<(String, String), Props> _chains;

  /// A style and those it is based on, the style first.
  List<WordStyle> _chain(String? id) {
    final chain = <WordStyle>[];
    final seen = <String>{};
    var at = id;
    while (at != null && seen.add(at) && chain.length < 20) {
      final s = styles[at];
      if (s == null) break;
      chain.add(s);
      at = s.basedOn;
    }
    return chain;
  }

  /// The keys of a style for paragraphs ("p") or runs ("r"), those of the
  /// styles it is based on under them.
  Props styleProps(String? id, String part) {
    if (id == null || !styles.containsKey(id)) return const {};
    return _chains[(id, part)] ??= {
      for (final s in _chain(id).reversed) ...(part == 'p' ? s.p : s.r),
    };
  }

  /// The table formatting of a style, those it is based on under it: its
  /// "tbl", "tr" or "tc".
  Map<String, Object?> tableProps(String? id, String part) => {
    for (final s in _chain(id).reversed) ...switch (part) {
      'tbl' => s.tbl,
      'tr' => s.tr,
      _ => s.tc,
    },
  };

  /// The paragraph style a mark names, or the default one.
  String? styleOf(Props mark) {
    final id = mark['pstyle'];
    return id != null && styles.containsKey(id) ? id : paragraphStyle;
  }

  /// The chain of a paragraph style cut at the default paragraph style:
  /// the default's part goes under a table's style, the rest over it.
  (List<WordStyle>, List<WordStyle>) _layers(String? id) {
    final chain = _chain(id);
    final base = styles[paragraphStyle];
    final at = base == null ? -1 : chain.indexOf(base);
    if (at < 0) return (const <WordStyle>[], chain.reversed.toList());
    return (chain.sublist(at).reversed.toList(), chain.sublist(0, at).reversed.toList());
  }

  /// The paragraph formatting in effect for a mark: the document's defaults,
  /// the default paragraph style, the table's style, the paragraph's own
  /// style, its list level, then the mark's own keys. Tab stops add up, a
  /// "clear" taking one away.
  Props paragraph(Props mark, {Props table = const {}}) => _paragraphs[_Pair(mark, table)] ??= _paragraph(mark, table);

  late final Map<_Pair, Props> _paragraphs, _runs;

  Props _paragraph(Props mark, Props table) {
    final (under, over) = _layers(styleOf(mark));
    final style = styleProps(styleOf(mark), 'p');
    final num = mark['num'] ?? style['num'];
    final lvl = mark['lvl'] ?? style['lvl'];
    final level = levelOf(num, lvl);
    final direct = {
      for (final e in mark.entries)
        if (wordParagraphKeys.contains(e.key)) e.key: e.value,
    };
    final layers = [defaultParagraph, for (final s in under) s.p, table, for (final s in over) s.p, ?level?.p, direct];
    final tabs = mergeTabs([for (final l in layers) l['tabs']]);
    return {
      for (final l in layers) ...l,
      'num': ?num,
      'lvl': ?lvl,
      'tabs': ?tabs,
    };
  }

  /// The run formatting a paragraph's runs start from: the defaults, the
  /// default paragraph style, the table's style and the paragraph's own
  /// style.
  Props paragraphRun(Props mark, {Props table = const {}}) => _runs[_Pair(mark, table)] ??= () {
    final (under, over) = _layers(styleOf(mark));
    return {
      ...defaultRun,
      for (final s in under) ...s.r,
      ...table,
      for (final s in over) ...s.r,
    };
  }();

  /// The run formatting in effect for the attributes of a run, from [base]
  /// the paragraph's.
  Props run(Props base, Props attrs) {
    final char = attrs['rstyle'];
    return {
      ...base,
      if (char != null) ...styleProps(char, 'r'),
      for (final e in attrs.entries)
        if (wordRunKeys.contains(e.key)) e.key: e.value,
    };
  }

  /// The level of a list, null when there is none: numbering "0" is none.
  WordLevel? levelOf(String? num, String? lvl) {
    if (num == null || num == '0') return null;
    final levels = numbering[num]?.levels;
    if (levels == null || levels.isEmpty) return null;
    final i = (int.tryParse(lvl ?? '') ?? 0).clamp(0, levels.length - 1);
    return levels[i];
  }

  /// The typeface of a font key: the theme's for "+major" and "+minor".
  String typeface(String? font) {
    if (font == null || font.isEmpty) return themeFonts['minor'] ?? 'Times New Roman';
    if (font == '+major') return themeFonts['major'] ?? 'Calibri Light';
    if (font == '+minor') return themeFonts['minor'] ?? 'Calibri';
    return font;
  }

  /// The sections of the body in order, each with the id of the text node
  /// and the offset of the mark ending it, the last one's null.
  List<(WordSection, String?, int?)> get sections {
    final out = <(WordSection, String?, int?)>[];
    void walk(String parent) {
      for (final n in tree.children(parent)) {
        switch (n.type) {
          case 'text':
            var at = 0;
            for (final op in n.text!.ops) {
              final s = op.insert ?? '';
              final sect = op.attributes?['sect'];
              if (sect != null) {
                var i = s.indexOf('\n');
                while (i >= 0) {
                  out.add((WordSection.fromJson(jsonObject(sect) ?? const {}), n.id, at + i));
                  i = s.indexOf('\n', i + 1);
                }
              }
              at += s.length;
            }
          case 'sdt':
            walk(n.id);
        }
      }
    }

    walk('body');
    out.add((lastSection, null, null));
    return out;
  }
}

/// Two maps of keys, equal when they hold the same: what formatting is
/// resolved from, found again whatever map holds it.
class _Pair {
  _Pair(this.a, this.b) : hashCode = Object.hash(_hash(a), _hash(b));

  final Props a, b;

  @override
  final int hashCode;

  static int _hash(Props m) => Object.hashAllUnordered([for (final e in m.entries) Object.hash(e.key, e.value)]);

  @override
  bool operator ==(Object other) => other is _Pair && mapEquals(other.a, a) && mapEquals(other.b, b);
}

Props _props(Object? v) => {
  if (v is Map<String, Object?>)
    for (final e in v.entries)
      if (e.value is String) e.key: e.value as String,
};

Map<String, Object?> _map(Object? v) => v is Map<String, Object?> ? v : const {};

Map<String, String> _strings(Object? v) => {
  if (v is Map<String, Object?>)
    for (final e in v.entries)
      if (e.value is String) e.key: e.value as String,
};

int _int(Object? v, int fallback) => v is num ? v.toInt() : fallback;

/// The tab stops of layers of formatting, lowest first, as one "tabs"
/// value; null when none has any.
String? mergeTabs(List<String?> layers) {
  if (layers.every((l) => l == null)) return null;
  final stops = <int, String>{};
  for (final l in layers) {
    for (final t in (l ?? '').split(' ')) {
      final parts = t.split(':');
      final pos = parts.length > 1 ? int.tryParse(parts[1]) : null;
      if (pos == null) continue;
      if (parts[0] == 'clear') {
        stops.remove(pos);
      } else {
        stops[pos] = t;
      }
    }
  }
  return (stops.keys.toList()..sort()).map((k) => stops[k]!).join(' ');
}

/// A JSON value of an attribute of a flow, null when it is not an object.
Map<String, Object?>? jsonObject(String? s) {
  if (s == null) return null;
  try {
    final v = jsonDecode(s);
    return v is Map<String, Object?> ? v : null;
  } on FormatException {
    return null;
  }
}
