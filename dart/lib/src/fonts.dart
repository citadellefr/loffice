import 'package:flutter/services.dart';

/// The free fonts the package ships to stand in for Office's (see
/// `Fonts.defaultSubstitutes`): loaded when an editor first opens rather
/// than with the app, and kept for the documents exported with them.
abstract final class OfficeFonts {
  /// The families, with the names of their files.
  static const families = {
    'Carlito': 'Carlito',
    'Caladea': 'Caladea',
    'Liberation Sans': 'LiberationSans',
    'Liberation Serif': 'LiberationSerif',
    'Liberation Mono': 'LiberationMono',
  };

  static const _styles = ['Regular', 'Bold', 'Italic', 'BoldItalic'];

  static final _files = <String, Uint8List>{};
  static Future<void>? _loading;
  static bool _settled = false;

  /// Whether the fonts are there, or known not to come: text is not laid
  /// out before, it would move once they arrive.
  static bool get settled => _settled;

  /// Why the fonts could not be loaded, null when they were.
  static Object? failure;

  /// Loads the fonts from the package, once. It never fails: without them
  /// text is drawn with what the platform has, and [failure] says why.
  static Future<void> load() => _loading ??= _load();

  static Future<void> _load() async {
    if (_settled) return;
    try {
      final files = <String, Uint8List>{};
      await Future.wait([
        for (final file in families.values)
          for (final style in _styles)
            rootBundle.load('packages/loffice/fonts/$file-$style.ttf').then((data) {
              files['$file-$style'] = Uint8List.sublistView(data);
            }),
      ]);
      await register(files);
    } on Object catch (error) {
      failure = error;
      _settled = true;
    }
  }

  /// Takes the fonts from elsewhere than the package: [files] by the name of
  /// their file without its extension ("Carlito-BoldItalic").
  static Future<void> register(Map<String, Uint8List> files) async {
    for (final family in families.entries) {
      final loader = FontLoader(family.key);
      for (final style in _styles) {
        final file = files['${family.value}-$style'];
        if (file != null) loader.addFont(Future.value(ByteData.sublistView(file)));
      }
      await loader.load();
    }
    _files.addAll(files);
    failure = null;
    _settled = true;
  }

  /// Does without the fonts: text is drawn with what the platform has.
  static void skip() => _settled = true;

  /// The file of a face of [family], null when it is not one of the fonts
  /// or they are not loaded.
  static Uint8List? file(String family, {bool bold = false, bool italic = false}) {
    final name = families[family];
    if (name == null) return null;
    final style = bold ? (italic ? 'BoldItalic' : 'Bold') : (italic ? 'Italic' : 'Regular');
    return _files['$name-$style'];
  }
}
