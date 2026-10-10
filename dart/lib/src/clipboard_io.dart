/// Null: only a browser tells its page what is pasted; elsewhere the
/// clipboard is read when the key is pressed.
void Function()? watchPaste(bool Function(String text) onPaste) => null;
