import 'dart:async';

import 'package:loffice/loffice.dart';

/// The tests draw with the font of the test framework, unless one loads the
/// fonts itself (fonts.dart).
Future<void> testExecutable(FutureOr<void> Function() testMain) async {
  OfficeFonts.skip();
  await testMain();
}
