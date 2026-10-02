import 'dart:io';

import 'package:flutter/services.dart';

/// Loads the fonts standing in for Office's found in \$LOFFICE_FONTS, for the
/// tests that draw documents.
Future<void> loadFonts() async {
  final fonts = <String, List<String>>{};
  for (final root in (Platform.environment['LOFFICE_FONTS'] ?? '').split(':').where((d) => d.isNotEmpty)) {
    for (final f in Directory(root).listSync(recursive: true).whereType<File>()) {
      final name = f.uri.pathSegments.last;
      for (final family in ['Carlito', 'Caladea', 'LiberationSans', 'LiberationSerif', 'LiberationMono']) {
        if (name.startsWith('$family-') && name.endsWith('.ttf')) (fonts[family] ??= []).add(f.path);
      }
    }
  }
  for (final e in fonts.entries) {
    final family = e.key.replaceFirst('Liberation', 'Liberation ');
    final loader = FontLoader(family);
    for (final path in e.value) {
      loader.addFont(Future.value(ByteData.sublistView(File(path).readAsBytesSync())));
    }
    await loader.load();
  }
}
