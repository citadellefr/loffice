import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/drawing/color.dart';

String _hex(Color? c) => c!.toARGB32().toRadixString(16).padLeft(8, '0').toUpperCase();

/// Office rounds in HSL on a 0 to 255 scale: a channel may be one off.
Matcher _near(String hex) => predicate<Color?>((c) {
  final want = int.parse(hex, radix: 16);
  final got = c!.toARGB32();
  return [0, 8, 16, 24].every((s) => (((got >> s) & 0xFF) - ((want >> s) & 0xFF)).abs() <= 1);
}, 'within one of $hex');

void main() {
  const office = ColorContext(scheme: {
    'dk1': Color(0xFF000000),
    'lt1': Color(0xFFFFFFFF),
    'dk2': Color(0xFF44546A),
    'lt2': Color(0xFFE7E6E6),
    'accent1': Color(0xFF4472C4),
  });

  test('resolves the theme through the color map', () {
    expect(_hex(office.resolve({'scheme': 'tx1'})), 'FF000000');
    expect(_hex(office.resolve({'scheme': 'bg1'})), 'FFFFFFFF');
    const dark = ColorContext(scheme: {'dk1': Color(0xFF000000), 'lt1': Color(0xFFFFFFFF)}, map: {'bg1': 'dk1', 'tx1': 'lt1'});
    expect(_hex(dark.resolve({'scheme': 'bg1'})), 'FF000000');
    expect(office.resolve({'scheme': 'accent6'}), isNull);
    expect(_hex(office.withPlaceholder(const Color(0xFF123456)).resolve({'scheme': 'phClr'})), 'FF123456');
  });

  test('transforms colors as Office does', () {
    // the shades of the palette Office shows for Accent 1
    expect(office.resolve({'scheme': 'accent1', 'mods': [['lumMod', 75000]]}), _near('FF2F5597'));
    expect(office.resolve({'scheme': 'accent1', 'mods': [['lumMod', 20000], ['lumOff', 80000]]}), _near('FFDAE3F3'));
    expect(office.resolve({'scheme': 'accent1', 'mods': [['lumMod', 60000], ['lumOff', 40000]]}), _near('FF8FAADC'));
    expect(_hex(office.resolve({'rgb': 'FF0000', 'mods': [['alpha', 50000]]})), '80FF0000');
    expect(_hex(office.resolve({'rgb': '808080', 'mods': [['inv']]})), 'FF7F7F7F');
    expect(_hex(office.resolve({'rgb': 'FFFFFF', 'mods': [['shade', 50000]]})), 'FFBCBCBC');
    expect(_hex(office.resolve({'rgb': '000000', 'mods': [['tint', 50000]]})), 'FFBCBCBC');
  });

  test('knows system and preset colors', () {
    expect(_hex(office.resolve({'sys': 'windowText', 'rgb': '000000'})), 'FF000000');
    expect(_hex(office.resolve({'prst': 'dkBlue'})), 'FF00008B');
    expect(_hex(office.resolve({'prst': 'ltGray'})), 'FFD3D3D3');
    expect(_hex(office.resolve({'prst': 'medAquamarine'})), 'FF66CDAA');
    expect(_hex(office.resolve({'scrgb': [100000, 0, 0]})), 'FFFF0000');
    expect(office.resolve({'rgb': 'nope'}), isNotNull);
  });
}
