import 'dart:convert';
import 'dart:typed_data';

import 'deflate.dart';
import 'truetype.dart';

/// A number as a PDF writes it: three decimals at most, no exponent.
String pdfNumber(double v) {
  if (!v.isFinite) return '0';
  final s = v.toStringAsFixed(3);
  var end = s.length;
  while (s.codeUnitAt(end - 1) == 0x30) {
    end--;
  }
  if (s.codeUnitAt(end - 1) == 0x2E) end--;
  final out = s.substring(0, end);
  return out == '-0' ? '0' : out;
}

/// The name of the PDF of a document: its own, with the extension changed.
String pdfName(String title) {
  final dot = title.lastIndexOf('.');
  final base = dot > 0 ? title.substring(0, dot) : title;
  return '${base.isEmpty ? 'document' : base}.pdf';
}

/// A PDF being written: pages of drawing operators, and what they draw
/// with — fonts, pictures, opacities and gradients — shared by all of them.
class PdfDocument {
  final _objects = <Uint8List?>[];
  final _pages = <({double width, double height, int content})>[];
  final _fonts = <TrueTypeFont, PdfFont>{};
  final _images = <int>[];
  final _alphas = <String, String>{};
  final _shadings = <int>[];
  late final int _resources = _reserve();
  late final int _pagesId = _reserve();

  int _reserve() {
    _objects.add(null);
    return _objects.length;
  }

  void _set(int id, String dictionary, [Uint8List? stream]) {
    final out = BytesBuilder(copy: false)..add(latin1.encode('$id 0 obj\n'));
    if (stream == null) {
      out.add(latin1.encode('$dictionary\nendobj\n'));
    } else {
      out
        ..add(latin1.encode('<< $dictionary /Length ${stream.length} >>\nstream\n'))
        ..add(stream)
        ..add(latin1.encode('\nendstream\nendobj\n'));
    }
    _objects[id - 1] = out.takeBytes();
  }

  int _add(String dictionary, [Uint8List? stream]) {
    final id = _reserve();
    _set(id, dictionary, stream);
    return id;
  }

  int _packed(String dictionary, Uint8List data) => _add('$dictionary /Filter /FlateDecode', deflate(data));

  /// Adds a page, in points, drawn by [content]. Its origin is its top
  /// left corner, as on a screen.
  void addPage(double width, double height, String content) {
    final flipped = '1 0 0 -1 0 ${pdfNumber(height)} cm\n$content';
    _pages.add((width: width, height: height, content: _packed('', latin1.encode(flipped))));
  }

  /// The font of a file, embedded once however many pages write with it.
  PdfFont font(TrueTypeFont font) => _fonts[font] ??= PdfFont._('F${_fonts.length + 1}', font);

  /// A picture of [width] by [height] pixels, three bytes each, with its
  /// opacity when it is not opaque. It is drawn in the unit square.
  String image(int width, int height, Uint8List rgb, [Uint8List? alpha]) {
    var mask = '';
    if (alpha != null) {
      final id = _packed('/Type /XObject /Subtype /Image /Width $width /Height $height /ColorSpace /DeviceGray /BitsPerComponent 8', alpha);
      mask = ' /SMask $id 0 R';
    }
    _images.add(_packed('/Type /XObject /Subtype /Image /Width $width /Height $height /ColorSpace /DeviceRGB /BitsPerComponent 8$mask', rgb));
    return 'Im${_images.length}';
  }

  /// A JPEG picture, embedded as it is.
  String jpeg(int width, int height, int components, Uint8List file) {
    final space = switch (components) {
      1 => '/DeviceGray',
      4 => '/DeviceCMYK /Decode [1 0 1 0 1 0 1 0]',
      _ => '/DeviceRGB',
    };
    _images.add(_add('/Type /XObject /Subtype /Image /Width $width /Height $height /ColorSpace $space /BitsPerComponent 8 /Filter /DCTDecode', file));
    return 'Im${_images.length}';
  }

  /// The graphics state that fills and strokes at an opacity.
  String alpha(double fill, double stroke) => _alphas.putIfAbsent(
    '<< /ca ${pdfNumber(fill)} /CA ${pdfNumber(stroke)} >>',
    () => 'G${_alphas.length + 1}',
  );

  /// A gradient through [colors] (RGB, 0 to 1) at [stops]: along the line
  /// of [coords] (x0 y0 x1 y1), or between the two circles of six numbers.
  String shading(List<double> coords, List<List<double>> colors, List<double> stops) {
    String part(int i) =>
        '<< /FunctionType 2 /Domain [0 1] /C0 [${colors[i].map(pdfNumber).join(' ')}] /C1 [${colors[i + 1].map(pdfNumber).join(' ')}] /N 1 >>';
    var function = part(0);
    if (colors.length > 2) {
      final bounds = [for (var i = 1; i < stops.length - 1; i++) pdfNumber(stops[i])].join(' ');
      final parts = [for (var i = 0; i < colors.length - 1; i++) part(i)].join(' ');
      final encode = [for (var i = 0; i < colors.length - 1; i++) '0 1'].join(' ');
      function = '<< /FunctionType 3 /Domain [0 1] /Functions [$parts] /Bounds [$bounds] /Encode [$encode] >>';
    }
    _shadings.add(_add(
      '<< /ShadingType ${coords.length == 4 ? 2 : 3} /ColorSpace /DeviceRGB /Coords [${coords.map(pdfNumber).join(' ')}] '
      '/Function $function /Extend [true true] >>',
    ));
    return 'Sh${_shadings.length}';
  }

  /// The file, titled [title].
  Uint8List save({String title = ''}) {
    final fonts = [for (final f in _fonts.values) '/${f.name} ${_embed(f)} 0 R'].join(' ');
    final images = [for (final (i, id) in _images.indexed) '/Im${i + 1} $id 0 R'].join(' ');
    final shadings = [for (final (i, id) in _shadings.indexed) '/Sh${i + 1} $id 0 R'].join(' ');
    final states = [for (final e in _alphas.entries) '/${e.value} ${e.key}'].join(' ');
    _set(_resources, '<< /Font << $fonts >> /XObject << $images >> /Shading << $shadings >> /ExtGState << $states >> >>');
    final kids = [
      for (final p in _pages)
        _add(
          '<< /Type /Page /Parent $_pagesId 0 R /MediaBox [0 0 ${pdfNumber(p.width)} ${pdfNumber(p.height)}] '
          '/Resources $_resources 0 R /Contents ${p.content} 0 R >>',
        ),
    ];
    _set(_pagesId, '<< /Type /Pages /Count ${kids.length} /Kids [${kids.map((k) => '$k 0 R').join(' ')}] >>');
    final catalog = _add('<< /Type /Catalog /Pages $_pagesId 0 R >>');
    final info = _add('<< /Title ${_text(title)} /Producer ${_text("L'Office")} >>');

    final out = BytesBuilder(copy: false)..add(latin1.encode('%PDF-1.7\n%\xE2\xE3\xCF\xD3\n'));
    final offsets = <int>[];
    for (final object in _objects) {
      offsets.add(out.length);
      out.add(object!);
    }
    final xref = out.length;
    final table = StringBuffer('xref\n0 ${_objects.length + 1}\n0000000000 65535 f \n');
    for (final offset in offsets) {
      table.write('${'$offset'.padLeft(10, '0')} 00000 n \n');
    }
    table.write('trailer\n<< /Size ${_objects.length + 1} /Root $catalog 0 R /Info $info 0 R >>\nstartxref\n$xref\n%%EOF\n');
    out.add(latin1.encode('$table'));
    return out.takeBytes();
  }

  /// A string of text, in UTF-16 as hexadecimal.
  static String _text(String s) {
    final hex = StringBuffer('<FEFF');
    for (final unit in s.codeUnits) {
      hex.write(unit.toRadixString(16).padLeft(4, '0'));
    }
    return '$hex>';
  }

  /// Writes a font with the glyphs it was asked for: the file, its
  /// measures, the widths of its glyphs and the characters they stand for,
  /// which is what makes the text of the page text.
  int _embed(PdfFont f) {
    final font = f.font;
    double em(num v) => v * 1000 / font.unitsPerEm;
    final glyphs = f._used.keys.toList()..sort();
    final file = font.subset(glyphs.toSet());
    final fileId = _packed('/Length1 ${file.length}', file);
    // six letters before the name tell a reader the font is a subset
    final tag = StringBuffer();
    for (var i = 0, seed = glyphs.fold(glyphs.length, (a, g) => (a * 31 + g) & 0xFFFFFF); i < 6; i++, seed ~/= 26) {
      tag.writeCharCode(0x41 + seed % 26);
    }
    final name = '$tag+${font.name}';
    final flags = 32 | (font.fixedPitch ? 1 : 0) | (font.italicAngle != 0 ? 64 : 0);
    final descriptor = _add(
      '<< /Type /FontDescriptor /FontName /$name /Flags $flags '
      '/FontBBox [${font.box.map((v) => pdfNumber(em(v))).join(' ')}] /ItalicAngle ${pdfNumber(font.italicAngle)} '
      '/Ascent ${pdfNumber(em(font.ascent))} /Descent ${pdfNumber(em(font.descent))} /CapHeight ${pdfNumber(em(font.capHeight))} '
      '/StemV ${font.bold ? 120 : 80} /FontFile2 $fileId 0 R >>',
    );
    final widths = [for (final g in glyphs) '$g [${pdfNumber(em(font.advance(g)))}]'].join(' ');
    final descendant = _add(
      '<< /Type /Font /Subtype /CIDFontType2 /BaseFont /$name /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> '
      '/FontDescriptor $descriptor 0 R /DW 1000 /W [$widths] /CIDToGIDMap /Identity >>',
    );
    final map = StringBuffer(
      '/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n'
      '/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n'
      '/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n',
    );
    for (var i = 0; i < glyphs.length; i += 100) {
      final chunk = glyphs.sublist(i, i + 100 > glyphs.length ? glyphs.length : i + 100);
      map.write('${chunk.length} beginbfchar\n');
      for (final g in chunk) {
        final units = String.fromCharCode(f._used[g]!).codeUnits;
        map.write('<${_hex4(g)}> <${units.map(_hex4).join()}>\n');
      }
      map.write('endbfchar\n');
    }
    map.write('endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n');
    final unicode = _packed('', latin1.encode('$map'));
    return _add(
      '<< /Type /Font /Subtype /Type0 /BaseFont /$name /Encoding /Identity-H '
      '/DescendantFonts [$descendant 0 R] /ToUnicode $unicode 0 R >>',
    );
  }
}

String _hex4(int v) => v.toRadixString(16).padLeft(4, '0').toUpperCase();

/// A font of a [PdfDocument], which remembers the glyphs written with it.
class PdfFont {
  PdfFont._(this.name, this.font);

  /// The name the pages select it by.
  final String name;
  final TrueTypeFont font;

  /// The character each glyph used stands for.
  final _used = <int, int>{};

  /// The glyph of a character, as the page writes it; null when the font
  /// has none.
  String? glyph(int character) {
    final g = font.glyph(character);
    if (g == 0) return null;
    _used.putIfAbsent(g, () => character);
    return _hex4(g);
  }

  /// How far the glyph of a character moves the pen, at a size of 1.
  double advance(int character) => font.advance(font.glyph(character)) / font.unitsPerEm;
}
