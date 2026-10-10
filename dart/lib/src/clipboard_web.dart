import 'dart:js_interop';

/// Listens to the text the page is given to paste, until the function it
/// returns is called. [onPaste] says whether it took the text: the browser
/// then pastes nothing itself, and nobody else on the page hears of it.
void Function()? watchPaste(bool Function(String text) onPaste) {
  final listener = (_ClipboardEvent event) {
    final text = event.clipboardData?.getData('text/plain') ?? '';
    if (text.isEmpty || !onPaste(text)) return;
    event.preventDefault();
    event.stopPropagation();
  }.toJS;
  _document.addEventListener('paste', listener, true);
  return () => _document.removeEventListener('paste', listener, true);
}

@JS('document')
external _Document get _document;

extension type _Document._(JSObject _) implements JSObject {
  external void addEventListener(String type, JSFunction listener, bool capture);

  external void removeEventListener(String type, JSFunction listener, bool capture);
}

extension type _ClipboardEvent._(JSObject _) implements JSObject {
  external _DataTransfer? get clipboardData;

  external void preventDefault();

  external void stopPropagation();
}

extension type _DataTransfer._(JSObject _) implements JSObject {
  external String getData(String format);
}
