import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'strings.dart';

/// Something of the host a keyword found, written as [text] and, where the
/// document holds links, leading to [uri].
@immutable
class Mention {
  const Mention(this.text, this.uri, {this.detail, this.icon});

  /// What the document reads: `@Quote for Ana`.
  final String text;
  final Uri uri;

  /// What the menu shows under the text: a list, a date.
  final String? detail;
  final IconData? icon;
}

/// What a keyword proposes for the text typed after it, empty at first.
typedef MentionSource = Future<List<Mention>> Function(String query);

/// What a keyword that answers says on its way: what it is doing, then —
/// once, last — the text that takes the place of the question.
@immutable
class Answer {
  const Answer.step(this.text) : done = false;
  const Answer.done(this.text) : done = true;

  final String text;
  final bool done;
}

/// Answers a question written in the document, which [before] and [after]
/// are the text around. Giving up on the stream gives up on the answer.
typedef Answerer = Stream<Answer> Function(String question, {required String before, required String after});

/// A keyword of the host, written after an `@`: `@taches` searches what to
/// mention, `@assistant` answers what is written after it in the paragraph.
/// A source or an answer that fails throws, after telling the person why.
@immutable
class Command {
  const Command(this.keyword, {this.icon, this.search, this.answer});

  /// As it is proposed and written: one word, typed on any keyboard.
  final String keyword;
  final IconData? icon;
  final MentionSource? search;

  /// Set on a keyword that writes instead of finding.
  final Answerer? answer;
}

/// The text a keyword is typed in, as its menu needs it: the editor that
/// holds the caret.
abstract interface class CommandField {
  /// The paragraph of the caret and the caret in it, null when nothing is
  /// being typed: no caret, a selection, a document only read.
  ({String text, int caret})? get typing;

  /// The text of the document before and after that paragraph.
  ({String before, String after}) get around;

  /// Where the character at [offset] of the paragraph is drawn, in global
  /// coordinates.
  Rect? rectAt(int offset);

  /// Replaces [start] to [end] of the paragraph by [text], the caret after
  /// it. With [link], the text leads there where the document holds links,
  /// and a space follows it.
  void write(int start, int end, String text, {Uri? link});
}

class _Query {
  const _Query(this.start, this.end, this.text, [this.command]);

  /// Where the `@` is in the paragraph, and where what it started ends:
  /// the caret, or the end of the paragraph for a question.
  final int start, end;

  /// The start of a keyword without [command], the search or the question
  /// with it.
  final String text;
  final Command? command;
}

class _Row {
  const _Row(this.text, this.take, {this.detail, this.icon});

  final String text;
  final String? detail;
  final IconData? icon;
  final VoidCallback take;
}

final _wordCharacter = RegExp(r"[\p{L}\p{N}_'-]", unicode: true);
final _nameCharacter = RegExp(r'[\p{L}\p{N}_]', unicode: true);

const _accents = {
  'à': 'a', 'â': 'a', 'ä': 'a', 'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'î': 'i', 'ï': 'i',
  'ô': 'o', 'ö': 'o', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ç': 'c', 'ñ': 'n',
};

String _fold(String text) => text.toLowerCase().split('').map((c) => _accents[c] ?? c).join();

/// The keywords of a host in the text of an editor: what an `@` typed
/// there proposes, what the keyword written after it finds, and the
/// answer of one that answers, which holds the editor until it is written.
///
/// The editor keeps the focus throughout. It calls [follow] when its text
/// or its caret changed, hands its keys to [key] first, and types nothing
/// while [answering].
class Commands extends ChangeNotifier {
  Commands(this.field, {required this.commands, required this.strings});

  final CommandField field;
  final List<Command> Function() commands;
  final LofficeStrings Function() strings;

  /// The longest a search goes before its `@` is something left behind,
  /// and the longest a question is.
  static const _maxSearch = 60, _maxQuestion = 2000;
  static const _searchDelay = Duration(milliseconds: 150);

  _Query? _query;
  var _rows = const <_Row>[];
  var _chosen = 0;
  var _searching = false;
  String? _note;

  /// The `@` Escape closed the menu on: it stays closed until another one
  /// is typed.
  int? _dismissed;
  Timer? _debounce;

  /// Counts the searches: one overtaken by the next is dropped.
  var _searches = 0;

  /// The answer on its way, what it was asked as in the paragraph and what
  /// it said it was doing.
  StreamSubscription<Answer>? _answer;
  var _asked = '';
  var _steps = const <String>[];

  bool get answering => _answer != null;

  @override
  void dispose() {
    _debounce?.cancel();
    unawaited(_answer?.cancel());
    super.dispose();
  }

  /// Follows what is typed: opens the menu on an `@`, searches under a
  /// keyword, closes when the caret left.
  void follow() {
    if (answering) return;
    final typing = field.typing;
    final typed = typing == null ? null : _queryIn(typing.text, typing.caret);
    if (typed?.start != _dismissed) _dismissed = null;
    final query = _dismissed == null ? typed : null;
    final before = _query;
    if (query == null) {
      if (before != null) _close();
      return;
    }
    final command = query.command;
    final same = before?.command == command && before?.text == query.text;
    _query = query;
    if (command == null) {
      _settle([
        for (final c in commands())
          if (_fold(c.keyword).startsWith(_fold(query.text)))
            _Row('@${c.keyword}', () => field.write(query.start, query.end, '@${c.keyword} '), icon: c.icon),
      ]);
    } else if (command.answer != null) {
      _settle([
        if (query.text.isNotEmpty)
          _Row(strings().ask, () => _ask(command, query), detail: strings().askHint, icon: Icons.keyboard_return),
      ], note: same ? _note : null);
    } else if (!same) {
      if (before?.command != command) _rows = const [];
      _chosen = 0;
      _searching = true;
      _note = null;
      _debounce?.cancel();
      // the first list is asked for at once: only typing is worth waiting on
      _debounce = Timer(before?.command == command ? _searchDelay : Duration.zero, () => _find(command, query));
    }
    notifyListeners();
  }

  void _settle(List<_Row> rows, {String? note}) {
    _debounce?.cancel();
    _searches++;
    _rows = rows;
    _chosen = 0;
    _searching = false;
    _note = note;
  }

  /// Closes the menu, as when the editor loses the focus; an answer on its
  /// way stays.
  void close() {
    if (!answering && _query != null) _close();
  }

  void _close() {
    _settle(const []);
    _query = null;
    notifyListeners();
  }

  Future<void> _find(Command command, _Query query) async {
    final search = ++_searches;
    var found = const <Mention>[];
    var failed = false;
    try {
      found = await command.search?.call(query.text) ?? const [];
    } on Object {
      failed = true;
    }
    if (search != _searches) return;
    _rows = [
      for (final m in found.take(8))
        _Row(m.text, () => field.write(query.start, query.end, m.text, link: m.uri), detail: m.detail, icon: m.icon),
    ];
    _chosen = 0;
    _searching = false;
    _note = failed ? strings().searchFailed : null;
    notifyListeners();
  }

  /// Asks [command] the question of [query], and holds the editor until
  /// its answer is written.
  void _ask(Command command, _Query query) {
    final typing = field.typing;
    if (typing == null) return;
    final around = field.around;
    _asked = typing.text.substring(query.start, query.end);
    _steps = const [];
    _rows = const [];
    _note = null;
    _answer =
        command.answer!(
          query.text,
          before: around.before + typing.text.substring(0, query.start),
          after: typing.text.substring(query.end) + around.after,
        ).listen(
          (said) {
            if (said.done) {
              _answered(said.text);
            } else {
              _steps = [..._steps, said.text];
              notifyListeners();
            }
          },
          onError: (Object _) => _stop(note: strings().answerFailed),
          onDone: () => _stop(note: strings().answerFailed),
        );
    notifyListeners();
  }

  /// Writes the answer over the question, wherever the edits of the others
  /// took it in its paragraph; at the caret when they took the question.
  void _answered(String text) {
    final typing = field.typing;
    if (typing == null) {
      _stop(note: strings().answerLost);
      return;
    }
    _stop();
    final at = typing.text.indexOf(_asked);
    if (at < 0) {
      field.write(typing.caret, typing.caret, text);
    } else {
      field.write(at, at + _asked.length, text);
    }
  }

  void _stop({String? note}) {
    final answer = _answer;
    if (answer == null) return;
    _answer = null;
    unawaited(answer.cancel());
    _note = note;
    notifyListeners();
    follow();
  }

  /// Takes the row chosen, as Enter does; whether there was one.
  bool take() {
    if (answering || _query == null || _rows.isEmpty) return false;
    _rows[_chosen].take();
    return true;
  }

  /// The keys the menu takes while it is open: the arrows walk the list,
  /// Enter and Tab take the row, Escape closes it or gives up on the
  /// answer. Anything else is typing — which waits for the answer.
  bool key(KeyEvent e) {
    if (_query == null) return false;
    final key = e.logicalKey;
    final enter = key == LogicalKeyboardKey.enter || key == LogicalKeyboardKey.numpadEnter;
    if (answering) {
      if (key == LogicalKeyboardKey.escape && e is! KeyUpEvent) _stop();
      return true;
    }
    if (e is KeyUpEvent) return false;
    if (key == LogicalKeyboardKey.escape) {
      _dismissed = _query?.start;
      _close();
      return true;
    }
    if (_rows.isEmpty) {
      // Enter while the list is on its way would break the paragraph under
      // the search, and under a keyword that answers without its question
      return enter && (_searching || _query?.command?.answer != null);
    }
    if (key == LogicalKeyboardKey.arrowDown || key == LogicalKeyboardKey.arrowUp) {
      _chosen = (_chosen + (key == LogicalKeyboardKey.arrowDown ? 1 : -1)) % _rows.length;
      notifyListeners();
      return true;
    }
    return (enter || key == LogicalKeyboardKey.tab) && take();
  }

  /// The query being typed in [text] at [caret], null when the writer is
  /// naming nothing. A question may hold an `@` of its own: the keyword
  /// that asks it is further up the paragraph.
  _Query? _queryIn(String text, int caret) {
    if (caret <= 0 || caret > text.length) return null;
    final last = text.lastIndexOf('@', caret - 1);
    _Query? typed;
    for (var at = last; at >= 0 && caret - at <= _maxQuestion;) {
      final query = _queryAt(text, at, caret);
      if (query?.command?.answer != null) return query;
      if (at == last) typed = query;
      if (at == 0) break;
      at = text.lastIndexOf('@', at - 1);
    }
    return typed;
  }

  _Query? _queryAt(String text, int at, int caret) {
    // an address, not a query: "quelqu'un@example.org"
    if (at > 0 && _nameCharacter.hasMatch(text[at - 1])) return null;
    final written = text.substring(at + 1, caret);
    final space = written.indexOf(' ');
    if (space < 0) {
      final word = written.split('').every(_wordCharacter.hasMatch);
      return word && written.length < _maxSearch ? _Query(at, caret, written) : null;
    }
    final keyword = _fold(written.substring(0, space));
    final command = commands().where((c) => _fold(c.keyword) == keyword).firstOrNull;
    if (command == null) return null;
    if (command.answer != null) return _Query(at, text.length, text.substring(at + space + 2).trim(), command);
    return written.length < _maxSearch ? _Query(at, caret, written.substring(space + 1).trimLeft(), command) : null;
  }
}

/// The menu of [commands] over [child], the editor they are typed in: it
/// opens under the `@`, or over it when there is no room below.
class CommandsMenu extends StatefulWidget {
  const CommandsMenu({super.key, required this.commands, required this.child});

  final Commands commands;
  final Widget child;

  @override
  State<CommandsMenu> createState() => _CommandsMenuState();
}

class _CommandsMenuState extends State<CommandsMenu> {
  final _portal = OverlayPortalController();

  Commands get _commands => widget.commands;

  @override
  void initState() {
    super.initState();
    _commands.addListener(_changed);
  }

  @override
  void didUpdateWidget(CommandsMenu old) {
    super.didUpdateWidget(old);
    if (old.commands != widget.commands) {
      old.commands.removeListener(_changed);
      _commands.addListener(_changed);
    }
  }

  @override
  void dispose() {
    _commands.removeListener(_changed);
    super.dispose();
  }

  void _changed() {
    if (!mounted) return;
    final c = _commands;
    final open = c._query != null && (c._query!.command != null || c._rows.isNotEmpty);
    open ? _portal.show() : _portal.hide();
    setState(() {});
  }

  @override
  Widget build(BuildContext context) =>
      OverlayPortal(controller: _portal, overlayChildBuilder: _menu, child: widget.child);

  Widget _menu(BuildContext context) {
    final c = _commands;
    final query = c._query;
    final anchor = query == null ? null : c.field.rectAt(query.start);
    if (query == null || anchor == null) return const SizedBox.shrink();
    final overlay = Overlay.of(context).context.findRenderObject()! as RenderBox;
    final strings = c.strings();
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    final muted = theme.textTheme.bodySmall?.copyWith(color: colors.onSurfaceVariant);
    final command = query.command;
    final failure = c._note;
    final hint = command?.answer != null
        ? strings.writeQuestion
        : c._searching
        ? null
        : strings.noResult;
    return CustomSingleChildLayout(
      delegate: _Below(Rect.fromPoints(overlay.globalToLocal(anchor.topLeft), overlay.globalToLocal(anchor.bottomRight))),
      child: TextFieldTapRegion(
        child: Material(
          elevation: 4,
          borderRadius: BorderRadius.circular(8),
          clipBehavior: Clip.antiAlias,
          color: colors.surfaceContainer,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minWidth: 220, maxWidth: 340),
            child: IntrinsicWidth(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (command != null) _heading(theme, strings, command, query.text, busy: c._searching || c.answering),
                  if (c.answering) ...[
                    Padding(
                      padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          if (c._steps.isEmpty) Text(strings.thinking, style: muted),
                          for (final (i, step) in c._steps.indexed)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 4),
                              child: Text(
                                step,
                                maxLines: 3,
                                overflow: TextOverflow.ellipsis,
                                style: i == c._steps.length - 1 ? theme.textTheme.bodySmall : muted,
                              ),
                            ),
                        ],
                      ),
                    ),
                    Align(
                      alignment: Alignment.centerRight,
                      child: TextButton(onPressed: c._stop, child: Text(strings.cancel)),
                    ),
                  ] else ...[
                    if (failure != null)
                      Padding(
                        padding: const EdgeInsets.all(12),
                        child: Text(failure, style: muted?.copyWith(color: colors.error)),
                      ),
                    for (final (i, row) in c._rows.indexed)
                      InkWell(
                        canRequestFocus: false,
                        onTap: row.take,
                        child: Container(
                          color: i == c._chosen ? colors.primary.withValues(alpha: 0.12) : null,
                          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                          child: Row(
                            children: [
                              if (row.icon != null) ...[
                                Icon(row.icon, size: 20, color: colors.onSurfaceVariant),
                                const SizedBox(width: 10),
                              ],
                              Flexible(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  mainAxisSize: MainAxisSize.min,
                                  children: [
                                    Text(row.text, maxLines: 1, overflow: TextOverflow.ellipsis),
                                    if (row.detail case final detail?)
                                      Text(detail, maxLines: 1, overflow: TextOverflow.ellipsis, style: muted),
                                  ],
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    if (failure == null && c._rows.isEmpty && hint != null)
                      Padding(padding: const EdgeInsets.all(12), child: Text(hint, style: muted)),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// The search or the question as the document holds it.
  Widget _heading(ThemeData theme, LofficeStrings strings, Command command, String typed, {required bool busy}) {
    final colors = theme.colorScheme;
    final empty = command.answer == null ? strings.searchIn(command.keyword) : strings.questionFor(command.keyword);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(border: Border(bottom: BorderSide(color: colors.outlineVariant))),
      child: Row(
        children: [
          Icon(command.icon ?? Icons.alternate_email, size: 18, color: colors.primary),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              typed.isEmpty ? empty : typed,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: theme.textTheme.bodyMedium?.copyWith(color: typed.isEmpty ? colors.onSurfaceVariant : null),
            ),
          ),
          if (busy) const SizedBox.square(dimension: 14, child: CircularProgressIndicator(strokeWidth: 2)),
        ],
      ),
    );
  }
}

class _Below extends SingleChildLayoutDelegate {
  _Below(this.anchor);

  final Rect anchor;

  static const _gap = 4.0;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) {
    final room = math.max(anchor.top, constraints.maxHeight - anchor.bottom) - _gap * 2;
    return BoxConstraints.loose(Size(constraints.maxWidth, math.max(0, room)));
  }

  @override
  Offset getPositionForChild(Size size, Size child) {
    final below = anchor.bottom + _gap + child.height <= size.height || anchor.top < child.height + _gap;
    final x = math.min(math.max(anchor.left, 0.0), math.max(0.0, size.width - child.width));
    return Offset(x, below ? anchor.bottom + _gap : anchor.top - _gap - child.height);
  }

  @override
  bool shouldRelayout(_Below old) => old.anchor != anchor;
}
