import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chart/chart.dart';
import '../chrome/ribbon.dart';
import '../chrome/strings.dart';
import '../clipboard.dart';
import 'charts.dart';
import 'edits.dart';
import 'filter.dart';
import 'filter_panel.dart';
import 'formula_text.dart';
import 'input.dart';
import 'lists.dart';
import 'number_format.dart';
import 'sheet_view.dart';
import 'workbook.dart';

/// An Excel editor on a session whose document is a workbook: the ribbon,
/// the formula bar, the cells of a sheet, its tabs and the status bar.
class SpreadsheetEditor extends StatefulWidget {
  const SpreadsheetEditor({
    super.key,
    required this.session,
    this.title = '',
    this.onClose,
    this.strings = const LofficeStrings(),
    this.fonts,
    this.hosted = false,
  });

  final DocSession session;
  final String title;
  final VoidCallback? onClose;
  final LofficeStrings strings;

  /// The package bundling the free fonts standing in for Office's.
  final String? fonts;

  /// Whether a host draws undo, redo and who else is here in a bar of its
  /// own: the ribbon then leaves them out.
  final bool hosted;

  @override
  State<SpreadsheetEditor> createState() => _SpreadsheetEditorState();
}

class _SpreadsheetEditorState extends State<SpreadsheetEditor> {
  final _selection = SheetSelection();
  final _gridFocus = FocusNode();
  final _cellFocus = FocusNode();
  final _barFocus = FocusNode();
  final _text = TextEditingController();
  final _nameBox = TextEditingController();
  final _view = GlobalKey<SheetViewState>();
  StreamSubscription<String>? _rejections;
  StreamSubscription<Edit>? _changes;
  void Function()? _unwatchPaste;
  Workbook? _book;
  String? _sheetId;
  var _editing = false;

  /// The cell being edited.
  var _editCell = (1, 1);
  var _zoom = 1.0;
  var _backstage = false;
  Clip? _clip;

  static const _locale = NumberLocale.fr;
  static const _peerColors = [
    Color(0xFFD83B01), Color(0xFF107C10), Color(0xFF8764B8), Color(0xFF0078D4), Color(0xFFC239B3), Color(0xFF00B7C3), //
  ];

  DocSession get _session => widget.session;
  LofficeStrings get _s => widget.strings;

  @override
  void initState() {
    super.initState();
    _nameBox.text = 'A1';
    _session.addListener(_repaint);
    _selection.addListener(_selected);
    _changes = _session.changes.listen(_changed);
    _unwatchPaste = watchPaste(_pasted);
    _rejections = _session.rejections.listen((reason) {
      if (!mounted) return;
      ScaffoldMessenger.maybeOf(context)?.showSnackBar(SnackBar(content: Text(_s.refused(reason))));
    });
  }

  @override
  void dispose() {
    _session.removeListener(_repaint);
    _selection.removeListener(_selected);
    _unwatchPaste?.call();
    unawaited(_rejections?.cancel());
    unawaited(_changes?.cancel());
    _selection.dispose();
    _gridFocus.dispose();
    _cellFocus.dispose();
    _barFocus.dispose();
    _text.dispose();
    _nameBox.dispose();
    super.dispose();
  }

  void _repaint() {
    if (mounted) setState(() {});
  }

  /// The workbook shown; its formats are read again when new ones come.
  Workbook get _wb => _book ??= Workbook(_session.document);

  void _changed(Edit edit) {
    if (edit.changes.any((c) => c.kind == ChangeKind.create && c.type == 'xf' || c.id == 'book')) _book = null;
    if (!_editing) _loadText();
  }

  SheetEdits get _edits => SheetEdits(_wb);

  List<Node> get _visibleSheets => [for (final s in _wb.sheets) if (s.attributes['state'] == null) s];

  Node? get _sheet {
    final sheets = _wb.sheets;
    if (sheets.isEmpty) return null;
    final current = sheets.where((s) => s.id == _sheetId).firstOrNull;
    if (current != null) return current;
    final active = _session.document['book']?.attributes['active'];
    return sheets.where((s) => s.id == active).firstOrNull ?? _visibleSheets.firstOrNull ?? sheets.first;
  }

  bool _edit(Edit edit) {
    if (edit.isEmpty) return false;
    return _session.edit(edit);
  }

  // selection and the text of the active cell

  Map<String, Object?> _fields(int row, int col) => _sheet?.grid?.cell(row, col) ?? const {};

  CellStyle get _activeStyle {
    final sheet = _sheet;
    if (sheet == null) return _wb.style(null);
    final (r, c) = _selection.active;
    return _wb.style(_edits.styleAt(sheet, r, c));
  }

  void _selected() {
    final a = _selection.area;
    _nameBox.text = a.single ? cellName(a.top, a.left) : '${cellName(a.top, a.left)}:${cellName(a.bottom, a.right)}';
    if (_editing) _commit();
    _loadText();
    final sheet = _sheet;
    if (sheet != null) _session.select(DocSelection.cells(sheet.id, a.top, a.left, a.bottom, a.right));
    _repaint();
  }

  void _loadText() {
    final (r, c) = _selection.active;
    _text.text = editText(_fields(r, c), locale: _locale, format: _activeStyle.format, date1904: _wb.date1904);
  }

  // editing a cell

  void _startEdit({String? typed, bool bar = false}) {
    if (_session.readOnly || _sheet?.grid == null) return;
    _editCell = _selection.active;
    setState(() => _editing = true);
    if (typed != null) {
      _text.value = TextEditingValue(text: typed, selection: TextSelection.collapsed(offset: typed.length));
    } else {
      _loadText();
      _text.selection = TextSelection.collapsed(offset: _text.text.length);
    }
    (bar ? _barFocus : _cellFocus).requestFocus();
  }

  void _cancel() {
    setState(() => _editing = false);
    _loadText();
    _gridFocus.requestFocus();
  }

  /// Ends editing: what was typed goes into its cell once the list that
  /// validates the cell takes it. Tells whether it went in.
  bool _commit() {
    final sheet = _sheet;
    if (!_editing || sheet == null) return true;
    _editing = false;
    final (r, c) = _editCell;
    final typed = _text.text;
    final rule = _wb.layout(sheet).listAt(r, c);
    final items = rule?.error == null || typed.startsWith('=') ? null : rule!.items(_wb, sheet, _locale);
    if (items != null && !rule!.takes(typed, items)) {
      unawaited(_refuse(rule, sheet, r, c, typed));
      setState(() {});
      return false;
    }
    _write(sheet, r, c, typed);
    setState(() {});
    return true;
  }

  void _write(Node sheet, int r, int c, String typed) {
    final input = parseInput(typed, locale: _locale, date1904: _wb.date1904);
    final current = sheet.grid?.cell(r, c) ?? const {};
    final changed = input.fields.entries.any((e) => current[e.key] != e.value);
    if (changed) _edit(_edits.setCells(sheet, [(r, c, input.fields)], format: input.format));
  }

  /// Says, as Excel does, that a value is not one of the list's, and
  /// writes it, edits it again or lets it go as the list and the user say.
  Future<void> _refuse(ListRule rule, Node sheet, int r, int c, String typed) async {
    final warning = rule.error == 'warning';
    final text = rule.errorText.isEmpty ? _s.notInList : rule.errorText;
    final choice = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        icon: Icon(switch (rule.error) {
          'warning' => Icons.warning_amber,
          'information' => Icons.info_outline,
          _ => Icons.cancel_outlined,
        }),
        title: Text(rule.errorTitle.isEmpty ? _s.dataValidation : rule.errorTitle),
        content: Text(warning ? '$text\n\n${_s.continueQuestion}' : text),
        actions: [
          for (final (value, label) in switch (rule.error) {
            'warning' => [('write', _s.yes), ('retry', _s.no), ('cancel', _s.cancel)],
            'information' => [('write', _s.ok), ('cancel', _s.cancel)],
            _ => [('retry', _s.retry), ('cancel', _s.cancel)],
          })
            TextButton(onPressed: () => Navigator.pop(context, value), child: Text(label)),
        ],
      ),
    );
    if (!mounted) return;
    switch (choice) {
      case 'write':
        _write(sheet, r, c, typed);
      case 'retry':
        setState(() => _sheetId = sheet.id);
        _selection.select(r, c);
        _startEdit(typed: typed);
      default:
        _loadText();
        _gridFocus.requestFocus();
    }
  }

  /// Ends editing and moves the active cell, as Enter and Tab do.
  void _commitAndStep(int rows, int cols) {
    if (_commit()) _selection.step(rows, cols);
    _gridFocus.requestFocus();
  }

  /// The list validating the active cell, when it has values to offer.
  (ListRule, List<String>)? get _activeList {
    final sheet = _sheet;
    if (sheet == null || _session.readOnly) return null;
    final (r, c) = _selection.active;
    final rule = _wb.layout(sheet).listAt(r, c);
    final items = rule != null && rule.arrow ? rule.items(_wb, sheet, _locale) : null;
    return items == null ? null : (rule!, items);
  }

  /// Opens the values of the active cell's list under it; the one picked
  /// goes into the cell.
  Future<void> _openList(Rect cell) async {
    final list = _activeList;
    final sheet = _sheet;
    if (list == null || sheet == null) return;
    final (r, c) = _selection.active;
    final screen = MediaQuery.sizeOf(context);
    final picked = await showMenu<String>(
      context: context,
      position: RelativeRect.fromLTRB(cell.left, cell.bottom, screen.width - cell.right, 0),
      constraints: BoxConstraints(minWidth: math.max(cell.width, 80), maxHeight: 8 * 32.0),
      items: [for (final v in list.$2) PopupMenuItem(value: v, height: 32, child: Text(v))],
    );
    if (picked == null || !mounted) return;
    _write(sheet, r, c, picked);
    _gridFocus.requestFocus();
  }

  /// What the list of the active cell asks to be typed, as Excel shows it
  /// beside the cell.
  Widget? get _prompt {
    final sheet = _sheet;
    if (sheet == null) return null;
    final (r, c) = _selection.active;
    final rule = _wb.layout(sheet).listAt(r, c);
    if (rule == null || rule.prompt.isEmpty && rule.promptTitle.isEmpty) return null;
    return IgnorePointer(
      child: Container(
        constraints: const BoxConstraints(maxWidth: 240),
        padding: const EdgeInsets.all(6),
        decoration: BoxDecoration(color: const Color(0xFFFFFFE1), border: Border.all(color: const Color(0xFF767676))),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: [
          if (rule.promptTitle.isNotEmpty) Text(rule.promptTitle, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.bold, color: Colors.black)),
          if (rule.prompt.isNotEmpty) Text(rule.prompt, style: const TextStyle(fontSize: 12, color: Colors.black)),
        ]),
      ),
    );
  }

  KeyEventResult _editorKey(FocusNode _, KeyEvent e) {
    if (e is KeyUpEvent) return KeyEventResult.ignored;
    final shift = HardwareKeyboard.instance.isShiftPressed;
    final alt = HardwareKeyboard.instance.isAltPressed;
    switch (e.logicalKey) {
      case LogicalKeyboardKey.enter || LogicalKeyboardKey.numpadEnter:
        if (alt) {
          final v = _text.value;
          final text = v.text.replaceRange(v.selection.start, v.selection.end, '\n');
          _text.value = TextEditingValue(text: text, selection: TextSelection.collapsed(offset: v.selection.start + 1));
          return KeyEventResult.handled;
        }
        _commitAndStep(shift ? -1 : 1, 0);
        return KeyEventResult.handled;
      case LogicalKeyboardKey.tab:
        _commitAndStep(0, shift ? -1 : 1);
        return KeyEventResult.handled;
      case LogicalKeyboardKey.escape:
        _cancel();
        return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  // the keys of Excel on the grid

  KeyEventResult _gridKey(FocusNode _, KeyEvent e) {
    if (e is KeyUpEvent || _editing) return KeyEventResult.ignored;
    final keys = HardwareKeyboard.instance;
    final ctrl = keys.isControlPressed || keys.isMetaPressed;
    final shift = keys.isShiftPressed;
    final key = e.logicalKey;
    final (r, c) = _selection.area.single || !shift ? _selection.active : _focus;
    void go(int row, int col) {
      if (shift) {
        _selection.select(row, col, extend: true);
      } else {
        _selection.select(row, col);
      }
    }

    if (ctrl) {
      // left to the browser, which then tells the page to paste
      if (key == LogicalKeyboardKey.keyV && _unwatchPaste != null) return KeyEventResult.ignored;
      switch (key) {
        case LogicalKeyboardKey.keyZ:
          _session.undo();
        case LogicalKeyboardKey.keyY:
          _session.redo();
        case LogicalKeyboardKey.keyC:
          _copy();
        case LogicalKeyboardKey.keyX:
          _copy(cut: true);
        case LogicalKeyboardKey.keyV:
          unawaited(_paste());
        case LogicalKeyboardKey.keyA:
          _selection.selectArea(const CellArea(1, 1, maxRows, maxCols));
        // Gras is Ctrl+G in the French version, Ctrl+B elsewhere
        case LogicalKeyboardKey.keyB || LogicalKeyboardKey.keyG:
          _toggleFont('b', _activeStyle.font.bold);
        case LogicalKeyboardKey.keyI:
          _toggleFont('i', _activeStyle.font.italic);
        case LogicalKeyboardKey.keyU:
          _setFont('u', _activeStyle.font.underline ? null : 'single');
        case LogicalKeyboardKey.keyL when shift:
          _toggleFilter();
        case LogicalKeyboardKey.home:
          go(1, 1);
        case LogicalKeyboardKey.end:
          final l = _wb.layout(_sheet!);
          go(math.max(1, l.used), math.max(1, l.usedCols));
        case LogicalKeyboardKey.arrowDown:
          go(_edge(r, c, 1, 0), c);
        case LogicalKeyboardKey.arrowUp:
          go(_edge(r, c, -1, 0), c);
        case LogicalKeyboardKey.arrowRight:
          go(r, _edge(r, c, 0, 1));
        case LogicalKeyboardKey.arrowLeft:
          go(r, _edge(r, c, 0, -1));
        case LogicalKeyboardKey.pageDown:
          _stepSheet(1);
        case LogicalKeyboardKey.pageUp:
          _stepSheet(-1);
        case LogicalKeyboardKey.semicolon:
          final now = DateTime.now();
          _setActive(serialOf(now.year, now.month, now.day, date1904: _wb.date1904), 'dd/mm/yyyy');
        case LogicalKeyboardKey.space:
          final a = _selection.area;
          _selection.selectArea(CellArea(1, a.left, maxRows, a.right), active: _selection.active);
        case LogicalKeyboardKey.equal || LogicalKeyboardKey.add || LogicalKeyboardKey.numpadAdd:
          _insert(rows: _selection.area.wholeRows);
        case LogicalKeyboardKey.minus || LogicalKeyboardKey.numpadSubtract:
          _delete(rows: _selection.area.wholeRows);
        default:
          return KeyEventResult.ignored;
      }
      return KeyEventResult.handled;
    }
    switch (key) {
      case LogicalKeyboardKey.arrowDown when keys.isAltPressed:
        final view = _view.currentState;
        if (view != null) unawaited(_openList(view.activeScreenRect));
      case LogicalKeyboardKey.arrowDown:
        go(r + 1, c);
      case LogicalKeyboardKey.arrowUp:
        go(r - 1, c);
      case LogicalKeyboardKey.arrowRight:
        go(r, c + 1);
      case LogicalKeyboardKey.arrowLeft:
        go(r, c - 1);
      case LogicalKeyboardKey.pageDown:
        go(r + (_view.currentState?.pageRows ?? 20), c);
      case LogicalKeyboardKey.pageUp:
        go(r - (_view.currentState?.pageRows ?? 20), c);
      case LogicalKeyboardKey.home:
        go(r, 1);
      case LogicalKeyboardKey.enter || LogicalKeyboardKey.numpadEnter:
        _selection.step(shift ? -1 : 1, 0);
      case LogicalKeyboardKey.tab:
        _selection.step(0, shift ? -1 : 1);
      case LogicalKeyboardKey.f2:
        _startEdit();
      case LogicalKeyboardKey.delete:
        _edit(_edits.clear(_sheet!, _selection.area));
      case LogicalKeyboardKey.backspace:
        _startEdit(typed: '');
      case LogicalKeyboardKey.space when shift:
        final a = _selection.area;
        _selection.selectArea(CellArea(a.top, 1, a.bottom, maxCols), active: _selection.active);
      default:
        final ch = e.character;
        if (ch == null || ch.isEmpty || ch.codeUnitAt(0) < 0x20 || keys.isAltPressed) return KeyEventResult.ignored;
        _startEdit(typed: ch);
    }
    return KeyEventResult.handled;
  }

  /// The corner of the selection opposite the active cell, which Shift
  /// with the arrows moves.
  (int, int) get _focus {
    final a = _selection.area;
    final (r, c) = _selection.active;
    return (r == a.top ? a.bottom : a.top, c == a.left ? a.right : a.left);
  }

  /// Where Ctrl with an arrow goes: the edge of the data.
  int _edge(int r, int c, int dr, int dc) {
    bool filled(int row, int col) {
      final f = _fields(row, col);
      return f['v'] != null || f['f'] != null || f['e'] != null;
    }

    final l = _wb.layout(_sheet!);
    final limitR = dr > 0 ? math.max(l.used, r) : 1, limitC = dc > 0 ? math.max(l.usedCols, c) : 1;
    var (row, col) = (r, c);
    bool inside(int row, int col) => dr > 0 ? row < limitR : (dr < 0 ? row > 1 : (dc > 0 ? col < limitC : col > 1));
    if (!inside(row, col)) return dr != 0 ? (dr > 0 ? maxRows : 1) : (dc > 0 ? maxCols : 1);
    final start = filled(row, col) && filled(row + dr, col + dc);
    while (inside(row, col)) {
      row += dr;
      col += dc;
      if (start ? !filled(row + dr, col + dc) : filled(row, col)) break;
    }
    if (!filled(row, col) && !inside(row, col)) return dr != 0 ? (dr > 0 ? maxRows : 1) : (dc > 0 ? maxCols : 1);
    return dr != 0 ? row : col;
  }

  void _setActive(double v, String format) {
    final (r, c) = _selection.active;
    _edit(_edits.setCells(_sheet!, [(r, c, {'v': v, 'f': null, 'e': null})], format: format));
  }

  // formats

  void _format(StyleChange change, {Object? Function(int row, int col)? key}) {
    final sheet = _sheet;
    if (sheet == null || _session.readOnly) return;
    _edit(_edits.format(sheet, _selection.area, change, key: key));
  }

  static Map<String, Object?> _part(Map<String, Object?> style, String key) =>
      {...?(style[key] as Map<String, Object?>?)};

  void _setFont(String key, Object? value) => _format((s, _, _) => {...s, 'font': {..._part(s, 'font'), key: value}});

  void _toggleFont(String key, bool on) => _setFont(key, on ? null : true);

  void _setFontName(String name) =>
      _format((s, _, _) => {...s, 'font': {..._part(s, 'font'), 'name': name, 'scheme': null}});

  void _growFont(bool up) {
    final now = _activeStyle.font.size;
    final next = up
        ? _sizes.firstWhere((s) => s > now + 0.01, orElse: () => now + 12)
        : _sizes.lastWhere((s) => s < now - 0.01, orElse: () => math.max(1, now - 1));
    _setFont('sz', next);
  }

  static const _sizes = <double>[8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 36, 48, 72];

  void _align(String key, Object? value) => _format((s, _, _) => {...s, 'align': {..._part(s, 'align'), key: value}});

  void _setFill(Map<String, Object?>? color) =>
      _format((s, _, _) => {...s, 'fill': color == null ? null : {'pattern': 'solid', 'fg': color}});

  void _setNumberFormat(String code) => _format((s, _, _) => {...s, 'fmt': code});

  void _decimals(int by) {
    final code = _activeStyle.formatCode;
    _setNumberFormat(changeDecimals(code, by));
  }

  void _border(String kind) {
    final a = _selection.area;
    const thin = {'style': 'thin', 'color': {'auto': true}};
    _format(
      (s, r, c) {
        final b = _part(s, 'border');
        switch (kind) {
          case 'none':
            return {...s, 'border': null};
          case 'all':
            return {...s, 'border': {...b, 'l': thin, 'r': thin, 't': thin, 'b': thin}};
          case 'outside':
            return {
              ...s,
              'border': {
                ...b,
                if (c == a.left || a.wholeRows) 'l': thin,
                if (c == a.right || a.wholeRows) 'r': thin,
                if (r == a.top || a.wholeCols) 't': thin,
                if (r == a.bottom || a.wholeCols) 'b': thin,
              },
            };
          case 'thickBottom':
            return r == a.bottom || a.wholeCols ? {...s, 'border': {...b, 'b': {'style': 'thick', 'color': {'auto': true}}}} : s;
        }
        final side = {'bottom': 'b', 'top': 't', 'left': 'l', 'right': 'r'}[kind]!;
        final edge = switch (side) {
          'b' => r == a.bottom || a.wholeCols,
          't' => r == a.top || a.wholeCols,
          'l' => c == a.left || a.wholeRows,
          _ => c == a.right || a.wholeRows,
        };
        return edge ? {...s, 'border': {...b, side: thin}} : s;
      },
      key: (r, c) => (r == a.top, r == a.bottom, c == a.left, c == a.right),
    );
  }

  void _mergeCenter() {
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    final merged = _wb.layout(sheet).merges.any((m) => m.intersects(a));
    if (merged) {
      _edit(_edits.unmerge(sheet, a));
    } else if (!a.single && !a.wholeCols && !a.wholeRows) {
      _edit(_edits.merge(sheet, a));
    }
  }

  // rows, columns and sheets

  void _insert({required bool rows}) {
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    _edit(Edit([
      rows ? Change.insert(sheet.id, dimRows, a.top, a.rows) : Change.insert(sheet.id, dimCols, a.left, a.cols),
    ]));
  }

  void _delete({required bool rows}) {
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    _edit(Edit([
      rows ? Change.remove(sheet.id, dimRows, a.top, a.rows) : Change.remove(sheet.id, dimCols, a.left, a.cols),
    ]));
  }

  void _resize(String dim, int index, double size) {
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    // every column or row selected whole takes the size given to one of them
    final indexes = dim == dimCols
        ? (a.wholeCols && index >= a.left && index <= a.right ? [for (var c = a.left; c <= a.right; c++) c] : [index])
        : (a.wholeRows && index >= a.top && index <= a.bottom ? [for (var r = a.top; r <= a.bottom; r++) r] : [index]);
    _edit(_edits.resize(sheet, dim, indexes, size));
  }

  void _freeze(int rows, int cols) {
    final sheet = _sheet;
    if (sheet == null) return;
    _edit(Edit([
      Change.set(sheet.id, attributes: {'frozen': rows == 0 && cols == 0 ? null : {'r': ?(rows > 0 ? rows : null), 'c': ?(cols > 0 ? cols : null)}}),
    ]));
  }

  void _closeBackstage() => setState(() => _backstage = false);

  void _goToSheet(Node sheet) {
    if (_editing) _commit();
    setState(() => _sheetId = sheet.id);
    _selection.select(1, 1);
  }

  void _stepSheet(int by) {
    final sheets = _visibleSheets;
    final i = sheets.indexWhere((s) => s.id == _sheet?.id);
    if (i < 0 || sheets.isEmpty) return;
    _goToSheet(sheets[(i + by).clamp(0, sheets.length - 1)]);
  }

  void _newSheet() {
    final change = _edits.newSheet(_edits.freeName(_s.sheetName), after: _sheet);
    if (_edit(Edit([change]))) _goToSheet(_session.document[change.id]!);
  }

  Future<void> _renameSheet(Node sheet) async {
    final controller = TextEditingController(text: '${sheet.attributes['name'] ?? ''}');
    final name = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(_s.renameSheet),
        content: TextField(
          controller: controller,
          autofocus: true,
          maxLength: 31,
          decoration: InputDecoration(labelText: _s.newSheetName),
          onSubmitted: (v) => Navigator.of(context).pop(v),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(_s.cancel)),
          FilledButton(onPressed: () => Navigator.of(context).pop(controller.text), child: Text(_s.ok)),
        ],
      ),
    );
    controller.dispose();
    final trimmed = name?.trim() ?? '';
    if (trimmed.isEmpty || trimmed == sheet.attributes['name']) return;
    _edit(Edit([Change.set(sheet.id, attributes: {'name': trimmed})]));
  }

  Future<void> _deleteSheet(Node sheet) async {
    if (_visibleSheets.length <= 1) return;
    final sure = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        content: Text(_s.deleteSheetConfirm('${sheet.attributes['name'] ?? ''}')),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(false), child: Text(_s.cancel)),
          FilledButton(onPressed: () => Navigator.of(context).pop(true), child: Text(_s.delete)),
        ],
      ),
    );
    if (sure != true) return;
    final sheets = _visibleSheets;
    final i = sheets.indexWhere((s) => s.id == sheet.id);
    if (_edit(Edit([Change.delete(sheet.id)]))) {
      final rest = _visibleSheets;
      if (rest.isNotEmpty) _goToSheet(rest[i.clamp(0, rest.length - 1)]);
    }
  }

  // clipboard

  void _copy({bool cut = false}) {
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    _clip = _edits.copy(sheet, a);
    unawaited(Clipboard.setData(ClipboardData(text: _tsv(sheet, a))));
    if (cut) _edit(_edits.clear(sheet, a));
  }

  /// The cells of an area as text, a tab between cells, as Excel copies
  /// them for other applications.
  String _tsv(Node sheet, CellArea a) {
    final l = _wb.layout(sheet);
    final rows = <String>[];
    for (var r = a.top; r <= math.min(a.bottom, math.max(l.used, a.top)); r++) {
      final cells = <String>[];
      for (var c = a.left; c <= math.min(a.right, math.max(l.usedCols, a.left)); c++) {
        final f = _fields(r, c);
        cells.add(cellText(f, _wb.style(f['s'] as String?), _locale, date1904: _wb.date1904).text);
      }
      rows.add(cells.join('\t'));
    }
    return rows.join('\n');
  }

  Future<void> _paste() async => _pasteText((await Clipboard.getData(Clipboard.kTextPlain))?.text);

  bool _pasted(String text) {
    if (_editing || !_gridFocus.hasFocus) return false;
    _pasteText(text);
    return true;
  }

  void _pasteText(String? text) {
    final sheet = _sheet;
    if (sheet == null || _session.readOnly) return;
    final (r, c) = _selection.active;
    final clip = _clip;
    if (clip != null && (text == null || text == _tsv(sheet, clip.area) || text.isEmpty)) {
      _edit(_edits.paste(sheet, clip, r, c));
      _selection.selectArea(CellArea(r, c, r + clip.area.rows - 1, c + clip.area.cols - 1), active: (r, c));
      return;
    }
    if (text == null || text.isEmpty) return;
    final lines = text.replaceAll('\r\n', '\n').split('\n');
    if (lines.length > 1 && lines.last.isEmpty) lines.removeLast();
    final values = <(int, int, Map<String, Object?>)>[];
    for (var i = 0; i < lines.length; i++) {
      final cells = lines[i].split('\t');
      for (var j = 0; j < cells.length; j++) {
        values.add((r + i, c + j, parseInput(cells[j], locale: _locale, date1904: _wb.date1904).fields));
      }
    }
    _edit(_edits.setCells(sheet, values));
  }

  // editing commands

  void _autoSum() {
    final sheet = _sheet;
    if (sheet == null) return;
    final (r, c) = _selection.active;
    bool number(int row, int col) => _fields(row, col)['v'] is num;
    var top = r - 1;
    while (top >= 1 && number(top, c)) {
      top--;
    }
    // a filtered list is summed as Excel does, leaving out the rows hidden
    final filter = SheetFilter.of(sheet);
    final filtered = filter != null &&
        filter.columns.isNotEmpty &&
        filter.area.left <= c &&
        c <= filter.area.right &&
        top < filter.rows.$2 &&
        r - 1 >= filter.rows.$1;
    String formula;
    if (top < r - 1) {
      final range = '${cellName(top + 1, c)}:${cellName(r - 1, c)}';
      formula = filtered ? 'SUBTOTAL(9,$range)' : 'SUM($range)';
    } else {
      var left = c - 1;
      while (left >= 1 && number(r, left)) {
        left--;
      }
      formula = left < c - 1 ? 'SUM(${cellName(r, left + 1)}:${cellName(r, c - 1)})' : '';
    }
    if (formula.isEmpty) {
      _startEdit(typed: '=SOMME()');
      _text.selection = TextSelection.collapsed(offset: _text.text.length - 1);
      return;
    }
    _edit(_edits.setCells(sheet, [(r, c, {'f': formula, 'v': null, 'e': null})]));
  }

  void _insertFunction(String name) {
    if (!_editing) _startEdit(typed: '=');
    final v = _text.value;
    final at = v.selection.isValid ? v.selection.start : v.text.length;
    final text = v.text.replaceRange(at, v.selection.isValid ? v.selection.end : at, '$name(');
    _text.value = TextEditingValue(text: text, selection: TextSelection.collapsed(offset: at + name.length + 1));
  }

  void _sort({required bool descending}) {
    final sheet = _sheet;
    if (sheet == null) return;
    var a = _selection.area;
    if (a.single) {
      final l = _wb.layout(sheet);
      a = CellArea(1, 1, math.max(l.used, 1), math.max(l.usedCols, 1));
    }
    _edit(_edits.sort(sheet, a, _selection.active.$2, descending: descending));
  }

  // filters

  void _toggleFilter() {
    final sheet = _sheet;
    if (sheet == null) return;
    if (SheetFilter.of(sheet) != null) {
      _edit(filterEdit(_wb, sheet, null, _locale));
      return;
    }
    final a = _selection.area;
    _edit(filterEdit(_wb, sheet, SheetFilter(a.single ? currentRegion(sheet, a.top, a.left) : a), _locale));
  }

  /// Applies the filter of the sheet again, its columns cleared when
  /// [clear].
  void _refilter({bool clear = false}) {
    final sheet = _sheet;
    final f = sheet == null ? null : SheetFilter.of(sheet);
    if (f == null) return;
    _edit(filterEdit(_wb, sheet!, clear ? SheetFilter(f.area) : f, _locale));
  }

  Future<void> _openFilter(int col, Rect at) async {
    final sheet = _sheet;
    final f = sheet == null ? null : SheetFilter.of(sheet);
    if (f == null) return;
    final (values, blanks) = columnValues(_wb, sheet!, f, col, _locale);
    final header = shownText(_wb, sheet.grid?.cell(f.area.top, col), _locale);
    final choice = await showFilterPanel(
      context,
      at: at,
      name: header.isEmpty ? columnName(col) : header,
      values: values,
      blanks: blanks,
      current: f.columns[col - f.area.left],
      strings: _s,
    );
    if (choice == null || !mounted) return;
    switch (choice.action) {
      case FilterAction.sortAscending || FilterAction.sortDescending:
        final (first, last) = f.rows;
        _edit(_edits.sort(sheet, CellArea(first, f.area.left, last, f.area.right), col, descending: choice.action == FilterAction.sortDescending));
        _refilter();
      case FilterAction.clear:
        _edit(filterEdit(_wb, sheet, f.withColumn(col, null), _locale));
      case FilterAction.show:
        _edit(filterEdit(_wb, sheet, f.withColumn(col, choice.column), _locale));
    }
    _gridFocus.requestFocus();
  }

  void _goToName() {
    final area = parseArea(_nameBox.text.trim().toUpperCase());
    if (area != null) _selection.selectArea(area);
    _gridFocus.requestFocus();
  }

  Future<void> _cellsMenu(Offset at, {bool rows = false, bool cols = false}) async {
    final editable = !_session.readOnly;
    final choice = await showMenu<String>(
      context: context,
      position: RelativeRect.fromLTRB(at.dx, at.dy, at.dx, at.dy),
      items: [
        PopupMenuItem(value: 'cut', enabled: editable, child: Text(_s.cut)),
        PopupMenuItem(value: 'copy', child: Text(_s.copy)),
        PopupMenuItem(value: 'paste', enabled: editable, child: Text(_s.paste)),
        const PopupMenuDivider(),
        PopupMenuItem(value: 'insertRows', enabled: editable, child: Text(_s.insertRows)),
        PopupMenuItem(value: 'insertCols', enabled: editable, child: Text(_s.insertColumns)),
        PopupMenuItem(value: 'deleteRows', enabled: editable, child: Text(_s.deleteRows)),
        PopupMenuItem(value: 'deleteCols', enabled: editable, child: Text(_s.deleteColumns)),
        PopupMenuItem(value: 'clear', enabled: editable, child: Text(_s.clearContents)),
        if (rows || cols) ...[
          const PopupMenuDivider(),
          PopupMenuItem(value: 'hide', enabled: editable, child: Text(_s.hide)),
          PopupMenuItem(value: 'unhide', enabled: editable, child: Text(_s.unhide)),
        ],
      ],
    );
    final sheet = _sheet;
    if (sheet == null) return;
    final a = _selection.area;
    switch (choice) {
      case 'cut':
        _copy(cut: true);
      case 'copy':
        _copy();
      case 'paste':
        await _paste();
      case 'insertRows':
        _insert(rows: true);
      case 'insertCols':
        _insert(rows: false);
      case 'deleteRows':
        _delete(rows: true);
      case 'deleteCols':
        _delete(rows: false);
      case 'clear':
        _edit(_edits.clear(sheet, a));
      case 'hide':
        _edit(_edits.resize(sheet, cols ? dimCols : dimRows, [
          for (var i = cols ? a.left : a.top; i <= (cols ? a.right : a.bottom); i++) i,
        ], 0));
      case 'unhide':
        final l = _wb.layout(sheet);
        _edit(_edits.resize(sheet, cols ? dimCols : dimRows, [
          for (var i = cols ? a.left : a.top; i <= (cols ? a.right : a.bottom); i++) i,
        ], cols ? l.defaultWidth : l.defaultHeight));
    }
  }

  List<PeerSelection> get _peers {
    final sheet = _sheet;
    if (sheet == null) return const [];
    var i = 0;
    return [
      for (final p in _session.peers)
        if (p.selection?.cells case final cells? when p.selection!.node == sheet.id)
          PeerSelection(p.name, _peerColors[i++ % _peerColors.length], CellArea(cells[0], cells[1], cells[2], cells[3])),
    ];
  }

  // building

  /// A chart sheet: its chart over the whole view, or what tells it is
  /// not shown.
  Widget _chartSheet(Node sheet) {
    final spec = ChartSpec.fromJson(sheet.attributes['chart']);
    if (spec == null) return Center(child: Text(_s.chartSheet));
    return LayoutBuilder(
      builder: (context, box) => CustomPaint(
        size: box.biggest,
        painter: _ChartSheetPainter(spec, _wb, sheet, widget.fonts),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (!_session.loaded) {
      final failure = _session.failure;
      return Center(
        child: _session.status == DocStatus.closed
            ? _Failure(message: '$failure', retry: _s.retry, onRetry: _session.retry)
            : const CircularProgressIndicator(),
      );
    }
    final sheet = _sheet;
    _sheetId = sheet?.id;
    return Stack(children: [
      Column(children: [
        _ribbon(context),
        ..._banners(context),
        _formulaBar(context),
        Expanded(
          child: sheet == null
              ? const SizedBox.expand()
              : sheet.grid == null
              ? _chartSheet(sheet)
              : Focus(
                  focusNode: _gridFocus,
                  autofocus: true,
                  onKeyEvent: _gridKey,
                  child: GestureDetector(
                    onTapDown: (_) {
                      if (!_editing) _gridFocus.requestFocus();
                    },
                    child: SheetView(
                      key: _view,
                      book: _wb,
                      sheet: sheet,
                      selection: _selection,
                      locale: _locale,
                      zoom: _zoom,
                      peers: _peers,
                      fonts: widget.fonts,
                      editing: _editing,
                      editor: _cellEditor(),
                      onEdit: _session.readOnly ? null : _startEdit,
                      onResize: _session.readOnly ? null : _resize,
                      onZoom: (z) => setState(() => _zoom = z),
                      onMenu: (at, {rows = false, cols = false}) => _cellsMenu(at, rows: rows, cols: cols),
                      onFill: _session.readOnly ? null : (from, to) => _edit(_edits.fill(sheet, from, to)),
                      onList: _activeList == null ? null : _openList,
                      prompt: _prompt,
                      filter: SheetFilter.of(sheet),
                      onFilter: _session.readOnly ? null : _openFilter,
                    ),
                  ),
                ),
        ),
        _tabs(context),
        _statusBar(context),
      ]),
      if (_backstage) _Backstage(editor: this),
    ]);
  }

  Widget _cellEditor() {
    final f = _activeStyle.font;
    return Focus(
      onKeyEvent: _editorKey,
      child: Material(
        color: Colors.white,
        shape: const Border.fromBorderSide(BorderSide(color: Color(0xFF217346), width: 2)),
        child: TextField(
          controller: _text,
          focusNode: _cellFocus,
          maxLines: null,
          style: TextStyle(
            fontFamily: substituteFont(f.name),
            package: widget.fonts,
            fontSize: f.size * 4 / 3 * _zoom,
            fontWeight: f.bold ? FontWeight.bold : FontWeight.normal,
            fontStyle: f.italic ? FontStyle.italic : FontStyle.normal,
            color: Colors.black,
          ),
          decoration: const InputDecoration(isCollapsed: true, border: InputBorder.none, contentPadding: EdgeInsets.fromLTRB(3, 2, 3, 0)),
        ),
      ),
    );
  }

  Widget _formulaBar(BuildContext context) {
    final border = BorderSide(color: Theme.of(context).dividerColor);
    return Container(
      height: 30,
      decoration: BoxDecoration(border: Border(bottom: border)),
      child: Row(children: [
        SizedBox(
          width: 110,
          child: TextField(
            controller: _nameBox,
            style: const TextStyle(fontSize: 13),
            textAlign: TextAlign.center,
            decoration: InputDecoration(isCollapsed: true, border: InputBorder.none, hintText: _s.nameBox, contentPadding: const EdgeInsets.all(7)),
            onSubmitted: (_) => _goToName(),
          ),
        ),
        VerticalDivider(width: 1, color: border.color),
        if (_editing) ...[
          IconButton(iconSize: 16, tooltip: _s.cancel, onPressed: _cancel, icon: const Icon(Icons.close)),
          IconButton(iconSize: 16, tooltip: _s.ok, onPressed: () => _commitAndStep(0, 0), icon: const Icon(Icons.check)),
        ],
        _functionsMenu(small: true),
        Expanded(
          child: Focus(
            onKeyEvent: _editorKey,
            child: TextField(
              controller: _text,
              focusNode: _barFocus,
              readOnly: _session.readOnly || _sheet?.grid == null,
              style: const TextStyle(fontSize: 13, fontFamily: 'Carlito'),
              decoration: const InputDecoration(isCollapsed: true, border: InputBorder.none, contentPadding: EdgeInsets.all(7)),
              onTap: () {
                if (!_editing) _startEdit(bar: true);
              },
            ),
          ),
        ),
      ]),
    );
  }

  static const _commonFunctions = [
    'SOMME', 'MOYENNE', 'NB', 'NBVAL', 'MAX', 'MIN', 'SI', 'SIERREUR', 'SOMME.SI', 'NB.SI', 'RECHERCHEV', 'RECHERCHEX', //
    'INDEX', 'EQUIV', 'CONCAT', 'TEXTE', 'ARRONDI', 'AUJOURDHUI', 'MAINTENANT', 'DATE', 'ANNEE', 'MOIS', 'JOUR',
  ];

  Widget _functionsMenu({bool small = false}) {
    final all = frenchFunctions.toSet().toList()..sort();
    return RibbonMenu<String>(
      icon: const Icon(Icons.functions),
      label: _s.insertFunction,
      large: !small,
      enabled: !_session.readOnly,
      items: [
        for (final f in _commonFunctions) PopupMenuItem(value: f, child: Text(f)),
        const PopupMenuDivider(),
        for (final f in all)
          if (!_commonFunctions.contains(f)) PopupMenuItem(value: f, child: Text(f, style: const TextStyle(fontSize: 12))),
      ],
      onSelected: _insertFunction,
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
      if (_wb.csv) banner(Icons.info_outline, _s.csvNotice, scheme.surfaceContainerHighest),
    ];
  }

  Widget _ribbon(BuildContext context) {
    final editable = !_session.readOnly && _sheet?.grid != null;
    final hasFilter = _sheet != null && SheetFilter.of(_sheet!) != null;
    final style = _activeStyle;
    final font = style.font;
    final colors = {for (var i = 0; i < 10; i++) themeNames[i]: _wb.themeColors[i]};
    final fonts = <String>{_wb.minorFont, _wb.majorFont, 'Calibri', 'Arial', 'Cambria', 'Times New Roman', 'Courier New', 'Aptos', font.name}.toList();
    String size(double s) => s.toString().replaceAll(RegExp(r'\.0$'), '');
    final formats = _s.numberFormats;
    final currentFormat = formats.where((f) => f.$1 == style.formatCode).firstOrNull?.$2 ?? style.formatCode;
    PopupMenuEntry<Object?> palette(ValueChanged<Map<String, Object?>?> pick, {String? none}) => PopupMenuItem(
      enabled: false,
      padding: EdgeInsets.zero,
      child: Builder(
        builder: (context) => ColorPalette(
          theme: colors,
          themeLabel: _s.themeColors,
          standardLabel: _s.standardColors,
          noneLabel: none,
          onSelected: (c) {
            Navigator.of(context).pop();
            pick(cellColor(c));
          },
        ),
      ),
    );
    final wrap = style.wrap;
    final h = style.horizontal;
    final v = style.vertical;
    final frozen = _sheet == null ? null : _wb.layout(_sheet!);
    return Ribbon(
      fileLabel: _s.file,
      onFile: () => setState(() => _backstage = true),
      leading: [
        if (!widget.hosted) ...[
          IconButton(
            tooltip: '${_s.undo} (Ctrl+Z)',
            iconSize: 18,
            onPressed: _session.canUndo ? () => setState(_session.undo) : null,
            icon: const Icon(Icons.undo),
          ),
          IconButton(
            tooltip: '${_s.redo} (Ctrl+Y)',
            iconSize: 18,
            onPressed: _session.canRedo ? () => setState(_session.redo) : null,
            icon: const Icon(Icons.redo),
          ),
        ],
      ],
      trailing: [
        if (!widget.hosted)
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
      initialTab: 0,
      tabs: [
        RibbonTab(_s.home, [
          RibbonGroup(_s.clipboard, [
            RibbonButton(icon: const Icon(Icons.content_paste), label: _s.paste, large: true, shortcut: 'Ctrl+V', onPressed: editable ? _paste : null),
            RibbonButton(icon: const Icon(Icons.content_cut), label: _s.cut, shortcut: 'Ctrl+X', onPressed: editable ? () => _copy(cut: true) : null),
            RibbonButton(icon: const Icon(Icons.content_copy), label: _s.copy, shortcut: 'Ctrl+C', onPressed: _copy),
          ]),
          RibbonGroup(_s.font, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonDropdown(label: _s.font, value: font.name, values: fonts, onChanged: editable ? _setFontName : null),
              const SizedBox(width: 4),
              RibbonDropdown(
                label: _s.fontSize,
                width: 56,
                value: size(font.size),
                values: [for (final s in _sizes) size(s)],
                onChanged: editable ? (v) => _setFont('sz', double.parse(v)) : null,
              ),
              RibbonButton(icon: const Icon(Icons.text_increase), label: _s.growFont, onPressed: editable ? () => _growFont(true) : null),
              RibbonButton(icon: const Icon(Icons.text_decrease), label: _s.shrinkFont, onPressed: editable ? () => _growFont(false) : null),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.format_bold), label: _s.bold, shortcut: 'Ctrl+G', selected: font.bold, onPressed: editable ? () => _toggleFont('b', font.bold) : null),
              RibbonButton(icon: const Icon(Icons.format_italic), label: _s.italic, shortcut: 'Ctrl+I', selected: font.italic, onPressed: editable ? () => _toggleFont('i', font.italic) : null),
              RibbonButton(
                icon: const Icon(Icons.format_underline),
                label: _s.underline,
                shortcut: 'Ctrl+U',
                selected: font.underline,
                onPressed: editable ? () => _setFont('u', font.underline ? null : 'single') : null,
              ),
              RibbonMenu<String>(
                icon: const Icon(Icons.border_all),
                label: _s.borders,
                enabled: editable,
                items: [
                  for (final (kind, label, icon) in [
                    ('bottom', _s.bottomBorder, Icons.border_bottom),
                    ('top', _s.topBorder, Icons.border_top),
                    ('left', _s.leftBorder, Icons.border_left),
                    ('right', _s.rightBorder, Icons.border_right),
                    ('none', _s.noBorder, Icons.border_clear),
                    ('all', _s.allBorders, Icons.border_all),
                    ('outside', _s.outsideBorders, Icons.border_outer),
                    ('thickBottom', _s.thickBottomBorder, Icons.border_bottom),
                  ])
                    PopupMenuItem(value: kind, child: Row(children: [Icon(icon, size: 18), const SizedBox(width: 8), Text(label)])),
                ],
                onSelected: _border,
              ),
              RibbonMenu<Object?>(
                icon: const Icon(Icons.format_color_fill),
                label: _s.fillColor,
                enabled: editable,
                items: [palette(_setFill, none: _s.noFill)],
                onSelected: (_) {},
              ),
              RibbonMenu<Object?>(
                icon: const Icon(Icons.format_color_text),
                label: _s.fontColor,
                enabled: editable,
                items: [palette((c) => _setFont('color', c), none: _s.automatic)],
                onSelected: (_) {},
              ),
            ]),
          ]),
          RibbonGroup(_s.alignment, [
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.vertical_align_top), label: _s.alignTop, selected: v == 'top', onPressed: editable ? () => _align('v', 'top') : null),
              RibbonButton(icon: const Icon(Icons.vertical_align_center), label: _s.alignMiddle, selected: v == 'center', onPressed: editable ? () => _align('v', 'center') : null),
              RibbonButton(icon: const Icon(Icons.vertical_align_bottom), label: _s.alignBottom, selected: v == 'bottom', onPressed: editable ? () => _align('v', null) : null),
              RibbonButton(icon: const Icon(Icons.wrap_text), label: _s.wrapText, selected: wrap, onPressed: editable ? () => _align('wrap', wrap ? null : true) : null),
            ]),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.format_align_left), label: _s.alignLeft, selected: h == 'left', onPressed: editable ? () => _align('h', h == 'left' ? null : 'left') : null),
              RibbonButton(icon: const Icon(Icons.format_align_center), label: _s.center, selected: h == 'center', onPressed: editable ? () => _align('h', h == 'center' ? null : 'center') : null),
              RibbonButton(icon: const Icon(Icons.format_align_right), label: _s.alignRight, selected: h == 'right', onPressed: editable ? () => _align('h', h == 'right' ? null : 'right') : null),
              RibbonButton(icon: const Icon(Icons.merge_type), label: _s.mergeAndCenter, onPressed: editable ? _mergeCenter : null),
            ]),
          ]),
          RibbonGroup(_s.number, [
            RibbonDropdown(
              label: _s.numberFormat,
              width: 130,
              value: currentFormat,
              values: {for (final f in formats) f.$2, currentFormat}.toList(),
              onChanged: editable ? (label) => _setNumberFormat(formats.firstWhere((f) => f.$2 == label, orElse: () => (label, label)).$1) : null,
            ),
            Row(mainAxisSize: MainAxisSize.min, children: [
              RibbonButton(icon: const Icon(Icons.euro), label: _s.accountingFormat, onPressed: editable ? () => _setNumberFormat(formats[3].$1) : null),
              RibbonButton(icon: const Icon(Icons.percent), label: _s.percentStyle, onPressed: editable ? () => _setNumberFormat('0%') : null),
              RibbonButton(icon: const Icon(Icons.more_horiz), label: _s.commaStyle, onPressed: editable ? () => _setNumberFormat('#,##0.00') : null),
              RibbonButton(icon: const Icon(Icons.add), label: _s.increaseDecimal, onPressed: editable ? () => _decimals(1) : null),
              RibbonButton(icon: const Icon(Icons.remove), label: _s.decreaseDecimal, onPressed: editable ? () => _decimals(-1) : null),
            ]),
          ]),
          RibbonGroup(_s.cells, [
            RibbonMenu<String>(
              icon: const Icon(Icons.add_box_outlined),
              label: _s.insertCells,
              large: true,
              enabled: editable,
              items: [
                PopupMenuItem(value: 'rows', child: Text(_s.insertRows)),
                PopupMenuItem(value: 'cols', child: Text(_s.insertColumns)),
                PopupMenuItem(value: 'sheet', enabled: !_wb.csv, child: Text(_s.insertSheet)),
              ],
              onSelected: (v) => v == 'sheet' ? _newSheet() : _insert(rows: v == 'rows'),
            ),
            RibbonMenu<String>(
              icon: const Icon(Icons.delete_outline),
              label: _s.deleteCells,
              large: true,
              enabled: editable,
              items: [
                PopupMenuItem(value: 'rows', child: Text(_s.deleteRows)),
                PopupMenuItem(value: 'cols', child: Text(_s.deleteColumns)),
                PopupMenuItem(value: 'sheet', child: Text(_s.deleteSheet)),
              ],
              onSelected: (v) => v == 'sheet' ? _deleteSheet(_sheet!) : _delete(rows: v == 'rows'),
            ),
          ]),
          RibbonGroup(_s.editing, [
            RibbonButton(icon: const Icon(Icons.functions), label: _s.autoSum, large: true, shortcut: 'Alt+=', onPressed: editable ? _autoSum : null),
            RibbonMenu<String>(
              icon: const Icon(Icons.sort_by_alpha),
              label: _s.sortAndFilter,
              enabled: editable,
              items: [
                PopupMenuItem(value: 'asc', child: Text(_s.sortAscending)),
                PopupMenuItem(value: 'desc', child: Text(_s.sortDescending)),
                const PopupMenuDivider(),
                PopupMenuItem(value: 'filter', child: Text(_s.filter)),
              ],
              onSelected: (v) => v == 'filter' ? _toggleFilter() : _sort(descending: v == 'desc'),
            ),
            RibbonMenu<String>(
              icon: const Icon(Icons.cleaning_services_outlined),
              label: _s.clear,
              enabled: editable,
              items: [
                PopupMenuItem(value: 'all', child: Text(_s.clearAll)),
                PopupMenuItem(value: 'formats', child: Text(_s.clearFormats)),
                PopupMenuItem(value: 'contents', child: Text(_s.clearContents)),
              ],
              onSelected: (v) {
                final sheet = _sheet!;
                final a = _selection.area;
                if (v != 'formats') _edit(_edits.clear(sheet, a));
                if (v != 'contents') _edit(_edits.clearFormats(sheet, a));
              },
            ),
          ]),
        ]),
        RibbonTab(_s.insert, [
          RibbonGroup(_s.cells, [
            RibbonButton(icon: const Icon(Icons.post_add), label: _s.insertSheet, large: true, onPressed: !_session.readOnly && !_wb.csv ? _newSheet : null),
            RibbonButton(icon: const Icon(Icons.table_rows_outlined), label: _s.insertRows, onPressed: editable ? () => _insert(rows: true) : null),
            RibbonButton(icon: const Icon(Icons.view_column_outlined), label: _s.insertColumns, onPressed: editable ? () => _insert(rows: false) : null),
          ]),
        ]),
        RibbonTab(_s.formulas, [
          RibbonGroup(_s.functionLibrary, [
            _functionsMenu(),
            RibbonButton(icon: const Icon(Icons.functions), label: _s.autoSum, large: true, onPressed: editable ? _autoSum : null),
          ]),
        ]),
        RibbonTab(_s.data, [
          RibbonGroup(_s.sortAndFilter, [
            RibbonButton(icon: const Icon(Icons.arrow_downward), label: _s.sortAscending, large: true, onPressed: editable ? () => _sort(descending: false) : null),
            RibbonButton(icon: const Icon(Icons.arrow_upward), label: _s.sortDescending, large: true, onPressed: editable ? () => _sort(descending: true) : null),
            RibbonButton(
              icon: const Icon(Icons.filter_alt_outlined),
              label: _s.filter,
              large: true,
              shortcut: 'Ctrl+Maj+L',
              selected: hasFilter,
              onPressed: editable ? _toggleFilter : null,
            ),
            RibbonButton(icon: const Icon(Icons.filter_alt_off_outlined), label: _s.clearFilters, onPressed: editable && hasFilter ? () => _refilter(clear: true) : null),
            RibbonButton(icon: const Icon(Icons.refresh), label: _s.reapply, onPressed: editable && hasFilter ? _refilter : null),
          ]),
        ]),
        RibbonTab(_s.view, [
          RibbonGroup(_s.window, [
            RibbonMenu<String>(
              icon: const Icon(Icons.grid_on),
              label: _s.freezePanes,
              large: true,
              enabled: editable,
              items: [
                if ((frozen?.frozenRows ?? 0) > 0 || (frozen?.frozenCols ?? 0) > 0)
                  PopupMenuItem(value: 'none', child: Text(_s.unfreezePanes))
                else
                  PopupMenuItem(value: 'here', child: Text(_s.freezePanes)),
                PopupMenuItem(value: 'row', child: Text(_s.freezeTopRow)),
                PopupMenuItem(value: 'col', child: Text(_s.freezeFirstColumn)),
              ],
              onSelected: (v) {
                final (r, c) = _selection.active;
                switch (v) {
                  case 'none':
                    _freeze(0, 0);
                  case 'here':
                    _freeze(r - 1, c - 1);
                  case 'row':
                    _freeze(1, 0);
                  case 'col':
                    _freeze(0, 1);
                }
              },
            ),
          ]),
          RibbonGroup(_s.show, [
            RibbonButton(
              icon: const Icon(Icons.border_clear),
              label: _s.gridlines,
              large: true,
              selected: frozen?.gridlines ?? true,
              onPressed: editable
                  ? () => _edit(Edit([Change.set(_sheet!.id, attributes: {'grid': (frozen?.gridlines ?? true) ? false : null})]))
                  : null,
            ),
          ]),
          RibbonGroup(_s.zoom, [
            RibbonButton(icon: const Icon(Icons.zoom_in), label: _s.zoom, large: true, onPressed: () => setState(() => _zoom = math.min(4, _zoom * 1.25))),
            RibbonButton(icon: const Icon(Icons.zoom_out), label: '100 %', large: true, onPressed: () => setState(() => _zoom = 1)),
          ]),
        ]),
      ],
    );
  }

  Widget _tabs(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final sheets = _visibleSheets;
    final editable = !_session.readOnly;
    return Container(
      height: 32,
      decoration: BoxDecoration(
        color: scheme.surfaceContainerLow,
        border: Border(top: BorderSide(color: Theme.of(context).dividerColor)),
      ),
      child: Row(children: [
        IconButton(iconSize: 16, onPressed: () => _stepSheet(-1), icon: const Icon(Icons.chevron_left)),
        IconButton(iconSize: 16, onPressed: () => _stepSheet(1), icon: const Icon(Icons.chevron_right)),
        Expanded(
          child: ReorderableListView.builder(
            scrollDirection: Axis.horizontal,
            buildDefaultDragHandles: false,
            itemCount: sheets.length,
            onReorderItem: (from, to) {
              if (editable) _edit(Edit([_edits.moveSheet(sheets[from], to > from ? to - 1 : to)]));
            },
            itemBuilder: (context, i) {
              final s = sheets[i];
              final selected = s.id == _sheet?.id;
              final tab = _wb.color(s.attributes['tab']);
              return ReorderableDelayedDragStartListener(
                key: ValueKey(s.id),
                index: i,
                enabled: editable,
                child: GestureDetector(
                  onTap: () => _goToSheet(s),
                  onDoubleTap: editable ? () => _renameSheet(s) : null,
                  onSecondaryTapDown: (d) => _sheetMenu(s, d.globalPosition),
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 14),
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: selected ? Colors.white : null,
                      border: Border(
                        bottom: BorderSide(color: selected ? const Color(0xFF217346) : (tab ?? Colors.transparent), width: 3),
                        right: BorderSide(color: Theme.of(context).dividerColor),
                      ),
                    ),
                    child: Text(
                      '${s.attributes['name'] ?? ''}',
                      style: TextStyle(fontSize: 12, fontWeight: selected ? FontWeight.bold : FontWeight.normal, color: selected ? const Color(0xFF217346) : null),
                    ),
                  ),
                ),
              );
            },
          ),
        ),
        if (!_wb.csv) IconButton(iconSize: 18, tooltip: _s.insertSheet, onPressed: editable ? _newSheet : null, icon: const Icon(Icons.add_circle_outline)),
      ]),
    );
  }

  Future<void> _sheetMenu(Node sheet, Offset at) async {
    final editable = !_session.readOnly;
    final hidden = [for (final s in _wb.sheets) if (s.attributes['state'] != null) s];
    final choice = await showMenu<String>(
      context: context,
      position: RelativeRect.fromLTRB(at.dx, at.dy, at.dx, at.dy),
      items: [
        PopupMenuItem(value: 'insert', enabled: editable && !_wb.csv, child: Text(_s.insertSheet)),
        PopupMenuItem(value: 'delete', enabled: editable && _visibleSheets.length > 1, child: Text(_s.deleteSheet)),
        PopupMenuItem(value: 'rename', enabled: editable, child: Text(_s.renameSheet)),
        PopupMenuItem(value: 'hide', enabled: editable && _visibleSheets.length > 1, child: Text(_s.hideSheet)),
        for (final h in hidden)
          PopupMenuItem(value: 'unhide:${h.id}', enabled: editable, child: Text('${_s.unhideSheet} « ${h.attributes['name']} »')),
      ],
    );
    switch (choice) {
      case 'insert':
        _newSheet();
      case 'delete':
        await _deleteSheet(sheet);
      case 'rename':
        await _renameSheet(sheet);
      case 'hide':
        if (_edit(Edit([Change.set(sheet.id, attributes: {'state': 'hidden'})]))) _stepSheet(0);
      case final String c when c.startsWith('unhide:'):
        _edit(Edit([Change.set(c.substring(7), attributes: {'state': null})]));
    }
  }

  Widget _statusBar(BuildContext context) {
    final sheet = _sheet;
    var count = 0, numbers = 0;
    var sum = 0.0;
    if (sheet?.grid != null) {
      final a = _selection.area;
      final l = _wb.layout(sheet!);
      for (final c in sheet.grid!.rows(a.top, a.bottom)) {
        if (c.row == 0 || c.col == 0 || !a.contains(c.row, c.col) || l.height(c.row) == 0 || l.width(c.col) == 0) continue;
        final v = c.fields['v'];
        if (v != null || c.fields['e'] != null) count++;
        if (v is num) {
          numbers++;
          sum += v;
        }
      }
    }
    String number(double n) => generalText(double.parse(n.toStringAsPrecision(12)), _locale);
    final state = _session.saveError != null
        ? _s.saveFailed(_session.saveError!)
        : _session.status == DocStatus.connecting
        ? _s.connecting
        : _session.saved
        ? _s.saved
        : _s.saving;
    final style = Theme.of(context).textTheme.labelSmall;
    // a phone shows the sum alone, and zooms with the fingers
    final phone = MediaQuery.sizeOf(context).width < phoneWidth;
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainer,
      child: SizedBox(
        height: 26,
        child: Row(children: [
          const SizedBox(width: 12),
          Icon(
            _session.saveError != null ? Icons.error_outline : (_session.saved ? Icons.cloud_done_outlined : Icons.cloud_upload_outlined),
            size: 14,
          ),
          const SizedBox(width: 4),
          Expanded(child: Text(state, style: style, overflow: TextOverflow.ellipsis)),
          if (count > 1) ...[
            if (numbers > 0 && !phone) Text('${_s.average} : ${number(sum / numbers)}', style: style),
            const SizedBox(width: 16),
            if (!phone) Text('${_s.countLabel} : $count', style: style),
            if (!phone) const SizedBox(width: 16),
            if (numbers > 0) Text('${_s.sum} : ${number(sum)}', style: style),
            const SizedBox(width: 16),
          ],
          if (phone) const SizedBox(width: 8) else ..._zoomControls(style),
        ]),
      ),
    );
  }

  List<Widget> _zoomControls(TextStyle? style) => [
          Text(_s.language, style: style),
          const SizedBox(width: 8),
          SizedBox(
            width: 120,
            child: Slider(
              value: math.log(_zoom) / math.ln2,
              min: math.log(0.1) / math.ln2,
              max: 2,
              onChanged: (v) => setState(() => _zoom = math.pow(2, v).toDouble()),
            ),
          ),
          Text('${(_zoom * 100).round()} %', style: style),
          const SizedBox(width: 12),
        ];
}

/// A number format with a decimal more, or less: 0 becomes 0.0, 0.00 %
/// becomes 0.0 %.
String changeDecimals(String code, int by) {
  if (code == 'General' || code.isEmpty) code = '0';
  final section = code.split(';').first;
  final m = RegExp(r'^(.*?[0#?])(\.([0#?]*))?([^0#?]*)$').firstMatch(section);
  if (m == null) return code;
  final decimals = m[3]?.length ?? 0;
  final n = math.max(0, decimals + by);
  final rebuilt = n == 0 ? '${m[1]}${m[4]}' : '${m[1]}.${'0' * n}${m[4]}';
  final rest = code.split(';').skip(1).map((s) => s.replaceFirst(RegExp(r'\.?[0#?]*(?=[^0#?]*$)'), n == 0 ? '' : '.${'0' * n}'));
  return [rebuilt, ...rest].join(';');
}

/// The backstage of the File tab: what the document is, and closing it.
class _Backstage extends StatelessWidget {
  const _Backstage({required this.editor});

  final _SpreadsheetEditorState editor;

  @override
  Widget build(BuildContext context) {
    final s = editor._s;
    final session = editor._session;
    final scheme = Theme.of(context).colorScheme;
    return Material(
      color: scheme.surface,
      child: Row(children: [
        Container(
          width: 220,
          color: const Color(0xFF217346),
          child: ListTileTheme(
            textColor: Colors.white,
            iconColor: Colors.white,
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              ListTile(leading: const Icon(Icons.arrow_back), title: Text(s.back), onTap: editor._closeBackstage),
              ListTile(leading: const Icon(Icons.info_outline), title: Text(s.info), selected: true, selectedColor: Colors.white),
              if (editor.widget.onClose != null) ListTile(leading: const Icon(Icons.close), title: Text(s.close), onTap: editor.widget.onClose),
            ]),
          ),
        ),
        Expanded(
          child: ListView(padding: const EdgeInsets.all(32), children: [
            Text(s.info, style: Theme.of(context).textTheme.headlineMedium),
            const SizedBox(height: 16),
            Text(editor.widget.title, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 16),
            Text(session.saveError != null ? s.saveFailed(session.saveError!) : (session.saved ? s.saved : s.saving)),
            if (session.readOnly) Text(s.readOnly),
            const SizedBox(height: 16),
            for (final p in session.peers) ListTile(leading: const Icon(Icons.person_outline), title: Text(p.name)),
          ]),
        ),
      ]),
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

class _ChartSheetPainter extends CustomPainter {
  _ChartSheetPainter(this.spec, this.book, this.sheet, this.fonts);

  final ChartSpec spec;
  final Workbook book;
  final Node sheet;
  final String? fonts;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(Offset.zero & size, Paint()..color = const Color(0xFFF3F3F3));
    paintSheetChart(canvas, (Offset.zero & size).deflate(16), spec, book, sheet, fonts: fonts);
  }

  @override
  bool shouldRepaint(_ChartSheetPainter old) => old.spec != spec || old.book != book || old.sheet != sheet;
}
