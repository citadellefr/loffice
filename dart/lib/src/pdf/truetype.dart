import 'dart:typed_data';

/// A TrueType font as a PDF needs it: which glyph draws a character, how
/// far each advances, the measures of its descriptor, and the font again
/// with only the glyphs a document uses.
class TrueTypeFont {
  TrueTypeFont(this.bytes) : _data = ByteData.sublistView(bytes) {
    final count = _data.getUint16(4);
    for (var i = 0; i < count; i++) {
      final at = 12 + 16 * i;
      final tag = String.fromCharCodes(bytes, at, at + 4);
      _tables[tag] = (offset: _data.getUint32(at + 8), length: _data.getUint32(at + 12));
    }
    final head = _table('head'), hhea = _table('hhea');
    unitsPerEm = _data.getUint16(head + 18);
    box = [for (var i = 0; i < 4; i++) _data.getInt16(head + 36 + 2 * i)];
    _longOffsets = _data.getInt16(head + 50) != 0;
    ascent = _data.getInt16(hhea + 4);
    descent = _data.getInt16(hhea + 6);
    _metrics = _data.getUint16(hhea + 34);
    glyphs = _data.getUint16(_table('maxp') + 4);
    capHeight = ascent;
    if (_tables['OS/2'] case final os2? when os2.length >= 90 && _data.getUint16(os2.offset) >= 2) {
      capHeight = _data.getInt16(os2.offset + 88);
    }
    bold = _tables['OS/2'] != null && _data.getUint16(_table('OS/2') + 4) >= 600;
    if (_tables['post'] case final post?) {
      italicAngle = _data.getInt32(post.offset + 4) / 65536;
      underlinePosition = _data.getInt16(post.offset + 8);
      underlineThickness = _data.getInt16(post.offset + 10);
      fixedPitch = _data.getUint32(post.offset + 12) != 0;
    }
    name = _postScriptName() ?? 'Font';
    _readCharacters();
  }

  final Uint8List bytes;
  final ByteData _data;
  final _tables = <String, ({int offset, int length})>{};

  late final int unitsPerEm;

  /// The box of all glyphs: xMin, yMin, xMax, yMax, in font units as every
  /// measure here.
  late final List<int> box;
  late final int ascent;
  late final int descent;
  late int capHeight;
  late final int glyphs;
  late final bool bold;
  double italicAngle = 0;
  int underlinePosition = -100;
  int underlineThickness = 50;
  bool fixedPitch = false;

  /// The name PostScript knows the font by.
  late final String name;

  late final bool _longOffsets;
  late final int _metrics;

  /// The segments of the character map: characters from [start] to [end]
  /// are the glyphs from [glyph] on, or those listed at [ids] in the file.
  final _segments = <({int start, int end, int glyph, int ids})>[];

  int _table(String tag) {
    final table = _tables[tag];
    if (table == null) throw FormatException('font without a $tag table');
    return table.offset;
  }

  String? _postScriptName() {
    final table = _tables['name'];
    if (table == null) return null;
    final at = table.offset;
    final count = _data.getUint16(at + 2), strings = at + _data.getUint16(at + 4);
    for (var i = 0; i < count; i++) {
      final record = at + 6 + 12 * i;
      if (_data.getUint16(record + 6) != 6) continue;
      final platform = _data.getUint16(record);
      final length = _data.getUint16(record + 8), start = strings + _data.getUint16(record + 10);
      final name = StringBuffer();
      for (var j = 0; j < length; j += platform == 1 ? 1 : 2) {
        final c = platform == 1 ? bytes[start + j] : _data.getUint16(start + j);
        // what a PDF name takes without escapes
        if (c > 0x20 && c < 0x7F && !'()<>[]{}/%#'.contains(String.fromCharCode(c))) name.writeCharCode(c);
      }
      if (name.isNotEmpty) return '$name';
    }
    return null;
  }

  void _readCharacters() {
    final cmap = _table('cmap');
    final count = _data.getUint16(cmap + 2);
    int? wide, basic;
    for (var i = 0; i < count; i++) {
      final platform = _data.getUint16(cmap + 4 + 8 * i), encoding = _data.getUint16(cmap + 6 + 8 * i);
      final at = cmap + _data.getUint32(cmap + 8 + 8 * i);
      final format = _data.getUint16(at);
      if (format == 12 && (platform == 3 && encoding == 10 || platform == 0)) wide = at;
      if (format == 4 && (platform == 3 && encoding == 1 || platform == 0)) basic ??= at;
    }
    if (wide != null) {
      final groups = _data.getUint32(wide + 12);
      for (var i = 0; i < groups; i++) {
        final g = wide + 16 + 12 * i;
        _segments.add((start: _data.getUint32(g), end: _data.getUint32(g + 4), glyph: _data.getUint32(g + 8), ids: 0));
      }
    } else if (basic != null) {
      final n = _data.getUint16(basic + 6) ~/ 2;
      final ends = basic + 14, starts = ends + 2 * n + 2, deltas = starts + 2 * n, offsets = deltas + 2 * n;
      for (var i = 0; i < n; i++) {
        final start = _data.getUint16(starts + 2 * i), end = _data.getUint16(ends + 2 * i);
        final offset = _data.getUint16(offsets + 2 * i);
        if (offset == 0) {
          // the delta wraps around sixteen bits: cut where it does
          final first = (start + _data.getUint16(deltas + 2 * i)) & 0xFFFF;
          final before = 0x10000 - first;
          if (before <= end - start) {
            _segments.add((start: start, end: start + before - 1, glyph: first, ids: 0));
            _segments.add((start: start + before, end: end, glyph: 0, ids: 0));
          } else {
            _segments.add((start: start, end: end, glyph: first, ids: 0));
          }
        } else {
          _segments.add((start: start, end: end, glyph: _data.getInt16(deltas + 2 * i), ids: offsets + 2 * i + offset));
        }
      }
    }
    _segments.sort((a, b) => a.start - b.start);
  }

  /// The glyph that draws a character, 0 when the font has none.
  int glyph(int character) {
    var lo = 0, hi = _segments.length - 1;
    while (lo <= hi) {
      final mid = (lo + hi) >> 1;
      final s = _segments[mid];
      if (character < s.start) {
        hi = mid - 1;
      } else if (character > s.end) {
        lo = mid + 1;
      } else {
        if (s.ids == 0) return s.glyph + character - s.start;
        final at = s.ids + 2 * (character - s.start);
        if (at + 2 > bytes.length) return 0;
        final id = _data.getUint16(at);
        return id == 0 ? 0 : (id + s.glyph) & 0xFFFF;
      }
    }
    return 0;
  }

  /// How far a glyph moves the pen.
  int advance(int glyph) {
    final hmtx = _table('hmtx');
    return _data.getUint16(hmtx + 4 * (glyph < _metrics ? glyph : _metrics - 1));
  }

  ({int start, int end}) _outline(int glyph) {
    final loca = _table('loca');
    if (glyph < 0 || glyph >= glyphs) return (start: 0, end: 0);
    return _longOffsets
        ? (start: _data.getUint32(loca + 4 * glyph), end: _data.getUint32(loca + 4 * glyph + 4))
        : (start: 2 * _data.getUint16(loca + 2 * glyph), end: 2 * _data.getUint16(loca + 2 * glyph + 2));
  }

  /// The font with the outlines of [used] alone, and of the glyphs they
  /// are made of. Glyphs keep their numbers: the others are left empty.
  Uint8List subset(Set<int> used) {
    final glyf = _table('glyf');
    final kept = <int>{0};
    final todo = [...used];
    while (todo.isNotEmpty) {
      final g = todo.removeLast();
      if (g < 0 || g >= glyphs || !kept.add(g) && g != 0) continue;
      final o = _outline(g);
      if (o.end - o.start < 10 || _data.getInt16(glyf + o.start) >= 0) continue;
      // a glyph made of others: flags, glyph, then arguments whose size
      // the flags tell
      for (var at = glyf + o.start + 10; at + 4 <= glyf + o.end;) {
        final flags = _data.getUint16(at);
        todo.add(_data.getUint16(at + 2));
        at += 4 + (flags & 0x01 != 0 ? 4 : 2);
        if (flags & 0x08 != 0) at += 2;
        if (flags & 0x40 != 0) at += 4;
        if (flags & 0x80 != 0) at += 8;
        if (flags & 0x20 == 0) break;
      }
    }

    final outlines = BytesBuilder(copy: false);
    final offsets = ByteData(4 * (glyphs + 1));
    for (var g = 0; g < glyphs; g++) {
      offsets.setUint32(4 * g, outlines.length);
      if (!kept.contains(g)) continue;
      final o = _outline(g);
      if (o.end <= o.start || glyf + o.end > bytes.length) continue;
      outlines.add(Uint8List.sublistView(bytes, glyf + o.start, glyf + o.end));
      if (outlines.length % 4 != 0) outlines.add(Uint8List(4 - outlines.length % 4));
    }
    offsets.setUint32(4 * glyphs, outlines.length);

    final head = Uint8List.fromList(_bytesOf('head')!);
    ByteData.sublistView(head)
      ..setUint32(8, 0)
      ..setInt16(50, 1);
    final tables = <String, Uint8List>{
      'head': head,
      'glyf': outlines.takeBytes(),
      'loca': offsets.buffer.asUint8List(),
      for (final tag in ['hhea', 'maxp', 'hmtx', 'cvt ', 'fpgm', 'prep'])
        if (_bytesOf(tag) case final table?) tag: table,
    };
    return _assemble(tables);
  }

  Uint8List? _bytesOf(String tag) {
    final table = _tables[tag];
    return table == null ? null : Uint8List.sublistView(bytes, table.offset, table.offset + table.length);
  }

  static Uint8List _assemble(Map<String, Uint8List> tables) {
    final tags = tables.keys.toList()..sort();
    var selector = 0;
    while (2 << selector <= tags.length) {
      selector++;
    }
    var at = 12 + 16 * tags.length;
    var size = at;
    for (final tag in tags) {
      size += (tables[tag]!.length + 3) & ~3;
    }
    final out = Uint8List(size);
    final data = ByteData.sublistView(out)
      ..setUint32(0, 0x00010000)
      ..setUint16(4, tags.length)
      ..setUint16(6, 16 << selector)
      ..setUint16(8, selector)
      ..setUint16(10, 16 * tags.length - (16 << selector));
    var headAt = 0;
    for (final (i, tag) in tags.indexed) {
      final table = tables[tag]!;
      out.setRange(at, at + table.length, table);
      final record = 12 + 16 * i;
      out.setRange(record, record + 4, tag.codeUnits);
      data
        ..setUint32(record + 4, _checksum(data, at, table.length))
        ..setUint32(record + 8, at)
        ..setUint32(record + 12, table.length);
      if (tag == 'head') headAt = at;
      at += (table.length + 3) & ~3;
    }
    data.setUint32(headAt + 8, (0xB1B0AFBA - _checksum(data, 0, size)) & 0xFFFFFFFF);
    return out;
  }

  /// The sum of the words of a table, which it ends with zeros to fill.
  static int _checksum(ByteData data, int at, int length) {
    var sum = 0;
    for (var i = 0; i < length; i += 4) {
      sum = (sum + data.getUint32(at + i)) & 0xFFFFFFFF;
    }
    return sum;
  }
}
