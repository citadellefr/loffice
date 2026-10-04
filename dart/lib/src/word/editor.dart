import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/ribbon.dart';
import '../chrome/strings.dart';
import '../powerpoint/slide_painter.dart' show MediaCache, MediaFetcher;
import '../text/text_frame.dart' show Fonts, Props;
import 'blocks.dart';
import 'comments.dart';
import 'comments_pane.dart';
import 'document.dart';
import 'edits.dart';
import 'layout.dart';
import 'page_painter.dart';
import 'pages_view.dart';
import 'paragraph.dart';
import 'revisions.dart';

/// A Word editor on a session whose document is a Word document: the
/// ribbon, the pages, the status bar.
class WordEditor extends StatefulWidget {
  const WordEditor({
    super.key,
    required this.session,
    required this.media,
    this.title = '',
    this.onClose,
    this.strings = const LofficeStrings(),
    this.fonts,
    this.onPicture,
  });

  final DocSession session;

  /// Fetches the pictures of the document by their name.
  final MediaFetcher media;
  final String title;
  final VoidCallback? onClose;
  final LofficeStrings strings;

  /// The package bundling the free fonts standing in for Office's.
  final String? fonts;

  /// Lets the user pick a picture and sends it to the document: its name
  /// and size in pixels, null when nothing was picked. Without it no
  /// picture can be inserted.
  final Future<({String media, int width, int height})?> Function()? onPicture;

  @override
  State<WordEditor> createState() => _WordEditorState();
}

/// Word's zoom of 100 %: a point is 1/72 inch, a logical pixel 1/96.
const _pointsToPixels = 96 / 72;

class _WordEditorState extends State<WordEditor> {
  late final MediaCache _media = MediaCache(widget.media)..addListener(_repaint);
  final _selection = WordSelection();
  final _focus = FocusNode();
  final _cache = ParaCache();
  final _view = GlobalKey<WordPagesViewState>();
  StreamSubscription<String>? _rejections;
  StreamSubscription<Edit>? _changes;
  final _find = TextEditingController();
  final _replacement = TextEditingController();
  final _findFocus = FocusNode();
  var _finding = false;
  var _replacing = false;
  var _matchCase = false;
  var _match = -1;
  var _zoom = 1.0;
  var _fitWidth = false;
  var _backstage = false;

  /// Whether the comments are shown, by default when the document has some;
  /// the thread whose card is open; the range of the comment being
  /// written.
  bool? _commentsShown;
  String? _activeThread;
  ((String, int), (String, int))? _draft;
  var _followCaret = true;
  List<WordThread>? _threads;
  var _threadRanges = <String, List<(String, int, int)>>{};
  List<WordRevision>? _revisionList;

  Tree? _laid;
  WordDocument? _doc;
  WordLayout? _layout;
  WordContext? _context;

  DocSession get _session => widget.session;
  LofficeStrings get _s => widget.strings;

  @override
  void initState() {
    super.initState();
    _session.addListener(_repaint);
    _selection.addListener(_repaint);
    _selection.addListener(_caretMoved);
    _rejections = _session.rejections.listen((reason) {
      if (!mounted) return;
      ScaffoldMessenger.maybeOf(context)?.showSnackBar(SnackBar(content: Text(_s.refused(reason))));
    });
    // the tree changes in place: each edit lays it out again
    _changes = _session.changes.listen((edit) {
      _laid = null;
      _draft = _moveDraft(edit);
    });
  }

  @override
  void dispose() {
    _session.removeListener(_repaint);
    _selection
      ..removeListener(_repaint)
      ..removeListener(_caretMoved);
    unawaited(_rejections?.cancel());
    unawaited(_changes?.cancel());
    _media
      ..removeListener(_repaint)
      ..dispose();
    _selection.dispose();
    _focus.dispose();
    _find.dispose();
    _replacement.dispose();
    _findFocus.dispose();
    super.dispose();
  }

  void _repaint() {
    if (mounted) setState(() {});
  }

  /// The document laid out, again only when the tree changed, and then
  /// only the paragraphs that changed.
  WordLayout get _current {
    final tree = _session.document;
    if (_laid != tree || _layout == null) {
      final doc = WordDocument(tree, previous: _doc);
      _doc = doc;
      _context = WordContext(doc, fonts: Fonts(theme: doc.typeface, package: widget.fonts));
      _layout = WordLayout(doc, _context!, _cache);
      _laid = tree;
      _threads = null;
      _revisionList = null;
    }
    return _layout!;
  }

  WordDocument get _document {
    _current;
    return _doc!;
  }

  bool _edit(Edit edit) {
    if (edit.isEmpty) return false;
    return _session.edit(edit);
  }

  Node? get _flow {
    final id = _selection.flow;
    return id == null ? null : _session.document[id];
  }

  /// Puts the caret at the start of the document when there is none yet.
  void _ensureCaret() {
    if (_selection.flow != null && _flow != null) return;
    final first = flowsOf(_session.document).firstOrNull;
    if (first != null) _selection.set(first.id, 0, null, 0, PageArea.body);
  }

  // formatting

  /// The paragraph formatting in effect where the selection starts.
  Props get _paraProps {
    final flow = _flow;
    if (flow == null) return const {};
    return _document.paragraph(wordEditing(flow).markAt(_selection.start));
  }

  /// The run formatting in effect where the selection starts, what is
  /// inherited included.
  Props get _runProps {
    final flow = _flow;
    if (flow == null) return const {};
    final editing = wordEditing(flow);
    final mark = editing.markAt(_selection.start);
    final own = _selection.collapsed
        ? (_selection.typing ?? editing.typingAttributes(_selection.start))
        : (editing.attributesAt(_selection.start) ?? const <String, String>{});
    return _document.run(_document.paragraphRun(mark), own);
  }

  /// Sets run attributes ("" removes) on the selection; at a caret, on what
  /// is typed next.
  void _formatRuns(Attributes attributes) {
    final flow = _flow;
    if (flow == null) return;
    if (_selection.spans) {
      _edit(Edit([
        for (final (node, a, b) in _ranges)
          if (wordEditing(node).formatRuns(a, b, attributes) case final d?) Change.text(node.id, d),
      ]));
      return;
    }
    final editing = wordEditing(flow);
    if (_selection.collapsed) {
      _selection.typing = {...(_selection.typing ?? editing.typingAttributes(_selection.start)), ...attributes}
        ..removeWhere((_, v) => v.isEmpty);
      _repaint();
      return;
    }
    final d = editing.formatRuns(_selection.start, _selection.end, attributes);
    if (d != null) _edit(Edit([Change.text(flow.id, d)]));
  }

  /// The ranges of the flows the selection covers, in reading order.
  List<(Node, int, int)> get _ranges {
    final tree = _session.document;
    final root = _selection.area == PageArea.body ? 'body' : headerNodeOf(tree, _selection.flow ?? '');
    return _selection.ranges(tree, root == null ? const [] : flowsOf(tree, root));
  }

  void _formatParagraphs(Attributes attributes) {
    final flow = _flow;
    if (flow == null) return;
    if (_selection.spans) {
      _edit(Edit([for (final (node, a, b) in _ranges) Change.text(node.id, wordEditing(node).formatParagraphs(a, b, attributes))]));
      return;
    }
    _edit(Edit([Change.text(flow.id, wordEditing(flow).formatParagraphs(_selection.start, _selection.end, attributes))]));
  }

  void _toggle(String key) => _formatRuns({key: _runProps[key] == '1' ? '0' : '1'});

  void _toggleUnderline() {
    final u = _runProps['u'];
    _formatRuns({'u': u != null && u != 'none' ? 'none' : 'single'});
  }

  void _toggleVertical(String va) => _formatRuns({'va': _runProps['va'] == va ? 'baseline' : va});

  static const _sizes = [8, 9, 10, 10.5, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72];

  double get _size => (double.tryParse(_runProps['sz'] ?? '') ?? 20) / 2;

  void _setSize(double pt) => _formatRuns({'sz': '${(pt * 2).round()}'});

  void _growFont(bool up) {
    final now = _size;
    final next = up
        ? _sizes.firstWhere((s) => s > now + 0.01, orElse: () => now + 12)
        : _sizes.lastWhere((s) => s < now - 0.01, orElse: () => now > 1 ? now - 1 : now);
    _setSize(next.toDouble());
  }

  void _clearFormatting() {
    _formatRuns({for (final k in wordRunKeys) k: ''});
    _formatParagraphs({'pstyle': '', 'jc': '', 'ind.left': '', 'ind.right': '', 'ind.first': '', 'sp.before': '', 'sp.after': '', 'sp.line': '', 'sp.rule': ''});
  }

  void _indent(int by) {
    final left = double.tryParse(_paraProps['ind.left'] ?? '') ?? 0;
    final step = _document.settings.tab.toDouble();
    final next = by > 0 ? ((left / step).floor() + 1) * step : ((left / step).ceil() - 1) * step;
    _formatParagraphs({'ind.left': '${next.clamp(0, 31680).round()}'});
  }

  void _lineSpacing(double lines) => _formatParagraphs({'sp.line': '${(lines * 240).round()}', 'sp.rule': 'auto'});

  void _spacing(String key, bool add) => _formatParagraphs({key: add ? '240' : '0'});

  /// Bullets or numbers: on, with the document's first list of the kind or
  /// a new one; off when the paragraph has them already.
  void _list(String kind) {
    final para = _paraProps;
    final level = _document.levelOf(para['num'], para['lvl']);
    final isKind = level != null && (level.format == 'bullet') == (kind == 'bullet');
    if (isKind) {
      final mark = _flow == null ? const <String, String>{} : wordEditing(_flow!).markAt(_selection.start);
      final fromStyle = mark['num'] == null;
      _formatParagraphs({'num': fromStyle ? '0' : '', 'lvl': ''});
      return;
    }
    final existing = _document.numbering.entries.where((e) {
      final first = e.value.levels.firstOrNull;
      return first != null && (first.format == 'bullet') == (kind == 'bullet') && (kind == 'bullet' || first.format == 'decimal');
    }).map((e) => e.key);
    final num = existing.firstOrNull ?? kind;
    _formatParagraphs({'num': num, 'lvl': '0'});
  }

  void _style(String id) {
    _formatParagraphs({'pstyle': id});
  }

  /// The quick styles of the document, in the order Word shows them: the
  /// default paragraph style first.
  List<MapEntry<String, WordStyle>> get _quickStyles {
    final doc = _document;
    final list = doc.styles.entries.where((e) => e.value.type == 'paragraph' && (e.value.quick || e.key == doc.paragraphStyle) && !e.value.hidden).toList();
    int rank(MapEntry<String, WordStyle> e) => e.key == doc.paragraphStyle ? -1 : e.value.priority ?? 99;
    list.sort((a, b) => rank(a).compareTo(rank(b)));
    return list;
  }

  // inserting

  void _insertTable(int rows, int columns) {
    final flow = _flow;
    if (flow == null || _session.document[flow.parent]?.type == 'tc') return;
    final page = _current.pages.firstOrNull;
    final width = page == null ? 450.0 : (page.body.width - 0.5);
    final edit = WordEdits(_document).insertTable(flow, _selection.start, rows, columns, width);
    if (_edit(edit)) {
      final first = edit.changes.firstWhere((c) => c.kind == ChangeKind.create && c.type == 'text');
      _selection.set(first.id, 0, null, null, PageArea.body);
      _focus.requestFocus();
    }
  }

  /// Inserts a picture at the caret, no wider than the text.
  Future<void> _picture() async {
    final pick = widget.onPicture;
    if (pick == null) return;
    final ({String media, int width, int height})? picture;
    try {
      picture = await pick();
    } on Object catch (e) {
      if (mounted) ScaffoldMessenger.maybeOf(context)?.showSnackBar(SnackBar(content: Text('${_s.pictureFailed} : $e')));
      return;
    }
    final flow = _flow;
    if (picture == null || flow == null || !mounted || picture.width <= 0 || picture.height <= 0) return;
    final layout = _current;
    final at = layout.caret(flow.id, _selection.extent, page: _selection.page);
    final page = at == null ? layout.pages.firstOrNull : layout.pages[at.$1];
    final room = page?.body.width ?? 450;
    var w = picture.width * 0.75, h = picture.height * 0.75;
    if (w > room) (w, h) = (room, h * room / w);
    const emu = 12700;
    _view.currentState?.replaceWith(flow, _selection.start, _selection.end, '￼', {
      'img': jsonEncode({'media': picture.media, 'w': (w * emu).round(), 'h': (h * emu).round()}),
    });
    _focus.requestFocus();
  }

  void _pageBreak() {
    final flow = _flow;
    if (flow == null) return;
    _view.currentState?.replaceWith(flow, _selection.start, _selection.end, '￼', {'br': 'page'});
    _focus.requestFocus();
  }

  /// Edits the header or footer of the page the caret is on, making one
  /// when the section has none; [pageNumber] adds the page's number.
  void _headerFooter({required bool footer, bool pageNumber = false}) {
    final layout = _current;
    final flow = _flow;
    final at = flow == null ? null : layout.caret(flow.id, _selection.extent, page: _selection.page);
    final pageIndex = at?.$1 ?? 0;
    if (pageIndex >= layout.pages.length) return;
    final page = layout.pages[pageIndex];
    final existing = footer ? page.footer : page.header;
    if (existing != null && !pageNumber) {
      final first = flowsOf(_session.document, existing).firstOrNull;
      if (first != null) {
        _selection.set(first.id, 0, null, pageIndex, footer ? PageArea.footer : PageArea.header);
        _focus.requestFocus();
      }
      return;
    }
    final kind = footer ? 'ftr' : 'hdr';
    final style = footer ? 'Footer' : 'Header';
    final mark = <String, String>{
      if (_document.styles.containsKey(style)) 'pstyle': style,
      if (pageNumber) 'jc': 'center',
    };
    final text = Delta();
    if (pageNumber) text.insert('1', {'field': 'PAGE'});
    text.insert('\n', mark.isEmpty ? null : mark);
    if (existing != null) {
      // the page number goes at the end of the footer there is
      final last = flowsOf(_session.document, existing).lastOrNull;
      if (last == null) return;
      final at = last.text!.length - 1;
      _edit(Edit([Change.text(last.id, Delta()..retain(at)..insert('\n', WordEdits.paragraphLike(wordEditing(last).markAt(at)))..insert('1', {'field': 'PAGE'}))]));
      return;
    }
    final id = randomId();
    final kids = _session.document.children('doc');
    final edits = WordEdits(_document).pageSetup((s) {
      final refs = {...(s[kind] as Map<String, Object?>? ?? const {})};
      refs['default'] = id;
      return {...s, kind: refs};
    });
    final edit = Edit([
      Change.create(Node(id: id, type: kind, parent: 'doc', key: keyBetween(kids.isEmpty ? '' : kids.last.key, ''))),
      Change.create(Node(id: randomId(), type: 'text', parent: id, key: 'V', text: text)),
      ...edits.changes,
    ]);
    if (_edit(edit)) {
      final first = edit.changes[1];
      _selection.set(first.id, 0, null, pageIndex, footer ? PageArea.footer : PageArea.header);
      _focus.requestFocus();
    }
  }

  void _closeBackstage() => setState(() => _backstage = false);

  void _closeHeaderFooter() {
    final body = flowsOf(_session.document).firstOrNull;
    if (body != null) _selection.set(body.id, 0, null, null, PageArea.body);
  }

  // page layout

  void _margins(int top, int right, int bottom, int left) =>
      _edit(WordEdits(_document).pageSetup((s) => {...s, 'top': top, 'right': right, 'bottom': bottom, 'left': left}));

  void _orientation(bool landscape) => _edit(WordEdits(_document).pageSetup((s) {
    final w = (s['w'] as num?) ?? 12240, h = (s['h'] as num?) ?? 15840;
    final long = w > h ? w : h, short = w > h ? h : w;
    return {...s, 'w': landscape ? long : short, 'h': landscape ? short : long, 'orient': landscape ? 'landscape' : 'portrait'};
  }));

  void _paperSize(int w, int h) => _edit(WordEdits(_document).pageSetup((s) {
    final landscape = s['orient'] == 'landscape';
    return {...s, 'w': landscape ? h : w, 'h': landscape ? w : h};
  }));

  void _columns(int n) => _edit(WordEdits(_document).pageSetup((s) => {...s, 'cols': n, 'colSpace': s['colSpace'] ?? 720}));

  // tables

  Node? get _cell {
    final flow = _flow;
    if (flow == null) return null;
    final cell = _session.document[flow.parent];
    return cell?.type == 'tc' ? cell : null;
  }

  void _tableEdit(Edit Function(WordEdits edits, Node cell) make) {
    final cell = _cell;
    if (cell == null) return;
    final flowBefore = _flow;
    final edit = make(WordEdits(_document), cell);
    if (!_edit(edit)) return;
    if (flowBefore != null && _session.document[flowBefore.id] == null) {
      final first = flowsOf(_session.document).firstOrNull;
      if (first != null) _selection.set(first.id, 0, null, null, PageArea.body);
    }
  }

  Future<void> _contextMenu(Offset at) async {
    final cell = _cell;
    final editable = !_session.readOnly;
    final revision = _revision;
    final choice = await showMenu<String>(
      context: context,
      position: RelativeRect.fromLTRB(at.dx, at.dy, at.dx, at.dy),
      items: [
        PopupMenuItem(value: 'cut', enabled: editable && !_selection.collapsed, child: Text(_s.cut)),
        PopupMenuItem(value: 'copy', enabled: !_selection.collapsed, child: Text(_s.copy)),
        PopupMenuItem(value: 'paste', enabled: editable, child: Text(_s.paste)),
        const PopupMenuDivider(),
        PopupMenuItem(value: 'comment', enabled: editable, child: Text(_s.newComment)),
        if (revision != null && editable) ...[
          const PopupMenuDivider(),
          PopupMenuItem(value: 'accept', child: Text(_s.settleRevision(true, revision.deleted))),
          PopupMenuItem(value: 'reject', child: Text(_s.settleRevision(false, revision.deleted))),
        ],
        if (cell != null && editable) ...[
          const PopupMenuDivider(),
          PopupMenuItem(value: 'rowAbove', child: Text(_s.insertRowAbove)),
          PopupMenuItem(value: 'rowBelow', child: Text(_s.insertRowBelow)),
          PopupMenuItem(value: 'colLeft', child: Text(_s.insertColumnLeft)),
          PopupMenuItem(value: 'colRight', child: Text(_s.insertColumnRight)),
          const PopupMenuDivider(),
          PopupMenuItem(value: 'delRow', child: Text(_s.deleteRow)),
          PopupMenuItem(value: 'delCol', child: Text(_s.deleteColumn)),
          PopupMenuItem(value: 'delTable', child: Text(_s.deleteTable)),
        ],
      ],
    );
    final key = switch (choice) {
      'cut' => LogicalKeyboardKey.keyX,
      'copy' => LogicalKeyboardKey.keyC,
      'paste' => LogicalKeyboardKey.keyV,
      _ => null,
    };
    if (key != null) {
      _clipboard(key);
      return;
    }
    switch (choice) {
      case 'comment':
        _newComment();
      case 'accept' || 'reject':
        _settle(accept: choice == 'accept');
      case 'rowAbove':
        _tableEdit((e, c) => e.insertRow(_session.document[c.parent]!, below: false));
      case 'rowBelow':
        _tableEdit((e, c) => e.insertRow(_session.document[c.parent]!, below: true));
      case 'colLeft':
        _tableEdit((e, c) => e.insertColumn(c, right: false));
      case 'colRight':
        _tableEdit((e, c) => e.insertColumn(c, right: true));
      case 'delRow':
        _tableEdit((e, c) => e.deleteRow(c));
      case 'delCol':
        _tableEdit((e, c) => e.deleteColumn(c));
      case 'delTable':
        _tableEdit((e, c) => e.deleteTable(_session.document[_session.document[c.parent]!.parent]!));
    }
  }

  void _clipboard(LogicalKeyboardKey key) {
    _focus.requestFocus();
    final flow = _flow;
    if (flow != null) _view.currentState?.clipboard(flow, key);
  }

  // comments

  List<WordThread> get _commentThreads {
    _current;
    if (_threads == null) {
      final tree = _session.document;
      final threads = commentThreads(tree);
      final flows = flowsOf(tree);
      _threads = threads;
      _threadRanges = {for (final t in threads) t.id: threadRanges(t, flows)};
    }
    return _threads!;
  }

  CommentAuthor get _author => (
    name: _session.name,
    date: DateTime.now(),
    style: _document.styles.containsKey('CommentText') ? 'CommentText' : null,
  );

  /// The thread whose range holds the caret, the innermost.
  String? _threadAt(String flow, int offset) {
    String? found;
    for (final t in _commentThreads) {
      if (_threadRanges[t.id]!.any((r) => r.$1 == flow && r.$2 <= offset && offset <= r.$3)) found = t.id;
    }
    return found;
  }

  void _caretMoved() {
    final flow = _selection.flow;
    if (!_followCaret || flow == null) return;
    final id = _threadAt(flow, _selection.extent);
    if (id != _activeThread) setState(() => _activeThread = id);
  }

  /// The range of the comment being written, moved over [edit]; none when
  /// its text went.
  ((String, int), (String, int))? _moveDraft(Edit edit) {
    final draft = _draft;
    if (draft == null) return null;
    (String, int)? move((String, int) at) {
      if (_session.document[at.$1] == null) return null;
      var offset = at.$2;
      for (final c in edit.changes) {
        if (c.kind == ChangeKind.text && c.id == at.$1) offset = c.text!.transformPosition(offset, thisFirst: false);
      }
      return (at.$1, offset);
    }

    final start = move(draft.$1), end = move(draft.$2);
    return start == null || end == null ? null : (start, end);
  }

  /// Starts a comment on the selection, or on the word of the caret.
  void _newComment() {
    final flow = _flow;
    if (flow == null || _session.readOnly || !_session.document.isUnder(flow.id, 'body')) return;
    final (String, int) start, end;
    if (_selection.spans) {
      final ranges = _ranges;
      if (ranges.isEmpty) return;
      start = (ranges.first.$1.id, ranges.first.$2);
      end = (ranges.last.$1.id, ranges.last.$3);
    } else if (_selection.collapsed) {
      final (a, b) = wordEditing(flow).wordAt(_selection.start);
      (start, end) = ((flow.id, a), (flow.id, b));
    } else {
      (start, end) = ((flow.id, _selection.start), (flow.id, _selection.end));
    }
    setState(() {
      _draft = (start, end);
      _commentsShown = true;
      _activeThread = null;
    });
  }

  void _postComment(String text) {
    final draft = _draft;
    if (draft == null) return;
    final edit = addComment(_session.document, draft.$1, draft.$2, text, _author);
    if (_edit(edit)) {
      setState(() {
        _draft = null;
        _activeThread = edit.changes.first.id;
      });
    }
  }

  void _cancelDraft() {
    setState(() => _draft = null);
    _focus.requestFocus();
  }

  /// Opens a thread's card and brings its text into view, the caret at its
  /// start.
  void _selectThread(WordThread t) {
    setState(() => _activeThread = t.id);
    final at = t.start ?? t.end ?? t.reference;
    if (at == null || _session.document[at.$1] == null) return;
    _followCaret = false;
    _selection.set(at.$1, at.$2, null, null, PageArea.body);
    _followCaret = true;
    WidgetsBinding.instance.addPostFrameCallback((_) => _view.currentState?.reveal());
  }

  /// Opens the thread before or after the one open, in the order of the
  /// text.
  void _stepThread(int by) {
    final threads = _commentThreads.where((t) => t.anchored).toList();
    if (threads.isEmpty) return;
    final i = threads.indexWhere((t) => t.id == _activeThread);
    final next = i < 0 ? (by > 0 ? 0 : threads.length - 1) : (i + by).clamp(0, threads.length - 1);
    setState(() => _commentsShown = true);
    _selectThread(threads[next]);
  }

  /// Deletes a comment, and its answers when it starts a thread.
  void _deleteComment(WordComment c) {
    final thread = _commentThreads.where((t) => t.id == c.id).firstOrNull;
    final ids = thread == null ? {c.id} : {for (final r in thread.all) r.id};
    if (_edit(deleteComments(_session.document, ids)) && ids.contains(_activeThread)) setState(() => _activeThread = null);
  }

  WordThread? get _active => _commentThreads.where((t) => t.id == _activeThread).firstOrNull;

  /// The ranges of the comments drawn on the text: the open one darker.
  List<(String, int, int, Color)> get _commentMarks {
    final out = <(String, int, int, Color)>[];
    for (final t in _commentThreads) {
      final active = t.id == _activeThread;
      if (t.done && !active) continue;
      final color = authorColor(t.root.author).withValues(alpha: active ? 0.34 : 0.14);
      for (final (f, a, b) in _threadRanges[t.id]!) {
        out.add((f, a, b, color));
      }
    }
    final draft = _draft;
    if (draft != null) {
      final flows = flowsOf(_session.document);
      final i = flows.indexWhere((f) => f.id == draft.$1.$1), j = flows.indexWhere((f) => f.id == draft.$2.$1);
      final color = Theme.of(context).colorScheme.primary.withValues(alpha: 0.3);
      for (var k = math.max(i, 0); i >= 0 && k <= j; k++) {
        out.add((flows[k].id, k == i ? draft.$1.$2 : 0, k == j ? draft.$2.$2 : flows[k].text!.length - 1, color));
      }
    }
    return out;
  }

  Widget _commentsPane(BuildContext context) => WordCommentsPane(
    threads: _commentThreads,
    active: _activeThread,
    me: _session.name,
    readOnly: _session.readOnly,
    drafting: _draft != null,
    strings: _s,
    onSelect: _selectThread,
    onPost: _postComment,
    onCancelDraft: _cancelDraft,
    onReply: (t, text) => _edit(replyTo(_session.document, t, text, _author)),
    onResolve: (t, done) => _edit(resolveThread(t, done)),
    onDelete: _deleteComment,
    onEdit: (c, text) => _edit(editComment(_session.document, c, text)),
    onClose: () => setState(() {
      _commentsShown = false;
      _draft = null;
    }),
  );

  // searching

  /// What the search finds in the body, in reading order.
  List<(Node, int, int)> get _matches =>
      _finding ? findIn(flowsOf(_session.document), _find.text, matchCase: _matchCase) : const [];

  // tracked changes

  /// Whether the document tracks changes, everyone's.
  bool get _tracking => _session.document['doc']?.attributes['track'] == true;

  /// The author this client's changes are tracked as, null when they are
  /// not tracked.
  String? get _trackAs => _tracking && _session.name.isNotEmpty ? _session.name : null;

  Edit _trackedEdit(Edit edit) {
    final author = _trackAs;
    return author == null ? edit : trackEdit(_session.document, edit, author, revisionDate(DateTime.now()));
  }

  void _toggleTracking() => _edit(Edit([Change.set('doc', attributes: {'track': !_tracking})]));

  List<WordRevision> get _revisions {
    _current;
    return _revisionList ??= revisionsOf(flowsOf(_session.document));
  }

  /// The tracked change the caret is in, or just after.
  WordRevision? get _revision {
    final flow = _selection.flow;
    if (flow == null || _selection.spans) return null;
    final at = _selection.start;
    final here = _revisions.where((r) => r.flow == flow);
    return here.where((r) => r.start <= at && at < r.end).firstOrNull ?? here.where((r) => r.end == at).firstOrNull;
  }

  /// Accepts or rejects the changes selected, or the one at the caret, or
  /// [all] of them, then [stop]s tracking or goes to the [next] one.
  void _settle({required bool accept, bool all = false, bool next = false, bool stop = false}) {
    final tree = _session.document;
    final flows = flowsOf(tree);
    final r = _revision;
    final flow = r == null ? null : tree[r.flow];
    final ranges = all
        ? [for (final f in flows) (f, 0, f.text!.length)]
        : !_selection.collapsed
        ? _selection.ranges(tree, flows)
        : [if (flow != null) (flow, r!.start, r.end)];
    _edit(Edit([
      ...settle(ranges, accept: accept).changes,
      if (stop && _tracking) Change.set('doc', attributes: {'track': false}),
    ]));
    if (next) _stepRevision(1);
    _focus.requestFocus();
  }

  /// Selects the tracked change after the selection, or before it, going
  /// round the document.
  void _stepRevision(int by) {
    final revisions = _revisions;
    if (revisions.isEmpty) return;
    final order = {for (final (i, f) in flowsOf(_session.document).indexed) f.id: i};
    final s = _selection;
    final flow = order[s.flow] ?? 0;
    bool current(WordRevision r) => r.flow == s.flow && r.start == s.start && r.end == s.end;
    int compare(WordRevision r, int at) => order[r.flow]! != flow ? order[r.flow]!.compareTo(flow) : at.compareTo(by > 0 ? s.end : s.start);
    final found = by > 0
        ? revisions.where((r) => compare(r, r.start) >= 0 && !current(r)).firstOrNull ?? revisions.first
        : revisions.where((r) => compare(r, r.end) <= 0 && !current(r)).lastOrNull ?? revisions.last;
    s.set(found.flow, found.start, found.end, null, PageArea.body);
    WidgetsBinding.instance.addPostFrameCallback((_) => _view.currentState?.reveal());
  }

  List<PopupMenuEntry<String>> _settleItems(bool accept) => [
    PopupMenuItem(value: 'next', child: Text(accept ? _s.acceptAndNext : _s.rejectAndNext)),
    PopupMenuItem(value: 'this', enabled: !_selection.collapsed || _revision != null, child: Text(accept ? _s.acceptThis : _s.rejectThis)),
    PopupMenuItem(value: 'all', child: Text(accept ? _s.acceptAll : _s.rejectAll)),
    PopupMenuItem(value: 'stop', enabled: _tracking, child: Text(accept ? _s.acceptAllAndStop : _s.rejectAllAndStop)),
  ];

  void _settleChosen(bool accept, String choice) =>
      _settle(accept: accept, all: choice == 'all' || choice == 'stop', next: choice == 'next', stop: choice == 'stop');

  void _openFind({bool replace = false}) {
    setState(() {
      _finding = true;
      _replacing = replace || _replacing;
      if (!_selection.collapsed && !_selection.spans && _flow != null) {
        _find.text = wordEditing(_flow!).text.substring(_selection.start, _selection.end);
      }
    });
    _findFocus.requestFocus();
    _find.selection = TextSelection(baseOffset: 0, extentOffset: _find.text.length);
  }

  void _closeFind() {
    setState(() {
      _finding = false;
      _match = -1;
    });
    _focus.requestFocus();
  }

  /// Selects the next match after the caret, or before it.
  void _step(int by) {
    final matches = _matches;
    if (matches.isEmpty) return setState(() => _match = -1);
    var i = _match;
    if (i < 0 || i >= matches.length) {
      final order = flowsOf(_session.document).map((n) => n.id).toList();
      final here = order.indexOf(_selection.flow ?? '');
      i = matches.indexWhere((m) {
        final at = order.indexOf(m.$1.id);
        return at > here || at == here && m.$2 >= _selection.start;
      });
      if (by < 0) i = (i < 0 ? matches.length : i) - 1;
      if (i < 0) i = by < 0 ? matches.length - 1 : 0;
    } else {
      i = (i + by) % matches.length;
    }
    final (node, a, b) = matches[i];
    setState(() => _match = i);
    _selection.set(node.id, a, b, null, PageArea.body);
    WidgetsBinding.instance.addPostFrameCallback((_) => _view.currentState?.reveal());
  }

  void _replaceOne() {
    final matches = _matches;
    final flow = _flow;
    final current = _match >= 0 && _match < matches.length ? matches[_match] : null;
    if (flow == null || current == null || current.$1.id != flow.id || current.$2 != _selection.start || current.$3 != _selection.end) {
      _step(1);
      return;
    }
    _edit(_trackedEdit(replaceAll([current], _replacement.text)));
    _match--;
    _step(1);
  }

  void _replaceEvery() {
    final matches = _matches;
    if (matches.isEmpty) return;
    _edit(_trackedEdit(replaceAll(matches, _replacement.text)));
    setState(() => _match = -1);
  }

  Widget _findBar(BuildContext context) {
    final matches = _matches;
    final s = _s;
    final count = _match >= 0 && _match < matches.length ? s.resultOf(_match + 1, matches.length) : s.results(matches.length);
    Widget field(TextEditingController c, String hint, {FocusNode? focus, ValueChanged<String>? submit}) => SizedBox(
      width: 220,
      height: 32,
      child: TextField(
        controller: c,
        focusNode: focus,
        onChanged: (_) => setState(() => _match = -1),
        onSubmitted: submit,
        decoration: InputDecoration(hintText: hint, isDense: true, border: const OutlineInputBorder(), contentPadding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8)),
      ),
    );
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainerLow,
      child: CallbackShortcuts(
        bindings: {const SingleActivator(LogicalKeyboardKey.escape): _closeFind},
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          child: Wrap(
            spacing: 8,
            runSpacing: 6,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              field(_find, s.find, focus: _findFocus, submit: (_) {
                _step(1);
                _findFocus.requestFocus();
              }),
              IconButton(tooltip: s.findPrevious, iconSize: 18, onPressed: () => _step(-1), icon: const Icon(Icons.keyboard_arrow_up)),
              IconButton(tooltip: s.findNext, iconSize: 18, onPressed: () => _step(1), icon: const Icon(Icons.keyboard_arrow_down)),
              Text(_find.text.isEmpty ? '' : count, style: Theme.of(context).textTheme.labelMedium),
              FilterChip(label: Text(s.matchCase), selected: _matchCase, onSelected: (v) => setState(() {
                _matchCase = v;
                _match = -1;
              })),
              if (!_replacing && !_session.readOnly) TextButton(onPressed: () => setState(() => _replacing = true), child: Text(s.replace)),
              if (_replacing && !_session.readOnly) ...[
                field(_replacement, s.replaceWith, submit: (_) => _replaceOne()),
                OutlinedButton(onPressed: matches.isEmpty ? null : _replaceOne, child: Text(s.replace)),
                OutlinedButton(onPressed: matches.isEmpty ? null : _replaceEvery, child: Text(s.replaceAll)),
              ],
              IconButton(tooltip: s.close, iconSize: 18, onPressed: _closeFind, icon: const Icon(Icons.close)),
            ],
          ),
        ),
      ),
    );
  }

  // keys

  /// The keys of Word the editor answers wherever the focus is in it.
  bool _shortcut(KeyEvent e) {
    if (e is KeyUpEvent) return false;
    final ctrl = HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed;
    final shift = HardwareKeyboard.instance.isShiftPressed;
    if (!ctrl) return false;
    final key = e.logicalKey;
    switch (key) {
      case LogicalKeyboardKey.keyF:
        _openFind();
        return true;
      case LogicalKeyboardKey.keyH:
        _openFind(replace: true);
        return true;
      case LogicalKeyboardKey.keyZ:
        _session.undo();
        return true;
      case LogicalKeyboardKey.keyY:
        _session.redo();
        return true;
    }
    if (_session.readOnly) return false;
    switch (key) {
      case LogicalKeyboardKey.keyM when HardwareKeyboard.instance.isAltPressed:
        _newComment();
        return true;
      case LogicalKeyboardKey.keyE when shift:
        _toggleTracking();
        return true;
      // Gras is Ctrl+G in the French version, Ctrl+B elsewhere
      case LogicalKeyboardKey.keyB || LogicalKeyboardKey.keyG:
        _toggle('b');
        return true;
      case LogicalKeyboardKey.keyI:
        _toggle('i');
        return true;
      case LogicalKeyboardKey.keyU:
        _toggleUnderline();
        return true;
      case LogicalKeyboardKey.keyE:
        _formatParagraphs({'jc': 'center'});
        return true;
      case LogicalKeyboardKey.keyL:
        _formatParagraphs({'jc': 'left'});
        return true;
      case LogicalKeyboardKey.keyR:
        _formatParagraphs({'jc': 'right'});
        return true;
      case LogicalKeyboardKey.keyJ:
        _formatParagraphs({'jc': 'both'});
        return true;
      case LogicalKeyboardKey.equal when shift:
        _toggleVertical('superscript');
        return true;
      case LogicalKeyboardKey.equal:
        _toggleVertical('subscript');
        return true;
      case LogicalKeyboardKey.space:
        _formatRuns({for (final k in wordRunKeys) k: ''});
        return true;
      case LogicalKeyboardKey.bracketRight || LogicalKeyboardKey.greater:
        _growFont(true);
        return true;
      case LogicalKeyboardKey.bracketLeft || LogicalKeyboardKey.less:
        _growFont(false);
        return true;
    }
    return false;
  }

  // building

  double _scale(BoxConstraints box) {
    if (!_fitWidth) return _zoom * _pointsToPixels;
    final widest = _current.pages.fold(0.0, (w, p) => p.size.width > w ? p.size.width : w);
    return ((box.maxWidth - 40) / (widest == 0 ? 612 : widest)).clamp(0.1, 5.0);
  }

  @override
  Widget build(BuildContext context) {
    final status = _session.status;
    if (!_session.loaded) {
      final failure = _session.failure;
      return Center(
        child: status == DocStatus.closed
            ? _Failure(message: '$failure', retry: _s.retry, onRetry: _session.retry)
            : const CircularProgressIndicator(),
      );
    }
    final layout = _current;
    _ensureCaret();
    final phone = MediaQuery.sizeOf(context).width < phoneWidth;
    if (phone && !_fitWidth) _fitWidth = true;
    final comments = _commentsShown ?? (!phone && _commentThreads.isNotEmpty);
    return Stack(
      children: [
        Column(
          children: [
            _ribbon(context),
            ..._banners(context),
            if (_finding) _findBar(context),
            Expanded(
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Expanded(
                    child: ColoredBox(
                      color: Theme.of(context).colorScheme.surfaceContainerHighest,
                      child: LayoutBuilder(
                        builder: (context, box) => WordPagesView(
                          key: _view,
                          session: _session,
                          layout: layout,
                          selection: _selection,
                          painter: PagePainter(images: (m) => _media[m], context: _context),
                          scale: _scale(box),
                          focusNode: _focus,
                          onShortcut: _shortcut,
                          onContextMenu: _contextMenu,
                          marks: _commentMarks,
                          highlights: [for (final (n, a, b) in _matches) (n.id, a, b)],
                          changed: [for (final r in _revisions) (r.flow, r.start, r.end)],
                          trackAs: _trackAs,
                        ),
                      ),
                    ),
                  ),
                  if (comments && !phone) ...[
                    const VerticalDivider(width: 1),
                    SizedBox(width: 320, child: _commentsPane(context)),
                  ],
                ],
              ),
            ),
            _statusBar(context),
          ],
        ),
        if (comments && phone) Positioned.fill(child: SafeArea(child: _commentsPane(context))),
        if (_backstage) _Backstage(editor: this),
      ],
    );
  }

  List<Widget> _banners(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Widget banner(IconData icon, String text, Color color, {VoidCallback? action, String? label}) => Material(
      color: color,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        child: Row(children: [
          Icon(icon, size: 18),
          const SizedBox(width: 8),
          Expanded(child: Text(text)),
          if (action != null) TextButton(onPressed: action, child: Text(label!)),
        ]),
      ),
    );
    final error = _session.saveError;
    return [
      if (error != null) banner(Icons.error_outline, _s.saveFailed(error), scheme.errorContainer),
      if (_session.status == DocStatus.offline)
        banner(Icons.cloud_off, _s.offline, scheme.tertiaryContainer, action: _session.retry, label: _s.retry),
      if (_session.status == DocStatus.closed) banner(Icons.block, '${_session.failure ?? ''}', scheme.errorContainer),
      if (_session.readOnly) banner(Icons.visibility_outlined, _s.readOnly, scheme.secondaryContainer),
      if (_tracking && !_session.readOnly && _session.name.isEmpty) banner(Icons.person_off_outlined, _s.untracked, scheme.errorContainer),
      if (_selection.area != PageArea.body)
        banner(Icons.vertical_split_outlined, _s.headerFooter, scheme.primaryContainer, action: _closeHeaderFooter, label: _s.closeHeaderFooter),
    ];
  }

  Widget _ribbon(BuildContext context) {
    final editable = !_session.readOnly && _flow != null;
    final run = _runProps;
    final para = _paraProps;
    final doc = _document;
    final fonts = <String>{
      '+major', '+minor', 'Calibri', 'Calibri Light', 'Cambria', 'Arial', 'Times New Roman', 'Courier New', 'Aptos', //
      if (run['font'] != null) run['font']!,
    }.toList();
    String fontName(String f) => f == '+major'
        ? '${doc.typeface(f)} (En-têtes)'
        : f == '+minor'
        ? '${doc.typeface(f)} (Corps)'
        : f;
    final theme = {for (final e in doc.themeColors.entries) if (hexColor(e.value) != null) e.key: hexColor(e.value)!};
    PopupMenuEntry<Map<String, Object?>?> palette(ValueChanged<Color?> pick, {String? none}) => PopupMenuItem(
      enabled: false,
      padding: EdgeInsets.zero,
      child: Builder(
        builder: (context) => ColorPalette(
          theme: theme,
          themeLabel: _s.themeColors,
          standardLabel: _s.standardColors,
          noneLabel: none,
          onSelected: (c) {
            Navigator.of(context).pop();
            pick(c == null ? null : ColorPalette.resolve(theme, c));
          },
        ),
      ),
    );
    String hex(Color c) => c.toARGB32().toRadixString(16).padLeft(8, '0').substring(2).toUpperCase();
    final jc = para['jc'] ?? 'left';
    final styles = _quickStyles;
    final level = doc.levelOf(para['num'], para['lvl']);
    final inCell = _cell != null;
    return Ribbon(
      fileLabel: _s.file,
      onFile: () => setState(() => _backstage = true),
      leading: [
        IconButton(tooltip: '${_s.undo} (Ctrl+Z)', iconSize: 18, onPressed: _session.canUndo ? () => setState(() => _session.undo()) : null, icon: const Icon(Icons.undo)),
        IconButton(tooltip: '${_s.redo} (Ctrl+Y)', iconSize: 18, onPressed: _session.canRedo ? () => setState(() => _session.redo()) : null, icon: const Icon(Icons.redo)),
      ],
      trailing: [
        IconButton(
          tooltip: _s.comments,
          iconSize: 18,
          isSelected: _commentsShown ?? _commentThreads.isNotEmpty,
          onPressed: () => setState(() => _commentsShown = !(_commentsShown ?? _commentThreads.isNotEmpty)),
          icon: const Icon(Icons.forum_outlined),
        ),
        for (final p in _session.peers.take(5))
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 2),
            child: Tooltip(
              message: p.name,
              child: CircleAvatar(radius: 12, child: Text(p.name.isEmpty ? '?' : p.name.characters.first.toUpperCase(), style: const TextStyle(fontSize: 11))),
            ),
          ),
        const SizedBox(width: 8),
      ],
      tabs: [
        RibbonTab(_s.home, [
          RibbonGroup(_s.clipboard, [
            RibbonButton(icon: const Icon(Icons.content_paste), label: _s.paste, large: true, shortcut: 'Ctrl+V', onPressed: editable ? () => _clipboard(LogicalKeyboardKey.keyV) : null),
            RibbonButton(icon: const Icon(Icons.content_cut), label: _s.cut, shortcut: 'Ctrl+X', onPressed: editable && !_selection.collapsed ? () => _clipboard(LogicalKeyboardKey.keyX) : null),
            RibbonButton(icon: const Icon(Icons.content_copy), label: _s.copy, shortcut: 'Ctrl+C', onPressed: !_selection.collapsed ? () => _clipboard(LogicalKeyboardKey.keyC) : null),
          ]),
          RibbonGroup(_s.font, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonDropdown(label: _s.font, value: run['font'] ?? '+minor', values: fonts, display: fontName, onChanged: editable ? (f) => _formatRuns({'font': f}) : null),
              const SizedBox(width: 4),
              RibbonDropdown(
                label: _s.fontSize,
                width: 56,
                value: _size.toString().replaceAll(RegExp(r'\.0$'), ''),
                values: [for (final s in _sizes) s.toString().replaceAll(RegExp(r'\.0$'), '')],
                onChanged: editable ? (v) => _setSize(double.parse(v)) : null,
              ),
              RibbonButton(icon: const Icon(Icons.text_increase), label: _s.growFont, shortcut: 'Ctrl+>', onPressed: editable ? () => _growFont(true) : null),
              RibbonButton(icon: const Icon(Icons.text_decrease), label: _s.shrinkFont, shortcut: 'Ctrl+<', onPressed: editable ? () => _growFont(false) : null),
              RibbonButton(icon: const Icon(Icons.format_clear), label: _s.clearFormatting, onPressed: editable ? _clearFormatting : null),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.format_bold), label: _s.bold, shortcut: 'Ctrl+G', selected: run['b'] == '1', onPressed: editable ? () => _toggle('b') : null),
              RibbonButton(icon: const Icon(Icons.format_italic), label: _s.italic, shortcut: 'Ctrl+I', selected: run['i'] == '1', onPressed: editable ? () => _toggle('i') : null),
              RibbonButton(icon: const Icon(Icons.format_underline), label: _s.underline, shortcut: 'Ctrl+U', selected: (run['u'] ?? 'none') != 'none', onPressed: editable ? _toggleUnderline : null),
              RibbonButton(icon: const Icon(Icons.format_strikethrough), label: _s.strikethrough, selected: run['strike'] == '1', onPressed: editable ? () => _toggle('strike') : null),
              RibbonButton(icon: const Icon(Icons.subscript), label: _s.subscript, shortcut: 'Ctrl+=', selected: run['va'] == 'subscript', onPressed: editable ? () => _toggleVertical('subscript') : null),
              RibbonButton(icon: const Icon(Icons.superscript), label: _s.superscript, shortcut: 'Ctrl+Maj+=', selected: run['va'] == 'superscript', onPressed: editable ? () => _toggleVertical('superscript') : null),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.border_color),
                label: _s.highlight,
                enabled: editable,
                items: [palette((c) => _formatRuns({'shd': c == null ? '' : hex(c), 'hl': ''}), none: _s.noFill)],
                onSelected: (_) {},
              ),
              RibbonMenu<Map<String, Object?>?>(
                icon: const Icon(Icons.format_color_text),
                label: _s.fontColor,
                enabled: editable,
                items: [palette((c) => _formatRuns({'color': c == null ? 'auto' : hex(c)}), none: _s.automatic)],
                onSelected: (_) {},
              ),
            ]),
          ]),
          RibbonGroup(_s.paragraph, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.format_list_bulleted), label: _s.bullets, selected: level?.format == 'bullet', onPressed: editable ? () => _list('bullet') : null),
              RibbonButton(icon: const Icon(Icons.format_list_numbered), label: _s.numbering, selected: level != null && level.format != 'bullet', onPressed: editable ? () => _list('decimal') : null),
              RibbonButton(icon: const Icon(Icons.format_indent_decrease), label: _s.decreaseIndent, onPressed: editable ? () => _indent(-1) : null),
              RibbonButton(icon: const Icon(Icons.format_indent_increase), label: _s.increaseIndent, onPressed: editable ? () => _indent(1) : null),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              for (final (value, icon, label, key) in [
                ('left', Icons.format_align_left, _s.alignLeft, 'Ctrl+L'),
                ('center', Icons.format_align_center, _s.center, 'Ctrl+E'),
                ('right', Icons.format_align_right, _s.alignRight, 'Ctrl+R'),
                ('both', Icons.format_align_justify, _s.justify, 'Ctrl+J'),
              ])
                RibbonButton(
                  icon: Icon(icon),
                  label: label,
                  shortcut: key,
                  selected: jc == value || (value == 'left' && jc == 'start') || (value == 'right' && jc == 'end'),
                  onPressed: editable ? () => _formatParagraphs({'jc': value}) : null,
                ),
              RibbonMenu<String>(
                icon: const Icon(Icons.format_line_spacing),
                label: _s.lineSpacing,
                enabled: editable,
                items: [
                  for (final l in const [1.0, 1.08, 1.15, 1.5, 2.0, 2.5, 3.0])
                    PopupMenuItem(value: 'line:$l', child: Text(l.toString().replaceAll('.', ','))),
                  const PopupMenuDivider(),
                  PopupMenuItem(value: 'before:1', child: Text(_s.addSpaceBefore)),
                  PopupMenuItem(value: 'before:0', child: Text(_s.removeSpaceBefore)),
                  PopupMenuItem(value: 'after:1', child: Text(_s.addSpaceAfter)),
                  PopupMenuItem(value: 'after:0', child: Text(_s.removeSpaceAfter)),
                ],
                onSelected: (v) {
                  final [what, value] = v.split(':');
                  switch (what) {
                    case 'line':
                      _lineSpacing(double.parse(value));
                    case 'before':
                      _spacing('sp.before', value == '1');
                    case 'after':
                      _spacing('sp.after', value == '1');
                  }
                },
              ),
            ]),
          ]),
          RibbonGroup(_s.styles, [
            _StyleGallery(
              styles: [for (final e in styles.take(8)) (e.key, _s.styleName(e.value.name))],
              current: doc.styleOf(_flow == null ? const {} : wordEditing(_flow!).markAt(_selection.start)),
              enabled: editable,
              onSelected: _style,
            ),
            RibbonMenu<String>(
              icon: const Icon(Icons.expand_more),
              label: _s.styles,
              enabled: editable,
              items: [for (final e in styles) PopupMenuItem(value: e.key, child: Text(_s.styleName(e.value.name)))],
              onSelected: _style,
            ),
          ]),
          RibbonGroup(_s.editing, [
            RibbonButton(icon: const Icon(Icons.search), label: _s.find, shortcut: 'Ctrl+F', onPressed: () => _openFind()),
            RibbonButton(icon: const Icon(Icons.find_replace), label: _s.replace, shortcut: 'Ctrl+H', onPressed: editable ? () => _openFind(replace: true) : null),
          ]),
        ]),
        RibbonTab(_s.insert, [
          RibbonGroup(_s.pages, [
            RibbonButton(icon: const Icon(Icons.insert_page_break_outlined), label: _s.pageBreak, large: true, shortcut: 'Ctrl+Entrée', onPressed: editable ? _pageBreak : null),
          ]),
          if (widget.onPicture != null)
            RibbonGroup(_s.illustrations, [
              RibbonButton(icon: const Icon(Icons.image_outlined), label: _s.pictures, large: true, onPressed: editable ? _picture : null),
            ]),
          RibbonGroup(_s.tables, [
            RibbonMenu<(int, int)>(
              icon: const Icon(Icons.table_chart_outlined),
              label: _s.table,
              large: true,
              enabled: editable && !inCell,
              items: [
                PopupMenuItem(
                  enabled: false,
                  child: Builder(builder: (context) => _TableGrid(
                    strings: _s,
                    onSelected: (cols, rows) {
                      Navigator.of(context).pop();
                      _insertTable(rows, cols);
                    },
                  )),
                ),
              ],
              onSelected: (_) {},
            ),
          ]),
          RibbonGroup(_s.comments, [
            RibbonButton(icon: const Icon(Icons.add_comment_outlined), label: _s.newComment, large: true, shortcut: 'Ctrl+Alt+M', onPressed: editable ? _newComment : null),
          ]),
          RibbonGroup(_s.headerFooter, [
            RibbonButton(icon: const Icon(Icons.vertical_align_top), label: _s.header, large: true, onPressed: editable ? () => _headerFooter(footer: false) : null),
            RibbonButton(icon: const Icon(Icons.vertical_align_bottom), label: _s.footer, large: true, onPressed: editable ? () => _headerFooter(footer: true) : null),
            RibbonButton(icon: const Icon(Icons.numbers), label: _s.pageNumber, large: true, onPressed: editable ? () => _headerFooter(footer: true, pageNumber: true) : null),
          ]),
        ]),
        RibbonTab(_s.layoutTab, [
          RibbonGroup(_s.pageSetup, [
            RibbonMenu<(int, int, int, int)>(
              icon: const Icon(Icons.margin),
              label: _s.margins,
              large: true,
              enabled: editable,
              items: [
                PopupMenuItem(value: (1417, 1417, 1417, 1417), child: Text('${_s.normalMargins} (2,5 cm)')),
                PopupMenuItem(value: (720, 720, 720, 720), child: Text('${_s.narrowMargins} (1,27 cm)')),
                PopupMenuItem(value: (1440, 1080, 1440, 1080), child: Text('${_s.moderateMargins} (2,54 × 1,91 cm)')),
                PopupMenuItem(value: (1440, 2880, 1440, 2880), child: Text('${_s.wideMargins} (2,54 × 5,08 cm)')),
              ],
              onSelected: (m) => _margins(m.$1, m.$2, m.$3, m.$4),
            ),
            RibbonMenu<bool>(
              icon: const Icon(Icons.crop_rotate),
              label: _s.orientation,
              large: true,
              enabled: editable,
              items: [PopupMenuItem(value: false, child: Text(_s.portrait)), PopupMenuItem(value: true, child: Text(_s.landscape))],
              onSelected: _orientation,
            ),
            RibbonMenu<(int, int)>(
              icon: const Icon(Icons.description_outlined),
              label: _s.size,
              large: true,
              enabled: editable,
              items: [for (final (name, w, h) in _s.paperSizes) PopupMenuItem(value: (w, h), child: Text(name))],
              onSelected: (s) => _paperSize(s.$1, s.$2),
            ),
            RibbonMenu<int>(
              icon: const Icon(Icons.view_column_outlined),
              label: _s.columns,
              large: true,
              enabled: editable,
              items: [for (final n in const [1, 2, 3]) PopupMenuItem(value: n, child: Text('$n'))],
              onSelected: _columns,
            ),
            RibbonButton(icon: const Icon(Icons.insert_page_break_outlined), label: _s.breaks, large: true, onPressed: editable ? _pageBreak : null),
          ]),
        ]),
        RibbonTab(_s.review, [
          RibbonGroup(_s.comments, [
            RibbonButton(icon: const Icon(Icons.add_comment_outlined), label: _s.newComment, large: true, shortcut: 'Ctrl+Alt+M', onPressed: editable ? _newComment : null),
            RibbonButton(
              icon: const Icon(Icons.delete_outline),
              label: _s.delete,
              large: true,
              onPressed: !_session.readOnly && _active != null ? () => _deleteComment(_active!.root) : null,
            ),
            RibbonButton(icon: const Icon(Icons.arrow_upward), label: _s.previousComment, large: true, onPressed: _commentThreads.isEmpty ? null : () => _stepThread(-1)),
            RibbonButton(icon: const Icon(Icons.arrow_downward), label: _s.nextComment, large: true, onPressed: _commentThreads.isEmpty ? null : () => _stepThread(1)),
            RibbonButton(
              icon: const Icon(Icons.forum_outlined),
              label: _s.showComments,
              large: true,
              selected: _commentsShown ?? _commentThreads.isNotEmpty,
              onPressed: () => setState(() => _commentsShown = !(_commentsShown ?? _commentThreads.isNotEmpty)),
            ),
          ]),
          RibbonGroup(_s.tracking, [
            RibbonButton(
              icon: const Icon(Icons.track_changes),
              label: _s.trackChanges,
              large: true,
              shortcut: 'Ctrl+Maj+E',
              selected: _tracking,
              onPressed: _session.readOnly ? null : _toggleTracking,
            ),
          ]),
          RibbonGroup(_s.changes, [
            RibbonMenu<String>(
              icon: const Icon(Icons.check_circle_outline),
              label: _s.accept,
              large: true,
              enabled: !_session.readOnly && _revisions.isNotEmpty,
              items: _settleItems(true),
              onSelected: (c) => _settleChosen(true, c),
            ),
            RibbonMenu<String>(
              icon: const Icon(Icons.cancel_outlined),
              label: _s.reject,
              large: true,
              enabled: !_session.readOnly && _revisions.isNotEmpty,
              items: _settleItems(false),
              onSelected: (c) => _settleChosen(false, c),
            ),
            RibbonButton(icon: const Icon(Icons.arrow_upward), label: _s.previousChange, large: true, onPressed: _revisions.isEmpty ? null : () => _stepRevision(-1)),
            RibbonButton(icon: const Icon(Icons.arrow_downward), label: _s.nextChange, large: true, onPressed: _revisions.isEmpty ? null : () => _stepRevision(1)),
          ]),
        ]),
        RibbonTab(_s.view, [
          RibbonGroup(_s.zoom, [
            RibbonButton(icon: const Icon(Icons.zoom_in), label: '100 %', large: true, selected: !_fitWidth && _zoom == 1, onPressed: () => setState(() {
              _fitWidth = false;
              _zoom = 1;
            })),
            RibbonButton(icon: const Icon(Icons.fit_screen_outlined), label: _s.pageWidth, large: true, selected: _fitWidth, onPressed: () => setState(() => _fitWidth = true)),
          ]),
        ]),
      ],
    );
  }

  Widget _statusBar(BuildContext context) {
    final layout = _current;
    final flow = _flow;
    final at = flow == null ? null : layout.caret(flow.id, _selection.extent, page: _selection.page);
    final page = (at?.$1 ?? 0) + 1;
    final state = _session.saveError != null
        ? _s.saveFailed(_session.saveError!)
        : _session.status == DocStatus.connecting
        ? _s.connecting
        : _session.saved
        ? _s.saved
        : _s.saving;
    final style = Theme.of(context).textTheme.labelSmall;
    final phone = MediaQuery.sizeOf(context).width < phoneWidth;
    final revision = phone ? null : _revision;
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainer,
      child: SizedBox(
        height: 26,
        child: Row(children: [
          const SizedBox(width: 12),
          Text(_s.pageOf(page, layout.pages.length), style: style),
          const SizedBox(width: 16),
          if (!phone) ...[Text(_s.words(_words), style: style), const SizedBox(width: 16), Text(_s.language, style: style), const SizedBox(width: 16)],
          if (_tracking || !phone) ...[
            Flexible(
              child: InkWell(
                onTap: _session.readOnly ? null : _toggleTracking,
                child: phone
                    ? const Icon(Icons.track_changes, size: 14)
                    : Text(_s.trackChangesState(_tracking), style: style, maxLines: 1, overflow: TextOverflow.ellipsis),
              ),
            ),
            const SizedBox(width: 16),
          ],
          if (revision != null) ...[
            Flexible(
              child: Text(
                _s.revision(revision.deleted, revision.author, revision.date),
                style: style?.copyWith(color: revisionColor(revision.author)),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            const SizedBox(width: 16),
          ],
          Icon(_session.saveError != null ? Icons.error_outline : (_session.saved ? Icons.cloud_done_outlined : Icons.cloud_upload_outlined), size: 14),
          const SizedBox(width: 4),
          Expanded(child: Text(state, style: style, overflow: TextOverflow.ellipsis)),
          if (!phone) ...[
            IconButton(iconSize: 16, tooltip: _s.zoom, onPressed: () => setState(() => _zoom = (_zoom - 0.1).clamp(0.1, 5)), icon: const Icon(Icons.remove)),
            SizedBox(
              width: 120,
              child: Slider(
                value: _zoom.clamp(0.1, 5),
                min: 0.1,
                max: 5,
                onChanged: (v) => setState(() {
                  _fitWidth = false;
                  _zoom = v;
                }),
              ),
            ),
            IconButton(iconSize: 16, tooltip: _s.zoom, onPressed: () => setState(() => _zoom = (_zoom + 0.1).clamp(0.1, 5)), icon: const Icon(Icons.add)),
            SizedBox(width: 44, child: Text('${(_zoom * 100).round()} %', style: style)),
          ],
          const SizedBox(width: 8),
        ]),
      ),
    );
  }

  /// The words of the body.
  int get _words {
    var n = 0;
    for (final f in flowsOf(_session.document)) {
      n += RegExp(r'[^\s\v￼]+').allMatches(f.text!.text).length;
    }
    return n;
  }
}

/// The styles of the gallery of the Home tab.
class _StyleGallery extends StatelessWidget {
  const _StyleGallery({required this.styles, required this.current, required this.enabled, required this.onSelected});

  final List<(String, String)> styles;
  final String? current;
  final bool enabled;
  final ValueChanged<String> onSelected;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      height: 56,
      child: Wrap(
        direction: Axis.vertical,
        spacing: 2,
        runSpacing: 2,
        children: [
          for (final (id, name) in styles)
            InkWell(
              onTap: enabled ? () => onSelected(id) : null,
              child: Container(
                width: 92,
                height: 26,
                alignment: Alignment.centerLeft,
                padding: const EdgeInsets.symmetric(horizontal: 6),
                decoration: BoxDecoration(
                  border: Border.all(color: id == current ? scheme.primary : scheme.outlineVariant),
                  color: id == current ? scheme.primaryContainer : null,
                ),
                child: Text(name, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 12)),
              ),
            ),
        ],
      ),
    );
  }
}

/// The grid Word offers to pick the size of a new table.
class _TableGrid extends StatefulWidget {
  const _TableGrid({required this.strings, required this.onSelected});

  final LofficeStrings strings;
  final void Function(int columns, int rows) onSelected;

  @override
  State<_TableGrid> createState() => _TableGridState();
}

class _TableGridState extends State<_TableGrid> {
  var _columns = 1, _rows = 1;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(widget.strings.tableSize(_columns, _rows), style: Theme.of(context).textTheme.labelMedium),
        const SizedBox(height: 6),
        for (var r = 1; r <= 8; r++)
          Row(mainAxisSize: MainAxisSize.min, children: [
            for (var c = 1; c <= 10; c++)
              MouseRegion(
                onEnter: (_) => setState(() {
                  _columns = c;
                  _rows = r;
                }),
                child: GestureDetector(
                  onTap: () => widget.onSelected(c, r),
                  child: Container(
                    width: 18,
                    height: 18,
                    margin: const EdgeInsets.all(1),
                    decoration: BoxDecoration(
                      color: c <= _columns && r <= _rows ? scheme.primaryContainer : null,
                      border: Border.all(color: c <= _columns && r <= _rows ? scheme.primary : scheme.outlineVariant),
                    ),
                  ),
                ),
              ),
          ]),
      ],
    );
  }
}

/// The backstage of the File tab: what the document is, and closing it.
class _Backstage extends StatelessWidget {
  const _Backstage({required this.editor});

  final _WordEditorState editor;

  @override
  Widget build(BuildContext context) {
    final s = editor._s;
    final session = editor._session;
    final scheme = Theme.of(context).colorScheme;
    return Material(
      color: scheme.surface,
      child: Row(
        children: [
          Container(
            width: 220,
            color: scheme.primary,
            child: ListTileTheme(
              textColor: scheme.onPrimary,
              iconColor: scheme.onPrimary,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  ListTile(leading: const Icon(Icons.arrow_back), title: Text(s.back), onTap: editor._closeBackstage),
                  ListTile(leading: const Icon(Icons.info_outline), title: Text(s.info), selected: true, selectedColor: scheme.onPrimary),
                  if (editor.widget.onClose != null) ListTile(leading: const Icon(Icons.close), title: Text(s.close), onTap: editor.widget.onClose),
                ],
              ),
            ),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(32),
              children: [
                Text(s.info, style: Theme.of(context).textTheme.headlineMedium),
                const SizedBox(height: 16),
                Text(editor.widget.title, style: Theme.of(context).textTheme.titleLarge),
                const SizedBox(height: 16),
                Text(session.saveError != null ? s.saveFailed(session.saveError!) : (session.saved ? s.saved : s.saving)),
                Text('${s.pageCount(editor._current.pages.length)} · ${s.words(editor._words)}'),
                if (session.readOnly) Text(s.readOnly),
                const SizedBox(height: 16),
                for (final p in session.peers) ListTile(leading: const Icon(Icons.person_outline), title: Text(p.name)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _Failure extends StatelessWidget {
  const _Failure({required this.message, required this.retry, required this.onRetry});

  final String message;
  final String retry;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      Icon(Icons.error_outline, size: 40, color: Theme.of(context).colorScheme.error),
      const SizedBox(height: 12),
      Text(message, textAlign: TextAlign.center),
      const SizedBox(height: 12),
      FilledButton(onPressed: onRetry, child: Text(retry)),
    ],
  );
}
