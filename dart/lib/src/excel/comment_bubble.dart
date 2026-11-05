import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../chrome/strings.dart';
import 'sheet_comments.dart';

/// What Excel shows when the pointer rests on a cell with a note or a
/// thread: the note on pale yellow, its author in bold; the thread on
/// white, each comment with its author and date.
class CommentBubble extends StatelessWidget {
  const CommentBubble({super.key, required this.thread, this.strings = const LofficeStrings()});

  final CellThread thread;
  final LofficeStrings strings;

  @override
  Widget build(BuildContext context) {
    const ink = TextStyle(fontSize: 12, color: Colors.black, fontFamily: 'Carlito', height: 1.25);
    final bold = ink.copyWith(fontWeight: FontWeight.bold);
    final small = ink.copyWith(fontSize: 10.5, color: const Color(0xFF616161));
    return IgnorePointer(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 260),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: thread.note ? const Color(0xFFFFFFE1) : Colors.white,
            border: Border.all(color: thread.note ? Colors.black87 : const Color(0xFFBDBDBD)),
            boxShadow: const [BoxShadow(color: Color(0x33000000), blurRadius: 4, offset: Offset(1, 2))],
          ),
          child: Padding(
            padding: const EdgeInsets.all(6),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (final (i, c) in thread.all.indexed) ...[
                  if (i > 0) const SizedBox(height: 6),
                  Text(c.author.isEmpty ? strings.unknownAuthor : c.author, style: bold),
                  if (c.date != null) Text(strings.commentDate(c.date!), style: small),
                  Text(c.text, style: ink),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// The note of a cell as Excel lets it be written in place: its bubble
/// holds the text, kept when the focus leaves it and dropped on Escape.
class NoteEditor extends StatefulWidget {
  const NoteEditor({super.key, required this.thread, required this.onSave, required this.onCancel, this.strings = const LofficeStrings()});

  final CellThread thread;
  final ValueChanged<String> onSave;
  final VoidCallback onCancel;
  final LofficeStrings strings;

  @override
  State<NoteEditor> createState() => _NoteEditorState();
}

class _NoteEditorState extends State<NoteEditor> {
  late final _text = TextEditingController(text: widget.thread.root.text);

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  void _save() {
    final text = _text.text.trim();
    if (text.isEmpty || text == widget.thread.root.text) {
      widget.onCancel();
    } else {
      widget.onSave(text);
    }
  }

  @override
  Widget build(BuildContext context) {
    const ink = TextStyle(fontSize: 12, color: Colors.black, fontFamily: 'Carlito', height: 1.25);
    final author = widget.thread.root.author;
    return TapRegion(
      onTapOutside: (_) => _save(),
      child: CallbackShortcuts(
        bindings: {const SingleActivator(LogicalKeyboardKey.escape): widget.onCancel},
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 260),
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: const Color(0xFFFFFFE1),
              border: Border.all(color: Colors.black87),
              boxShadow: const [BoxShadow(color: Color(0x33000000), blurRadius: 4, offset: Offset(1, 2))],
            ),
            child: Padding(
              padding: const EdgeInsets.all(6),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(author.isEmpty ? widget.strings.unknownAuthor : author, style: ink.copyWith(fontWeight: FontWeight.bold)),
                  TextField(
                    controller: _text,
                    autofocus: true,
                    minLines: 2,
                    maxLines: 8,
                    keyboardType: TextInputType.multiline,
                    style: ink,
                    decoration: const InputDecoration.collapsed(hintText: ''),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
