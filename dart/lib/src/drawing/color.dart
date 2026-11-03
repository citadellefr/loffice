import 'dart:math' as math;
import 'package:flutter/painting.dart';

/// What colors of a document are resolved against: the theme's scheme, the
/// map from the names text uses (bg1, tx1…) to those of the scheme (lt1,
/// dk1…), and the color a style gives to "phClr".
class ColorContext {
  const ColorContext({this.scheme = const {}, this.map = const {}, this.placeholder});

  /// The colors of the theme, by the names of its scheme: dk1, lt1,
  /// accent1…
  final Map<String, Color> scheme;
  final Map<String, String> map;
  final Color? placeholder;

  ColorContext withPlaceholder(Color? color) => ColorContext(scheme: scheme, map: map, placeholder: color);

  /// A DrawingML color as the Go package writes it, null when it is not
  /// one.
  Color? resolve(Object? json) {
    if (json is! Map<String, Object?>) return null;
    var base = _base(json);
    if (base == null) return null;
    final mods = json['mods'];
    if (mods is List<Object?>) {
      for (final m in mods) {
        if (m is List<Object?> && m.isNotEmpty && m[0] is String) {
          base = _apply(base!, m[0]! as String, m.length > 1 && m[1] is num ? (m[1]! as num).toDouble() : 0);
        }
      }
    }
    return base!.color;
  }

  _Rgba? _base(Map<String, Object?> json) {
    final scheme = json['scheme'];
    if (scheme is String) {
      final c = scheme == 'phClr' ? placeholder : this.scheme[map[scheme] ?? _defaultMap[scheme] ?? scheme];
      return c == null ? null : _Rgba.of(c);
    }
    if (json['sys'] is String || json['rgb'] is String) {
      final rgb = json['rgb'];
      final v = rgb is String ? int.tryParse(rgb, radix: 16) : null;
      if (v != null && rgb!.toString().length == 6) return _Rgba.of(Color(0xFF000000 | v));
      return json['sys'] == 'window' ? _Rgba(1, 1, 1, 1) : _Rgba(0, 0, 0, 1);
    }
    final preset = json['prst'];
    if (preset is String) {
      final v = presetColor(preset);
      return v == null ? null : _Rgba.of(v);
    }
    for (final (key, linear) in [('scrgb', true), ('hsl', false)]) {
      final v = json[key];
      if (v is List<Object?> && v.length == 3 && v.every((x) => x is num)) {
        final n = v.cast<num>();
        if (linear) {
          return _Rgba(_toSrgb(n[0] / 100000), _toSrgb(n[1] / 100000), _toSrgb(n[2] / 100000), 1);
        }
        return _Rgba.fromHsl(n[0] / 60000, n[1] / 100000, n[2] / 100000);
      }
    }
    return null;
  }
}

const _defaultMap = {'bg1': 'lt1', 'tx1': 'dk1', 'bg2': 'lt2', 'tx2': 'dk2'};

/// A color being transformed, in sRGB from 0 to 1.
class _Rgba {
  _Rgba(this.r, this.g, this.b, this.a);

  factory _Rgba.of(Color c) => _Rgba(c.r, c.g, c.b, c.a);

  factory _Rgba.fromHsl(double h, double s, double l, [double a = 1]) {
    final c = HSLColor.fromAHSL(1, (h % 360 + 360) % 360, s.clamp(0, 1), l.clamp(0, 1)).toColor();
    return _Rgba(c.r, c.g, c.b, a);
  }

  double r, g, b, a;

  Color get color => Color.from(alpha: a.clamp(0, 1), red: r.clamp(0, 1), green: g.clamp(0, 1), blue: b.clamp(0, 1));

  HSLColor get hsl => HSLColor.fromColor(Color.from(alpha: 1, red: r.clamp(0, 1), green: g.clamp(0, 1), blue: b.clamp(0, 1)));
}

double _toLinear(double c) => c <= 0.04045 ? c / 12.92 : math.pow((c + 0.055) / 1.055, 2.4).toDouble();

double _toSrgb(double c) => c <= 0.0031308 ? c * 12.92 : 1.055 * math.pow(c, 1 / 2.4) - 0.055;

/// Applies a color transform, as Office does: hue, saturation and
/// luminance in HSL, tint and shade and the channels in linear RGB.
_Rgba _apply(_Rgba c, String name, double v) {
  final f = v / 100000;
  _Rgba hsl(double Function(HSLColor) h, double Function(HSLColor) s, double Function(HSLColor) l) {
    final x = c.hsl;
    return _Rgba.fromHsl(h(x), s(x), l(x), c.a);
  }

  _Rgba linear(double Function(double) f) => _Rgba(_toSrgb(f(_toLinear(c.r))), _toSrgb(f(_toLinear(c.g))), _toSrgb(f(_toLinear(c.b))), c.a);

  _Rgba channel(int i, double Function(double) f) {
    final l = [_toLinear(c.r), _toLinear(c.g), _toLinear(c.b)];
    l[i] = f(l[i]).clamp(0, 1);
    return _Rgba(_toSrgb(l[0]), _toSrgb(l[1]), _toSrgb(l[2]), c.a);
  }

  switch (name) {
    case 'alpha':
      return c..a = f;
    case 'alphaMod':
      return c..a *= f;
    case 'alphaOff':
      return c..a += f;
    case 'hue':
      return hsl((_) => v / 60000, (x) => x.saturation, (x) => x.lightness);
    case 'hueMod':
      return hsl((x) => x.hue * f, (x) => x.saturation, (x) => x.lightness);
    case 'hueOff':
      return hsl((x) => x.hue + v / 60000, (x) => x.saturation, (x) => x.lightness);
    case 'sat':
      return hsl((x) => x.hue, (_) => f, (x) => x.lightness);
    case 'satMod':
      return hsl((x) => x.hue, (x) => x.saturation * f, (x) => x.lightness);
    case 'satOff':
      return hsl((x) => x.hue, (x) => x.saturation + f, (x) => x.lightness);
    case 'lum':
      return hsl((x) => x.hue, (x) => x.saturation, (_) => f);
    case 'lumMod':
      return hsl((x) => x.hue, (x) => x.saturation, (x) => x.lightness * f);
    case 'lumOff':
      return hsl((x) => x.hue, (x) => x.saturation, (x) => x.lightness + f);
    case 'tint':
      return linear((x) => 1 - (1 - x) * f);
    case 'shade':
      return linear((x) => x * f);
    case 'comp':
      return hsl((x) => x.hue + 180, (x) => x.saturation, (x) => x.lightness);
    case 'inv':
      return _Rgba(1 - c.r, 1 - c.g, 1 - c.b, c.a);
    case 'gray':
      final y = 0.2126 * c.r + 0.7152 * c.g + 0.0722 * c.b;
      return _Rgba(y, y, y, c.a);
    case 'gamma':
      return _Rgba(_toSrgb(c.r), _toSrgb(c.g), _toSrgb(c.b), c.a);
    case 'invGamma':
      return _Rgba(_toLinear(c.r), _toLinear(c.g), _toLinear(c.b), c.a);
    case 'red':
      return channel(0, (_) => f);
    case 'redMod':
      return channel(0, (x) => x * f);
    case 'redOff':
      return channel(0, (x) => x + f);
    case 'green':
      return channel(1, (_) => f);
    case 'greenMod':
      return channel(1, (x) => x * f);
    case 'greenOff':
      return channel(1, (x) => x + f);
    case 'blue':
      return channel(2, (_) => f);
    case 'blueMod':
      return channel(2, (x) => x * f);
    case 'blueOff':
      return channel(2, (x) => x + f);
  }
  return c;
}

/// A preset color of DrawingML: the CSS color names, "dk", "lt" and "med"
/// spelling out "dark", "light" and "medium".
Color? presetColor(String name) {
  var n = name.toLowerCase();
  for (final (short, long) in [('dk', 'dark'), ('lt', 'light'), ('med', 'medium')]) {
    if (n.startsWith(short) && !n.startsWith(long)) {
      n = long + n.substring(short.length);
      break;
    }
  }
  final v = _cssColors[n];
  return v == null ? null : Color(0xFF000000 | v);
}

const _cssColors = {
  'aliceblue': 0xF0F8FF,
  'antiquewhite': 0xFAEBD7,
  'aqua': 0x00FFFF,
  'aquamarine': 0x7FFFD4,
  'azure': 0xF0FFFF,
  'beige': 0xF5F5DC,
  'bisque': 0xFFE4C4,
  'black': 0x000000,
  'blanchedalmond': 0xFFEBCD,
  'blue': 0x0000FF,
  'blueviolet': 0x8A2BE2,
  'brown': 0xA52A2A,
  'burlywood': 0xDEB887,
  'cadetblue': 0x5F9EA0,
  'chartreuse': 0x7FFF00,
  'chocolate': 0xD2691E,
  'coral': 0xFF7F50,
  'cornflowerblue': 0x6495ED,
  'cornsilk': 0xFFF8DC,
  'crimson': 0xDC143C,
  'cyan': 0x00FFFF,
  'darkblue': 0x00008B,
  'darkcyan': 0x008B8B,
  'darkgoldenrod': 0xB8860B,
  'darkgray': 0xA9A9A9,
  'darkgrey': 0xA9A9A9,
  'darkgreen': 0x006400,
  'darkkhaki': 0xBDB76B,
  'darkmagenta': 0x8B008B,
  'darkolivegreen': 0x556B2F,
  'darkorange': 0xFF8C00,
  'darkorchid': 0x9932CC,
  'darkred': 0x8B0000,
  'darksalmon': 0xE9967A,
  'darkseagreen': 0x8FBC8F,
  'darkslateblue': 0x483D8B,
  'darkslategray': 0x2F4F4F,
  'darkslategrey': 0x2F4F4F,
  'darkturquoise': 0x00CED1,
  'darkviolet': 0x9400D3,
  'deeppink': 0xFF1493,
  'deepskyblue': 0x00BFFF,
  'dimgray': 0x696969,
  'dimgrey': 0x696969,
  'dodgerblue': 0x1E90FF,
  'firebrick': 0xB22222,
  'floralwhite': 0xFFFAF0,
  'forestgreen': 0x228B22,
  'fuchsia': 0xFF00FF,
  'gainsboro': 0xDCDCDC,
  'ghostwhite': 0xF8F8FF,
  'gold': 0xFFD700,
  'goldenrod': 0xDAA520,
  'gray': 0x808080,
  'grey': 0x808080,
  'green': 0x008000,
  'greenyellow': 0xADFF2F,
  'honeydew': 0xF0FFF0,
  'hotpink': 0xFF69B4,
  'indianred': 0xCD5C5C,
  'indigo': 0x4B0082,
  'ivory': 0xFFFFF0,
  'khaki': 0xF0E68C,
  'lavender': 0xE6E6FA,
  'lavenderblush': 0xFFF0F5,
  'lawngreen': 0x7CFC00,
  'lemonchiffon': 0xFFFACD,
  'lightblue': 0xADD8E6,
  'lightcoral': 0xF08080,
  'lightcyan': 0xE0FFFF,
  'lightgoldenrodyellow': 0xFAFAD2,
  'lightgray': 0xD3D3D3,
  'lightgrey': 0xD3D3D3,
  'lightgreen': 0x90EE90,
  'lightpink': 0xFFB6C1,
  'lightsalmon': 0xFFA07A,
  'lightseagreen': 0x20B2AA,
  'lightskyblue': 0x87CEFA,
  'lightslategray': 0x778899,
  'lightslategrey': 0x778899,
  'lightsteelblue': 0xB0C4DE,
  'lightyellow': 0xFFFFE0,
  'lime': 0x00FF00,
  'limegreen': 0x32CD32,
  'linen': 0xFAF0E6,
  'magenta': 0xFF00FF,
  'maroon': 0x800000,
  'mediumaquamarine': 0x66CDAA,
  'mediumblue': 0x0000CD,
  'mediumorchid': 0xBA55D3,
  'mediumpurple': 0x9370DB,
  'mediumseagreen': 0x3CB371,
  'mediumslateblue': 0x7B68EE,
  'mediumspringgreen': 0x00FA9A,
  'mediumturquoise': 0x48D1CC,
  'mediumvioletred': 0xC71585,
  'midnightblue': 0x191970,
  'mintcream': 0xF5FFFA,
  'mistyrose': 0xFFE4E1,
  'moccasin': 0xFFE4B5,
  'navajowhite': 0xFFDEAD,
  'navy': 0x000080,
  'oldlace': 0xFDF5E6,
  'olive': 0x808000,
  'olivedrab': 0x6B8E23,
  'orange': 0xFFA500,
  'orangered': 0xFF4500,
  'orchid': 0xDA70D6,
  'palegoldenrod': 0xEEE8AA,
  'palegreen': 0x98FB98,
  'paleturquoise': 0xAFEEEE,
  'palevioletred': 0xDB7093,
  'papayawhip': 0xFFEFD5,
  'peachpuff': 0xFFDAB9,
  'peru': 0xCD853F,
  'pink': 0xFFC0CB,
  'plum': 0xDDA0DD,
  'powderblue': 0xB0E0E6,
  'purple': 0x800080,
  'red': 0xFF0000,
  'rosybrown': 0xBC8F8F,
  'royalblue': 0x4169E1,
  'saddlebrown': 0x8B4513,
  'salmon': 0xFA8072,
  'sandybrown': 0xF4A460,
  'seagreen': 0x2E8B57,
  'seashell': 0xFFF5EE,
  'sienna': 0xA0522D,
  'silver': 0xC0C0C0,
  'skyblue': 0x87CEEB,
  'slateblue': 0x6A5ACD,
  'slategray': 0x708090,
  'slategrey': 0x708090,
  'snow': 0xFFFAFA,
  'springgreen': 0x00FF7F,
  'steelblue': 0x4682B4,
  'tan': 0xD2B48C,
  'teal': 0x008080,
  'thistle': 0xD8BFD8,
  'tomato': 0xFF6347,
  'turquoise': 0x40E0D0,
  'violet': 0xEE82EE,
  'wheat': 0xF5DEB3,
  'white': 0xFFFFFF,
  'whitesmoke': 0xF5F5F5,
  'yellow': 0xFFFF00,
  'yellowgreen': 0x9ACD32,
};
