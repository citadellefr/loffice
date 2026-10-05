import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../chrome/strings.dart';
import 'comments.dart';

/// The comments of a document as Word lists them beside its pages: a card
/// by thread, in the order of the text, answered, resolved, edited and
/// deleted there; and the comment being written.
class CommentsPane<T extends CommentThread> extends StatefulWidget {
  const CommentsPane({
    super.key,
    required this.threads,
    required this.active,
    required this.me,
    required this.readOnly,
    required this.drafting,
    required this.onSelect,
    required this.onPost,
    required this.onCancelDraft,
    required this.onReply,
    required this.onResolve,
    required this.onDelete,
    required this.onEdit,
    required this.onClose,
    this.resolvable = true,
    this.emptyHint,
    this.strings = const LofficeStrings(),
  });

  final List<T> threads;

  /// The thread whose card is open: the one of the caret, or clicked.
  final String? active;

  /// The name of the user, whose comments they may edit.
  final String me;
  final bool readOnly;

  /// Whether a new comment is being written.
  final bool drafting;
  final ValueChanged<T> onSelect;
  final ValueChanged<String> onPost;
  final VoidCallback onCancelDraft;
  final void Function(T thread, String text) onReply;
  final void Function(T thread, bool done) onResolve;

  /// Deletes a comment, or its whole thread when it is the first.
  final ValueChanged<DocComment> onDelete;
  final void Function(DocComment comment, String text) onEdit;
  final VoidCallback onClose;

  /// Whether threads can be resolved, which not every format keeps.
  final bool resolvable;

  /// What an empty pane says to do, by default what Word says.
  final String? emptyHint;
  final LofficeStrings strings;

  @override
  State<CommentsPane<T>> createState() => _CommentsPaneState<T>();
}

class _CommentsPaneState<T extends CommentThread> extends State<CommentsPane<T>> {
  final _draft = TextEditingController();
  final _reply = TextEditingController();
  final _editing = TextEditingController();
  final _draftFocus = FocusNode();
  final _keys = <String, GlobalKey>{};
  String? _edited;

  LofficeStrings get _s => widget.strings;

  @override
  void initState() {
    super.initState();
    if (widget.drafting) _draftFocus.requestFocus();
  }

  @override
  void didUpdateWidget(CommentsPane<T> old) {
    super.didUpdateWidget(old);
    if (widget.drafting && !old.drafting) {
      _draft.clear();
      _draftFocus.requestFocus();
    }
    if (widget.active != old.active) {
      _reply.clear();
      final key = _keys[widget.active];
      WidgetsBinding.instance.addPostFrameCallback((_) {
        final context = key?.currentContext;
        if (context != null && context.mounted) Scrollable.ensureVisible(context, duration: const Duration(milliseconds: 200), alignmentPolicy: ScrollPositionAlignmentPolicy.keepVisibleAtEnd);
      });
    }
  }

  @override
  void dispose() {
    _draft.dispose();
    _reply.dispose();
    _editing.dispose();
    _draftFocus.dispose();
    super.dispose();
  }

  void _post() {
    final text = _draft.text.trim();
    if (text.isEmpty) return;
    widget.onPost(text);
    _draft.clear();
  }

  void _sendReply(T thread) {
    final text = _reply.text.trim();
    if (text.isEmpty) return;
    widget.onReply(thread, text);
    _reply.clear();
  }

  void _startEditing(DocComment c) => setState(() {
    _edited = c.id;
    _editing.text = c.text;
  });

  void _saveEditing(DocComment c) {
    final text = _editing.text.trim();
    if (text.isNotEmpty && text != c.text) widget.onEdit(c, text);
    setState(() => _edited = null);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final threads = widget.threads;
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 4, 8),
            child: Row(children: [
              Icon(Icons.forum_outlined, size: 18, color: theme.colorScheme.primary),
              const SizedBox(width: 8),
              Expanded(child: Text(_s.comments, style: theme.textTheme.titleSmall)),
              if (threads.isNotEmpty) Text('${threads.length}', style: theme.textTheme.labelMedium),
              IconButton(tooltip: _s.close, iconSize: 18, onPressed: widget.onClose, icon: const Icon(Icons.close)),
            ]),
          ),
          const Divider(height: 1),
          Expanded(
            child: threads.isEmpty && !widget.drafting
                ? _empty(theme)
                : ListView(
                    padding: const EdgeInsets.all(12),
                    children: [
                      if (widget.drafting) _draftCard(theme),
                      for (final t in threads) _threadCard(theme, t),
                    ],
                  ),
          ),
        ],
      ),
    );
  }

  Widget _empty(ThemeData theme) => Center(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        Icon(Icons.chat_bubble_outline, size: 36, color: theme.colorScheme.outline),
        const SizedBox(height: 12),
        Text(_s.noComments, style: theme.textTheme.titleSmall),
        const SizedBox(height: 6),
        if (!widget.readOnly) Text(widget.emptyHint ?? _s.noCommentsHint, textAlign: TextAlign.center, style: theme.textTheme.bodySmall),
      ]),
    ),
  );

  Widget _card(ThemeData theme, {required bool active, required Widget child, VoidCallback? onTap, Key? key, bool dim = false}) {
    final scheme = theme.colorScheme;
    return Padding(
      key: key,
      padding: const EdgeInsets.only(bottom: 10),
      child: Opacity(
        opacity: dim ? 0.7 : 1,
        child: Material(
          color: scheme.surface,
          elevation: active ? 3 : 0,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(8),
            side: BorderSide(color: active ? scheme.primary : scheme.outlineVariant, width: active ? 1.5 : 1),
          ),
          clipBehavior: Clip.antiAlias,
          child: InkWell(onTap: onTap, child: Padding(padding: const EdgeInsets.all(12), child: child)),
        ),
      ),
    );
  }

  Widget _draftCard(ThemeData theme) => _card(
    theme,
    active: true,
    child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      _header(theme, widget.me, initialsOf(widget.me), null),
      const SizedBox(height: 8),
      _field(_draft, _s.startConversation, focus: _draftFocus, submit: _post, cancel: widget.onCancelDraft),
      const SizedBox(height: 8),
      OverflowBar(alignment: MainAxisAlignment.end, spacing: 8, children: [
        TextButton(onPressed: widget.onCancelDraft, child: Text(_s.cancel)),
        ListenableBuilder(
          listenable: _draft,
          builder: (context, _) => FilledButton.icon(
            onPressed: _draft.text.trim().isEmpty ? null : _post,
            icon: const Icon(Icons.send, size: 16),
            label: Text(_s.post),
          ),
        ),
      ]),
    ]),
  );

  Widget _threadCard(ThemeData theme, T t) {
    final active = t.id == widget.active;
    final editable = !widget.readOnly;
    final collapsed = t.done && !active;
    return _card(
      theme,
      key: _keys.putIfAbsent(t.id, GlobalKey.new),
      active: active,
      dim: t.done,
      onTap: () => widget.onSelect(t),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        _comment(theme, t.root, t, first: true, collapsed: collapsed),
        if (!collapsed)
          for (final r in t.replies) ...[
            const SizedBox(height: 10),
            _comment(theme, r, t, first: false, collapsed: false),
          ],
        if (collapsed && t.replies.isNotEmpty)
          Padding(padding: const EdgeInsets.only(top: 6), child: Text(_s.replies(t.replies.length), style: theme.textTheme.labelSmall)),
        if (!t.anchored)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(_s.commentedTextGone, style: theme.textTheme.bodySmall?.copyWith(fontStyle: FontStyle.italic, color: theme.colorScheme.outline)),
          ),
        if (active && editable && !t.done) ...[
          const SizedBox(height: 10),
          Row(children: [
            Expanded(child: _field(_reply, _s.replyHint, submit: () => _sendReply(t))),
            ListenableBuilder(
              listenable: _reply,
              builder: (context, _) => IconButton(
                tooltip: _s.post,
                onPressed: _reply.text.trim().isEmpty ? null : () => _sendReply(t),
                icon: const Icon(Icons.send, size: 18),
              ),
            ),
          ]),
        ],
        if (active && editable && t.done)
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(onPressed: () => widget.onResolve(t, false), icon: const Icon(Icons.replay, size: 16), label: Text(_s.reopen)),
          ),
      ]),
    );
  }

  Widget _comment(ThemeData theme, DocComment c, T t, {required bool first, required bool collapsed}) {
    final editable = !widget.readOnly;
    final mine = c.author == widget.me && widget.me.isNotEmpty;
    final text = _edited == c.id
        ? Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            _field(_editing, '', autofocus: true, submit: () => _saveEditing(c), cancel: () => setState(() => _edited = null)),
            OverflowBar(alignment: MainAxisAlignment.end, spacing: 8, children: [
              TextButton(onPressed: () => setState(() => _edited = null), child: Text(_s.cancel)),
              FilledButton(onPressed: () => _saveEditing(c), child: Text(_s.save)),
            ]),
          ])
        : Text(c.text, maxLines: collapsed ? 1 : null, overflow: collapsed ? TextOverflow.ellipsis : null, style: theme.textTheme.bodyMedium);
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Expanded(child: _header(theme, c.author, c.initials, c.date, small: !first)),
        if (first && t.done)
          Padding(
            padding: const EdgeInsets.only(right: 4),
            child: Chip(label: Text(_s.resolved), visualDensity: VisualDensity.compact, labelStyle: theme.textTheme.labelSmall),
          ),
        if (editable)
          PopupMenuButton<String>(
            tooltip: _s.moreActions,
            iconSize: 18,
            icon: const Icon(Icons.more_horiz),
            itemBuilder: (context) => [
              if (mine) PopupMenuItem(value: 'edit', child: Text(_s.editComment)),
              if (first && widget.resolvable) PopupMenuItem(value: 'resolve', child: Text(t.done ? _s.reopen : _s.resolveThread)),
              PopupMenuItem(value: 'delete', child: Text(first ? _s.deleteThread : _s.deleteComment)),
            ],
            onSelected: (v) => switch (v) {
              'edit' => _startEditing(c),
              'resolve' => widget.onResolve(t, !t.done),
              _ => widget.onDelete(c),
            },
          ),
      ]),
      const SizedBox(height: 4),
      text,
    ]);
  }

  Widget _header(ThemeData theme, String name, String initials, DateTime? date, {bool small = false}) {
    final color = authorColor(name);
    return Row(children: [
      CircleAvatar(
        radius: small ? 11 : 14,
        backgroundColor: color,
        child: Text(initials.isEmpty ? '?' : initials, style: TextStyle(fontSize: small ? 9 : 11, color: Colors.white, fontWeight: FontWeight.w600)),
      ),
      const SizedBox(width: 8),
      Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(name.isEmpty ? _s.unknownAuthor : name, overflow: TextOverflow.ellipsis, style: theme.textTheme.labelLarge),
          if (date != null) Text(_s.commentDate(date), style: theme.textTheme.labelSmall?.copyWith(color: theme.colorScheme.outline)),
        ]),
      ),
    ]);
  }

  Widget _field(TextEditingController c, String hint, {FocusNode? focus, bool autofocus = false, required VoidCallback submit, VoidCallback? cancel}) =>
      CallbackShortcuts(
        bindings: {
          const SingleActivator(LogicalKeyboardKey.enter, control: true): submit,
          const SingleActivator(LogicalKeyboardKey.enter, meta: true): submit,
          const SingleActivator(LogicalKeyboardKey.escape): ?cancel,
        },
        child: TextField(
          controller: c,
          focusNode: focus,
          autofocus: autofocus,
          minLines: 1,
          maxLines: 6,
          keyboardType: TextInputType.multiline,
          decoration: InputDecoration(hintText: hint, isDense: true, border: const OutlineInputBorder(), contentPadding: const EdgeInsets.all(8)),
        ),
      );
}

const _authorColors = [
  Color(0xFF2B579A), Color(0xFFB4009E), Color(0xFF038387), Color(0xFFCA5010), Color(0xFF498205), //
  Color(0xFF8764B8), Color(0xFFC239B3), Color(0xFF0078D4), Color(0xFF986F0B), Color(0xFF00B7C3),
];

/// The color of an author, the same wherever the name is shown.
Color authorColor(String name) {
  var h = 0;
  for (final c in name.codeUnits) {
    h = (h * 31 + c) & 0x7FFFFFFF;
  }
  return _authorColors[h % _authorColors.length];
}
