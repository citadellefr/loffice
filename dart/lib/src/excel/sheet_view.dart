import 'dart:math' as math;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import 'charts.dart';
import 'conditional.dart';
import 'filter.dart';
import 'number_format.dart';
import 'workbook.dart';

/// Where a sheet is selected: the active cell and the area around it.
class SheetSelection extends ChangeNotifier {
  var _active = (1, 1);
  var _anchor = (1, 1);
  var _focus = (1, 1);

  /// The cell typing goes to.
  (int, int) get active => _active;

  /// The area selected, from the anchor to where it was extended.
  CellArea get area => CellArea(_anchor.$1, _anchor.$2, _focus.$1, _focus.$2);

  /// Selects a cell alone, or extends the area to it.
  void select(int row, int col, {bool extend = false}) {
    row = row.clamp(1, maxRows);
    col = col.clamp(1, maxCols);
    if (extend) {
      _focus = (row, col);
    } else {
      _active = _anchor = _focus = (row, col);
    }
    notifyListeners();
  }

  /// Selects an area, the active cell at its first corner.
  void selectArea(CellArea a, {(int, int)? active}) {
    _anchor = (a.top, a.left);
    _focus = (a.bottom, a.right);
    _active = active ?? _anchor;
    notifyListeners();
  }

  /// Moves the active cell inside the area, as Enter and Tab do.
  void step(int rows, int cols) {
    final a = area;
    if (a.single) {
      select(_active.$1 + rows, _active.$2 + cols);
      return;
    }
    var (r, c) = _active;
    if (cols != 0) {
      c += cols;
      if (c > a.right) {
        c = a.left;
        r = r >= a.bottom ? a.top : r + 1;
      } else if (c < a.left) {
        c = a.right;
        r = r <= a.top ? a.bottom : r - 1;
      }
    } else {
      r += rows;
      if (r > a.bottom) {
        r = a.top;
        c = c >= a.right ? a.left : c + 1;
      } else if (r < a.top) {
        r = a.bottom;
        c = c <= a.left ? a.right : c - 1;
      }
    }
    _active = (r, c);
    notifyListeners();
  }
}

/// Someone else's selection, drawn in their color.
class PeerSelection {
  const PeerSelection(this.name, this.color, this.area);

  final String name;
  final Color color;
  final CellArea area;
}

const _headerHeight = 22.0;
const _gridColor = Color(0xFFD9D9D9);
const _headerColor = Color(0xFFF3F3F3);
const _selectColor = Color(0xFF217346);

/// The cells of a sheet as Excel draws them, scrolled, zoomed and
/// selected with the mouse, a finger or a pen.
class SheetView extends StatefulWidget {
  const SheetView({
    super.key,
    required this.book,
    required this.sheet,
    required this.selection,
    required this.locale,
    this.zoom = 1,
    this.peers = const [],
    this.fonts,
    this.editing = false,
    this.editor,
    this.onEdit,
    this.onResize,
    this.onZoom,
    this.onMenu,
    this.onFill,
    this.onList,
    this.prompt,
    this.filter,
    this.onFilter,
  });

  final Workbook book;
  final Node sheet;
  final SheetSelection selection;
  final NumberLocale locale;
  final double zoom;
  final List<PeerSelection> peers;
  final String? fonts;

  /// Whether the active cell is being edited, [editor] drawn over it.
  final bool editing;
  final Widget? editor;

  /// Asks to edit the active cell.
  final VoidCallback? onEdit;

  /// Sets the width of a column (dim "c") or the height of a row ("r"), in
  /// pixels at 100 %.
  final void Function(String dim, int index, double size)? onResize;
  final ValueChanged<double>? onZoom;

  /// Opens the menu of the cells, at a point of the screen.
  final void Function(Offset at, {bool rows, bool cols})? onMenu;

  /// Fills from the selection to an area, as the fill handle does.
  final void Function(CellArea from, CellArea to)? onFill;

  /// Opens the list of values of the active cell, under its rectangle on
  /// screen: an arrow beside the cell shows it is there.
  final ValueChanged<Rect>? onList;

  /// What the active cell asks to be typed, shown under it.
  final Widget? prompt;

  /// The filter of the sheet, whose arrows open [onFilter] with their
  /// column and their rectangle on screen.
  final SheetFilter? filter;
  final void Function(int col, Rect at)? onFilter;

  @override
  State<SheetView> createState() => SheetViewState();
}

class SheetViewState extends State<SheetView> {
  var _scroll = Offset.zero;
  Size _size = Size.zero;
  _Drag? _drag;
  final _texts = _TextCache();

  /// The fingers on the sheet, and the gap between two of them and the
  /// zoom when they came, for a pinch.
  final _touches = <int, Offset>{};
  (double, double)? _pinch;

  SheetLayout get _layout => widget.book.layout(widget.sheet);

  double get _z => widget.zoom;

  double get _rowHeader => math.max(34, 8 + 8 * '${_layout.rowAt(_scroll.dy) + 40}'.length.toDouble()) * math.min(_z, 1.4);

  double get _colHeader => _headerHeight * math.min(_z, 1.4);

  @override
  void initState() {
    super.initState();
    widget.selection.addListener(_followActive);
  }

  @override
  void didUpdateWidget(SheetView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.selection != widget.selection) {
      oldWidget.selection.removeListener(_followActive);
      widget.selection.addListener(_followActive);
    }
    if (oldWidget.sheet.id != widget.sheet.id) _scroll = Offset.zero;
  }

  @override
  void dispose() {
    widget.selection.removeListener(_followActive);
    super.dispose();
  }

  /// The origin of the part of the sheet that scrolls, past the frozen
  /// panes, in sheet pixels.
  Offset get _frozen => Offset(_layout.x(_layout.frozenCols + 1), _layout.y(_layout.frozenRows + 1));

  /// Where a column starts on screen.
  double screenX(int col) {
    final l = _layout;
    final x = l.x(col);
    return _rowHeader + (col <= l.frozenCols ? x : x - _scroll.dx) * _z;
  }

  double screenY(int row) {
    final l = _layout;
    final y = l.y(row);
    return _colHeader + (row <= l.frozenRows ? y : y - _scroll.dy) * _z;
  }

  /// Where a rectangle of the sheet, in pixels at 100 %, lies on screen:
  /// past the frozen panes it scrolls.
  Rect screenRect(Rect r) {
    final f = _frozen;
    final dx = r.left < f.dx ? 0.0 : _scroll.dx, dy = r.top < f.dy ? 0.0 : _scroll.dy;
    return Rect.fromLTWH(_rowHeader + (r.left - dx) * _z, _colHeader + (r.top - dy) * _z, r.width * _z, r.height * _z);
  }

  /// The cell under a point of the view.
  (int, int) cellAt(Offset p) {
    final l = _layout;
    final f = _frozen;
    final x = (p.dx - _rowHeader) / _z;
    final y = (p.dy - _colHeader) / _z;
    final col = x < f.dx ? l.colAt(x) : l.colAt(x + _scroll.dx);
    final row = y < f.dy ? l.rowAt(y) : l.rowAt(y + _scroll.dy);
    return (row, col);
  }

  /// The rectangle of an area on screen, merged cells whole.
  Rect areaRect(CellArea a) {
    var area = a;
    if (a.single) area = _layout.mergeAt(a.top, a.left) ?? a;
    final l = _layout;
    return Rect.fromLTRB(
      screenX(area.left),
      screenY(area.top),
      screenX(area.right) + l.width(area.right) * _z,
      screenY(area.bottom) + l.height(area.bottom) * _z,
    );
  }

  /// The rectangle of the active cell, where its editor goes.
  Rect get activeRect => areaRect(CellArea.cell(widget.selection.active.$1, widget.selection.active.$2));

  /// The rectangle of the active cell on the screen.
  Rect get activeScreenRect => _onScreen(activeRect);

  Rect _onScreen(Rect r) {
    final box = context.findRenderObject() as RenderBox?;
    return box == null ? r : box.localToGlobal(r.topLeft) & r.size;
  }

  /// The arrows of the filter on the columns in view, where they go.
  List<(int, Rect)> get _filterArrows {
    final f = widget.filter;
    if (f == null || _size.isEmpty) return const [];
    final l = _layout;
    final first = cellAt(Offset(_rowHeader + _frozen.dx * _z + 1, _colHeader + 1)).$2;
    final last = cellAt(Offset(_size.width - 1, _colHeader + 1)).$2;
    final out = <(int, Rect)>[];
    for (var col = f.area.left; col <= f.area.right; col++) {
      if (col > l.frozenCols && (col < first || col > last)) continue;
      final r = areaRect(CellArea.cell(f.area.top, col));
      final side = math.min(r.height - 2, 17 * _z);
      if (side > 6) out.add((col, Rect.fromLTWH(r.right - side - 1, r.bottom - side - 1, side, side)));
    }
    return out;
  }

  /// Where the arrow of the active cell's list goes, beside the cell.
  Rect? get _arrowRect {
    if (widget.onList == null || widget.editing) return null;
    final r = activeRect;
    final side = math.min(r.height, 20 * _z);
    return Rect.fromLTWH(r.right + 1, r.bottom - side, side, side);
  }

  /// Scrolls the active cell into view.
  void _followActive() {
    if (!mounted || _size.isEmpty) return;
    final l = _layout;
    final (row, col) = widget.selection.active;
    var s = _scroll;
    final viewW = (_size.width - _rowHeader) / _z - (_frozen.dx);
    final viewH = (_size.height - _colHeader) / _z - (_frozen.dy);
    if (col > l.frozenCols) {
      final left = l.x(col) - _frozen.dx, right = left + l.width(col);
      if (left < s.dx) s = Offset(left, s.dy);
      if (right > s.dx + viewW) s = Offset(math.max(0, right - viewW), s.dy);
    }
    if (row > l.frozenRows) {
      final top = l.y(row) - _frozen.dy, bottom = top + l.height(row);
      if (top < s.dy) s = Offset(s.dx, top);
      if (bottom > s.dy + viewH) s = Offset(s.dx, math.max(0, bottom - viewH));
    }
    if (s != _scroll) setState(() => _scroll = s);
  }

  void scrollBy(Offset d) {
    final l = _layout;
    final maxX = math.max(0.0, l.totalWidth - _frozen.dx);
    final maxY = math.max(0.0, l.totalHeight - _frozen.dy);
    setState(() => _scroll = Offset((_scroll.dx + d.dx).clamp(0, maxX), (_scroll.dy + d.dy).clamp(0, maxY)));
  }

  /// How many rows a page scrolls by.
  int get pageRows => math.max(1, ((_size.height - _colHeader) / _z / _layout.defaultHeight).floor() - 1);

  // pointer

  void _signal(PointerSignalEvent e) {
    if (e is! PointerScrollEvent) return;
    final keys = HardwareKeyboard.instance;
    if (keys.isControlPressed || keys.isMetaPressed) {
      widget.onZoom?.call((_z * (e.scrollDelta.dy < 0 ? 1.1 : 1 / 1.1)).clamp(0.1, 4));
      return;
    }
    var d = e.scrollDelta / _z;
    if (keys.isShiftPressed) d = Offset(d.dy, d.dx);
    scrollBy(d);
  }

  _Target _target(Offset p) {
    final l = _layout;
    if (p.dy < _colHeader && p.dx < _rowHeader) return const _Target.all();
    if (p.dy < _colHeader) {
      final (_, col) = cellAt(Offset(p.dx, _colHeader + 1));
      final right = screenX(col) + l.width(col) * _z;
      final left = screenX(col);
      if ((p.dx - right).abs() < 4) return _Target.colEdge(col);
      if ((p.dx - left).abs() < 4 && col > 1) return _Target.colEdge(col - 1);
      return _Target.col(col);
    }
    if (p.dx < _rowHeader) {
      final (row, _) = cellAt(Offset(_rowHeader + 1, p.dy));
      final bottom = screenY(row) + l.height(row) * _z;
      final top = screenY(row);
      if ((p.dy - bottom).abs() < 3) return _Target.rowEdge(row);
      if ((p.dy - top).abs() < 3 && row > 1) return _Target.rowEdge(row - 1);
      return _Target.row(row);
    }
    final sel = areaRect(widget.selection.area);
    if ((p - sel.bottomRight).distance < 6 && widget.onFill != null) return const _Target.fill();
    final (row, col) = cellAt(p);
    return _Target.cell(row, col);
  }

  void _down(PointerDownEvent e) {
    if (_arrowRect?.contains(e.localPosition) ?? false) return;
    if (_filterArrows.any((a) => a.$2.contains(e.localPosition))) return;
    if (e.buttons == kSecondaryMouseButton) {
      final t = _target(e.localPosition);
      if (t.kind == _Kind.cell && !widget.selection.area.contains(t.row, t.col)) {
        widget.selection.select(t.row, t.col);
      }
      widget.onMenu?.call(e.position, rows: t.kind == _Kind.row, cols: t.kind == _Kind.col);
      return;
    }
    final touch = e.kind == PointerDeviceKind.touch;
    if (touch) {
      _touches[e.pointer] = e.localPosition;
      if (_touches.length == 2) {
        final [a, b] = _touches.values.toList();
        _pinch = ((a - b).distance, _z);
        _drag = null;
        return;
      }
    }
    final t = _target(e.localPosition);
    final shift = HardwareKeyboard.instance.isShiftPressed;
    final s = widget.selection;
    switch (t.kind) {
      case _Kind.all:
        s.selectArea(const CellArea(1, 1, maxRows, maxCols));
      case _Kind.col:
        if (shift) {
          s.selectArea(CellArea(1, s.area.left, maxRows, t.col), active: s.active);
        } else {
          s.selectArea(CellArea(1, t.col, maxRows, t.col));
        }
        _drag = _Drag.cols(t.col);
      case _Kind.row:
        if (shift) {
          s.selectArea(CellArea(s.area.top, 1, t.row, maxCols), active: s.active);
        } else {
          s.selectArea(CellArea(t.row, 1, t.row, maxCols));
        }
        _drag = _Drag.rows(t.row);
      case _Kind.colEdge:
        _drag = _Drag.resize('c', t.col, _layout.width(t.col), e.localPosition);
      case _Kind.rowEdge:
        _drag = _Drag.resize('r', t.row, _layout.height(t.row), e.localPosition);
      case _Kind.fill:
        _drag = _Drag.fill(s.area);
      case _Kind.cell:
        if (touch) {
          _drag = _Drag.scroll(e.localPosition, t.row, t.col);
          return;
        }
        if (shift) {
          s.select(t.row, t.col, extend: true);
        } else if (!s.area.contains(t.row, t.col) || !s.area.single || s.active != (t.row, t.col)) {
          final merged = _layout.mergeAt(t.row, t.col);
          if (merged != null) {
            s.selectArea(merged);
          } else {
            s.select(t.row, t.col);
          }
        }
        _drag = _Drag.cells();
    }
  }

  void _move(PointerMoveEvent e) {
    if (_touches.containsKey(e.pointer)) _touches[e.pointer] = e.localPosition;
    final pinch = _pinch;
    if (pinch != null && _touches.length >= 2) {
      final [a, b, ...] = _touches.values.toList();
      if (pinch.$1 > 0) widget.onZoom?.call((pinch.$2 * (a - b).distance / pinch.$1).clamp(0.1, 4));
      return;
    }
    final d = _drag;
    if (d == null) return;
    final s = widget.selection;
    switch (d.kind) {
      case _DragKind.scroll:
        d.moved = d.moved || (e.localPosition - d.start).distance > 6;
        scrollBy(-e.delta / _z);
      case _DragKind.cells:
        final (row, col) = cellAt(e.localPosition);
        s.select(row, col, extend: true);
      case _DragKind.cols:
        final (_, col) = cellAt(e.localPosition);
        s.selectArea(CellArea(1, d.index, maxRows, col), active: (1, d.index));
      case _DragKind.rows:
        final (row, _) = cellAt(e.localPosition);
        s.selectArea(CellArea(d.index, 1, row, maxCols), active: (d.index, 1));
      case _DragKind.resize:
        final delta = (d.dim == 'c' ? e.localPosition.dx - d.start.dx : e.localPosition.dy - d.start.dy) / _z;
        setState(() => d.size = math.max(0, d.original + delta));
      case _DragKind.fill:
        final (row, col) = cellAt(e.localPosition);
        final from = d.area!;
        // down or right, as the pointer went furthest
        final down = row - from.bottom, right = col - from.right;
        final up = from.top - row, left = from.left - col;
        final best = [down, right, up, left].reduce(math.max);
        var to = from;
        if (best > 0) {
          if (best == down) to = CellArea(from.top, from.left, row, from.right);
          if (best == right) to = CellArea(from.top, from.left, from.bottom, col);
          if (best == up) to = CellArea(row, from.left, from.bottom, from.right);
          if (best == left) to = CellArea(from.top, col, from.bottom, from.right);
        }
        setState(() => d.target = to);
    }
  }

  void _lift(PointerEvent e) {
    _touches.remove(e.pointer);
    if (_touches.length < 2) _pinch = null;
  }

  void _up(PointerUpEvent e) {
    _lift(e);
    final d = _drag;
    _drag = null;
    if (d == null) return;
    switch (d.kind) {
      case _DragKind.scroll:
        if (!d.moved) {
          final merged = _layout.mergeAt(d.row, d.col);
          if (merged != null) {
            widget.selection.selectArea(merged);
          } else {
            widget.selection.select(d.row, d.col);
          }
        }
      case _DragKind.resize:
        if (d.size != d.original) widget.onResize?.call(d.dim, d.index, d.size);
        setState(() {});
      case _DragKind.fill:
        final to = d.target;
        if (to != null && to != d.area) widget.onFill?.call(d.area!, to);
        setState(() {});
      default:
    }
  }

  MouseCursor _cursor = SystemMouseCursors.basic;

  void _hover(PointerHoverEvent e) {
    final t = _target(e.localPosition);
    final c = switch (t.kind) {
      _Kind.colEdge => SystemMouseCursors.resizeColumn,
      _Kind.rowEdge => SystemMouseCursors.resizeRow,
      _Kind.fill => SystemMouseCursors.precise,
      _Kind.cell => SystemMouseCursors.cell,
      _ => SystemMouseCursors.basic,
    };
    if (c != _cursor) setState(() => _cursor = c);
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, box) {
      _size = box.biggest;
      final d = _drag;
      return MouseRegion(
        cursor: _cursor,
        onHover: _hover,
        child: Listener(
          onPointerSignal: _signal,
          onPointerDown: _down,
          onPointerMove: _move,
          onPointerUp: _up,
          onPointerCancel: _lift,
          child: GestureDetector(
            onDoubleTap: widget.onEdit,
            onLongPressStart: (e) {
              final (row, col) = cellAt(e.localPosition);
              widget.selection.select(row, col);
              widget.onMenu?.call(e.globalPosition);
            },
            child: Stack(children: [
              Positioned.fill(
                child: CustomPaint(
                  painter: _SheetPainter(
                    view: this,
                    book: widget.book,
                    sheet: widget.sheet,
                    layout: _layout,
                    selection: widget.selection.area,
                    active: widget.selection.active,
                    peers: widget.peers,
                    locale: widget.locale,
                    fonts: widget.fonts,
                    texts: _texts,
                    fillTarget: d?.kind == _DragKind.fill ? d!.target : null,
                    resizing: d?.kind == _DragKind.resize ? (d!.dim, d.index, d.size) : null,
                    editing: widget.editing,
                    scheme: Theme.of(context).colorScheme,
                  ),
                ),
              ),
              if (widget.editing && widget.editor != null)
                Positioned.fromRect(rect: activeRect.inflate(1), child: widget.editor!),
              if (_arrowRect case final arrow?)
                Positioned.fromRect(rect: arrow, child: _Arrow(icon: Icons.arrow_drop_down, onDown: () => widget.onList!(activeScreenRect))),
              for (final (col, r) in _filterArrows)
                Positioned.fromRect(
                  rect: r,
                  child: _Arrow(
                    icon: widget.filter!.filters(col) ? Icons.filter_alt : Icons.arrow_drop_down,
                    onDown: widget.onFilter == null ? null : () => widget.onFilter!(col, _onScreen(r)),
                  ),
                ),
              if (widget.prompt != null)
                Positioned(left: activeRect.left + 12, top: activeRect.bottom + 6, child: widget.prompt!),
            ]),
          ),
        ),
      );
    });
  }
}

/// The button of a list or a filter, opened on the press as Excel opens
/// them, past the double tap of the grid.
class _Arrow extends StatelessWidget {
  const _Arrow({required this.icon, this.onDown});

  final IconData icon;
  final VoidCallback? onDown;

  @override
  Widget build(BuildContext context) => Material(
    color: const Color(0xFFF3F3F3),
    shape: const Border.fromBorderSide(BorderSide(color: Color(0xFFABABAB))),
    child: MouseRegion(
      cursor: SystemMouseCursors.basic,
      child: Listener(
        onPointerDown: onDown == null ? null : (_) => onDown!(),
        child: FittedBox(child: Icon(icon, color: Colors.grey.shade800)),
      ),
    ),
  );
}

enum _Kind { cell, col, row, colEdge, rowEdge, all, fill }

class _Target {
  const _Target.cell(this.row, this.col) : kind = _Kind.cell;
  const _Target.col(this.col) : kind = _Kind.col, row = 0;
  const _Target.row(this.row) : kind = _Kind.row, col = 0;
  const _Target.colEdge(this.col) : kind = _Kind.colEdge, row = 0;
  const _Target.rowEdge(this.row) : kind = _Kind.rowEdge, col = 0;
  const _Target.all() : kind = _Kind.all, row = 0, col = 0;
  const _Target.fill() : kind = _Kind.fill, row = 0, col = 0;

  final _Kind kind;
  final int row, col;
}

enum _DragKind { scroll, cells, cols, rows, resize, fill }

class _Drag {
  _Drag.scroll(this.start, this.row, this.col) : kind = _DragKind.scroll, dim = '', index = 0, original = 0;
  _Drag.cells() : kind = _DragKind.cells, start = Offset.zero, row = 0, col = 0, dim = '', index = 0, original = 0;
  _Drag.cols(this.index) : kind = _DragKind.cols, start = Offset.zero, row = 0, col = 0, dim = '', original = 0;
  _Drag.rows(this.index) : kind = _DragKind.rows, start = Offset.zero, row = 0, col = 0, dim = '', original = 0;
  _Drag.resize(this.dim, this.index, this.original, this.start) : kind = _DragKind.resize, row = 0, col = 0 {
    size = original;
  }
  _Drag.fill(this.area) : kind = _DragKind.fill, start = Offset.zero, row = 0, col = 0, dim = '', index = 0, original = 0;

  final _DragKind kind;
  final Offset start;
  final int row, col;
  final String dim;
  final int index;
  final double original;
  double size = 0;
  var moved = false;
  CellArea? area;
  CellArea? target;
}

/// Text laid out once for as long as it is drawn the same.
class _TextCache {
  final _cache = <(String, TextStyle, double, bool), TextPainter>{};

  TextPainter get(String text, TextStyle style, double width, bool wrap) {
    final key = (text, style, wrap ? width : 0.0, wrap);
    final hit = _cache[key];
    if (hit != null) return hit;
    if (_cache.length > 4000) _cache.clear();
    final p = TextPainter(text: TextSpan(text: text, style: style), textDirection: TextDirection.ltr, maxLines: wrap ? null : 1)
      ..layout(maxWidth: wrap ? math.max(width, 1) : double.infinity);
    return _cache[key] = p;
  }
}

class _SheetPainter extends CustomPainter {
  _SheetPainter({
    required this.view,
    required this.book,
    required this.sheet,
    required this.layout,
    required this.selection,
    required this.active,
    required this.peers,
    required this.locale,
    required this.fonts,
    required this.texts,
    required this.fillTarget,
    required this.resizing,
    required this.editing,
    required this.scheme,
  });

  final SheetViewState view;
  final Workbook book;
  final Node sheet;
  final SheetLayout layout;
  final CellArea selection;
  final (int, int) active;
  final List<PeerSelection> peers;
  final NumberLocale locale;
  final String? fonts;
  final _TextCache texts;
  final CellArea? fillTarget;
  final (String, int, double)? resizing;
  final bool editing;
  final ColorScheme scheme;

  double get z => view._z;

  late final looks = book.looks(sheet);

  /// The format of a cell, with what its conditional formats change.
  CellStyle _style(Cell c) {
    final id = c.fields['s'] as String?;
    return looks.style(book.style(id), id, c.row, c.col);
  }

  double width(int col) => resizing != null && resizing!.$1 == 'c' && resizing!.$2 == col ? resizing!.$3 : layout.width(col);

  double height(int row) => resizing != null && resizing!.$1 == 'r' && resizing!.$2 == row ? resizing!.$3 : layout.height(row);

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(Offset.zero & size, Paint()..color = Colors.white);
    final l = layout;
    final top = view._colHeader, left = view._rowHeader;
    final fx = left + l.x(l.frozenCols + 1) * z, fy = top + l.y(l.frozenRows + 1) * z;
    final (lastRow, lastCol) = view.cellAt(Offset(size.width - 1, size.height - 1));
    final firstRow = view.cellAt(Offset(left + 1, math.max(fy, top) + 1)).$1;
    final firstCol = view.cellAt(Offset(math.max(fx, left) + 1, top + 1)).$2;
    final panes = <(Rect, CellArea)>[
      if (l.frozenRows > 0 && l.frozenCols > 0) (Rect.fromLTRB(left, top, fx, fy), CellArea(1, 1, l.frozenRows, l.frozenCols)),
      if (l.frozenRows > 0) (Rect.fromLTRB(fx, top, size.width, fy), CellArea(1, firstCol, l.frozenRows, lastCol)),
      if (l.frozenCols > 0) (Rect.fromLTRB(left, fy, fx, size.height), CellArea(firstRow, 1, lastRow, l.frozenCols)),
      (Rect.fromLTRB(fx, fy, size.width, size.height), CellArea(firstRow, firstCol, lastRow, lastCol)),
    ];
    for (final (rect, area) in panes) {
      if (rect.isEmpty) continue;
      canvas.save();
      canvas.clipRect(rect);
      _cells(canvas, area);
      _selection(canvas);
      for (final c in SheetChart.of(sheet)) {
        paintSheetChart(canvas, view.screenRect(c.rect(l)), c.spec, book, sheet, zoom: z, fonts: fonts);
      }
      canvas.restore();
    }
    _headers(canvas, size, panes);
    if (l.frozenCols > 0) canvas.drawLine(Offset(fx, top), Offset(fx, size.height), Paint()..color = const Color(0xFF9E9E9E));
    if (l.frozenRows > 0) canvas.drawLine(Offset(left, fy), Offset(size.width, fy), Paint()..color = const Color(0xFF9E9E9E));
  }

  Rect _rect(int r1, int c1, int r2, int c2) => Rect.fromLTRB(
    view.screenX(c1),
    view.screenY(r1),
    view.screenX(c2) + width(c2) * z,
    view.screenY(r2) + height(r2) * z,
  );

  void _cells(Canvas canvas, CellArea area) {
    final grid = sheet.grid ?? const Grid.empty();
    final l = layout;
    // cells whose merged area starts before the pane still show
    final merges = [for (final m in l.merges) if (m.intersects(area)) m];
    final covered = <(int, int)>{};
    for (final m in merges) {
      for (var r = m.top; r <= m.bottom; r++) {
        for (var c = m.left; c <= m.right; c++) {
          if (r != m.top || c != m.left) covered.add((r, c));
        }
      }
    }
    // the style of whole rows and columns
    final rowStyles = <int, String>{};
    final colStyles = <int, String>{};
    for (final c in grid.rows(0, 0)) {
      if (c.fields['s'] is String) colStyles[c.col] = c.fields['s']! as String;
    }

    // fills
    final cells = <Cell>[];
    for (final c in grid.rows(area.top, area.bottom)) {
      if (c.col == 0) {
        if (c.fields['s'] is String) rowStyles[c.row] = c.fields['s']! as String;
        continue;
      }
      if (c.col < area.left || c.col > area.right) continue;
      cells.add(c);
    }
    for (final m in merges) {
      if (!area.contains(m.top, m.left)) {
        final f = grid.cell(m.top, m.left);
        if (f != null) cells.add(Cell(m.top, m.left, f));
      }
    }
    // empty cells a conditional format fills
    if (!looks.isEmpty) {
      final held = {for (final c in cells) (c.row, c.col)};
      for (final c in looks.grid.rows(area.top, area.bottom)) {
        if (c.col >= area.left && c.col <= area.right && !held.contains((c.row, c.col))) cells.add(Cell(c.row, c.col, const {}));
      }
    }
    for (final e in colStyles.entries) {
      if (e.key < area.left || e.key > area.right) continue;
      final fill = book.style(e.value).fill;
      if (fill != null) canvas.drawRect(_rect(area.top, e.key, area.bottom, e.key), Paint()..color = fill);
    }
    for (final e in rowStyles.entries) {
      final fill = book.style(e.value).fill;
      if (fill != null) canvas.drawRect(_rect(e.key, area.left, e.key, area.right), Paint()..color = fill);
    }
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final fill = _style(c).fill;
      if (fill == null) continue;
      final m = l.mergeAt(c.row, c.col);
      canvas.drawRect(m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right), Paint()..color = fill);
    }

    // gridlines, which merged cells and fills hide
    if (l.gridlines) {
      final p = Paint()
        ..color = _gridColor
        ..strokeWidth = 1;
      final y1 = view.screenY(area.top), y2 = view.screenY(area.bottom) + height(area.bottom) * z;
      final x1 = view.screenX(area.left), x2 = view.screenX(area.right) + width(area.right) * z;
      for (var c = area.left; c <= area.right + 1; c++) {
        final x = view.screenX(c) - 0.5;
        canvas.drawLine(Offset(x, y1), Offset(x, y2), p);
      }
      for (var r = area.top; r <= area.bottom + 1; r++) {
        final y = view.screenY(r) - 0.5;
        canvas.drawLine(Offset(x1, y), Offset(x2, y), p);
      }
      final white = Paint()..color = Colors.white;
      for (final m in merges) {
        final f = grid.cell(m.top, m.left);
        if (book.style(f?['s'] as String?).fill == null) canvas.drawRect(_rect(m.top, m.left, m.bottom, m.right).deflate(0.5), white);
      }
    }

    // data bars and icons
    final icons = <(int, int)>{};
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final bar = looks.bar(c.row, c.col);
      final icon = looks.icon(c.row, c.col);
      if (bar == null && icon == null) continue;
      final r = _rect(c.row, c.col, c.row, c.col);
      if (r.height < 6 || r.width < 6) continue;
      if (bar != null) {
        final b = Rect.fromLTWH(r.left + 1, r.top + 2, (r.width - 2) * bar.$1 / 100, r.height - 4);
        canvas.drawRect(b, Paint()..shader = LinearGradient(colors: [bar.$2, Color.lerp(bar.$2, Colors.white, 0.85)!]).createShader(b));
      }
      if (icon != null) {
        final side = math.min(r.height - 4, 14 * z);
        paintIcon(canvas, Rect.fromLTWH(r.left + 2 * z, r.bottom - side - 2, side, side), icon.$1, icon.$2);
        icons.add((c.row, c.col));
      }
    }

    // texts
    final occupied = <(int, int)>{for (final c in cells) if (_shows(c.fields)) (c.row, c.col)};
    for (final c in cells) {
      if (covered.contains((c.row, c.col)) || !_shows(c.fields) || looks.hidesValue(c.row, c.col)) continue;
      _text(canvas, c, occupied, area, icon: icons.contains((c.row, c.col)));
    }

    // borders
    for (final c in cells) {
      if (covered.contains((c.row, c.col))) continue;
      final s = _style(c);
      if (s.left == null && s.right == null && s.top == null && s.bottom == null) continue;
      final m = l.mergeAt(c.row, c.col);
      final r = m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right);
      _edge(canvas, s.top, r.topLeft, r.topRight);
      _edge(canvas, s.bottom, r.bottomLeft, r.bottomRight);
      _edge(canvas, s.left, r.topLeft, r.bottomLeft);
      _edge(canvas, s.right, r.topRight, r.bottomRight);
    }
  }

  static bool _shows(Map<String, Object?> f) => f['v'] != null || f['e'] != null;

  void _edge(Canvas canvas, Edge? e, Offset a, Offset b) {
    if (e == null) return;
    final p = Paint()
      ..color = e.color
      ..strokeWidth = e.width * math.min(z, 1.5);
    if (e.style == 'double') {
      final n = a.dx == b.dx ? const Offset(1.5, 0) : const Offset(0, 1.5);
      p.strokeWidth = 1;
      canvas.drawLine(a - n, b - n, p);
      canvas.drawLine(a + n, b + n, p);
      return;
    }
    if (e.style.contains('ash') || e.style.contains('ot') || e.style == 'hair') {
      final dash = e.style == 'dotted' || e.style == 'hair' ? 1.5 : 4.0;
      final len = (b - a).distance;
      final dir = (b - a) / len;
      for (var t = 0.0; t < len; t += dash * 2) {
        canvas.drawLine(a + dir * t, a + dir * math.min(t + dash, len), p);
      }
      return;
    }
    canvas.drawLine(a, b, p);
  }

  /// Paints the text of a cell, past the icon a conditional format gives
  /// it if any.
  void _text(Canvas canvas, Cell c, Set<(int, int)> occupied, CellArea area, {bool icon = false}) {
    final l = layout;
    final style = _style(c);
    final shown = cellText(c.fields, style, locale, date1904: book.date1904);
    if (shown.text.isEmpty) return;
    final m = l.mergeAt(c.row, c.col);
    final rect = m == null ? _rect(c.row, c.col, c.row, c.col) : _rect(m.top, m.left, m.bottom, m.right);
    final f = style.font;
    final ts = TextStyle(
      fontFamily: substituteFont(f.name),
      package: fonts,
      fontSize: f.size * 4 / 3 * z,
      fontWeight: f.bold ? FontWeight.bold : FontWeight.normal,
      fontStyle: f.italic ? FontStyle.italic : FontStyle.normal,
      decoration: TextDecoration.combine([
        if (f.underline) TextDecoration.underline,
        if (f.strike) TextDecoration.lineThrough,
      ]),
      color: shown.color ?? f.color ?? Colors.black,
      height: 1.2,
    );
    final pad = 2 * z + style.indent * 9 * z;
    var text = shown.text;
    var painter = texts.get(text, ts, rect.width - 2 * pad, style.wrap);
    // numbers that do not fit show ###
    if (shown.number && !style.wrap && painter.width > rect.width - 2 * pad && m == null) {
      final hash = texts.get('#', ts, 0, false).width;
      text = '#' * math.max(1, ((rect.width - 2 * pad) / math.max(hash, 1)).floor());
      painter = texts.get(text, ts, 0, false);
    }
    // text runs over the empty cells next to it
    var clip = rect;
    if (!style.wrap && m == null && !shown.number && painter.width > rect.width - 2 * pad) {
      if (shown.align == 'left' || shown.align == 'center') {
        var col = c.col + 1;
        while (col <= area.right + 1 && !occupied.contains((c.row, col)) && clip.right < rect.left + painter.width + 2 * pad) {
          clip = Rect.fromLTRB(clip.left, clip.top, view.screenX(col) + width(col) * z, clip.bottom);
          col++;
        }
      }
      if (shown.align == 'right' || shown.align == 'center') {
        var col = c.col - 1;
        while (col >= 1 && !occupied.contains((c.row, col)) && clip.left > rect.right - painter.width - 2 * pad) {
          clip = Rect.fromLTRB(view.screenX(col), clip.top, clip.right, clip.bottom);
          col--;
        }
      }
    }
    final dx = switch (shown.align) {
      'right' => rect.right - pad - painter.width,
      'center' => rect.center.dx - painter.width / 2,
      _ => rect.left + pad + (icon ? 16 * z : 0),
    };
    final dy = switch (style.vertical) {
      'top' => rect.top + 1,
      'center' || 'justify' || 'distributed' => rect.center.dy - painter.height / 2,
      _ => rect.bottom - painter.height - 1,
    };
    canvas.save();
    canvas.clipRect(clip.deflate(0.5));
    painter.paint(canvas, Offset(dx, dy));
    canvas.restore();
  }

  void _selection(Canvas canvas) {
    final sel = view.areaRect(selection);
    final one = selection.single || layout.mergeAt(selection.top, selection.left) == selection;
    if (!one) {
      final path = Path()..addRect(sel);
      if (!editing) path.addRect(view.activeRect);
      path.fillType = PathFillType.evenOdd;
      canvas.drawPath(path, Paint()..color = _selectColor.withValues(alpha: 0.12));
    }
    for (final p in peers) {
      final r = view.areaRect(p.area);
      canvas.drawRect(r.deflate(1), Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2
        ..color = p.color);
      final tag = texts.get(p.name, TextStyle(fontFamily: substituteFont(book.minorFont), package: fonts, fontSize: 10, color: Colors.white), 0, false);
      final box = Rect.fromLTWH(r.right - tag.width - 6, r.top - tag.height - 2, tag.width + 6, tag.height + 2);
      canvas.drawRect(box, Paint()..color = p.color);
      tag.paint(canvas, box.topLeft + const Offset(3, 1));
    }
    final border = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..color = _selectColor;
    canvas.drawRect(sel.deflate(1), border);
    final fill = fillTarget;
    if (fill != null) {
      canvas.drawRect(view.areaRect(fill).deflate(1), Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1
        ..color = const Color(0xFF7F7F7F));
    }
    if (!editing) {
      canvas.drawRect(Rect.fromCenter(center: sel.bottomRight - const Offset(1, 1), width: 6, height: 6), Paint()..color = _selectColor);
    }
  }

  void _headers(Canvas canvas, Size size, List<(Rect, CellArea)> panes) {
    final top = view._colHeader, left = view._rowHeader;
    final bg = Paint()..color = _headerColor;
    canvas.drawRect(Rect.fromLTWH(0, 0, size.width, top), bg);
    canvas.drawRect(Rect.fromLTWH(0, 0, left, size.height), bg);
    final line = Paint()..color = const Color(0xFFBFBFBF);
    final ts = TextStyle(fontFamily: substituteFont(book.minorFont), package: fonts, fontSize: 11 * math.min(z, 1.4), color: const Color(0xFF444444));
    final hot = ts.copyWith(color: _selectColor, fontWeight: FontWeight.bold);
    final done = <String>{};
    for (final (rect, area) in panes) {
      canvas.save();
      canvas.clipRect(Rect.fromLTRB(rect.left, 0, rect.right, top));
      for (var c = area.left; c <= area.right; c++) {
        if (!done.add('c$c')) continue;
        final x = view.screenX(c), w = width(c) * z;
        if (w <= 0) continue;
        final selected = c >= selection.left && c <= selection.right;
        if (selected) canvas.drawRect(Rect.fromLTWH(x, 0, w, top), Paint()..color = const Color(0xFFD2D2D2));
        final t = texts.get(columnName(c), selected ? hot : ts, 0, false);
        t.paint(canvas, Offset(x + (w - t.width) / 2, (top - t.height) / 2));
        canvas.drawLine(Offset(x + w - 0.5, 0), Offset(x + w - 0.5, top), line);
      }
      canvas.restore();
      canvas.save();
      canvas.clipRect(Rect.fromLTRB(0, rect.top, left, rect.bottom));
      for (var r = area.top; r <= area.bottom; r++) {
        if (!done.add('r$r')) continue;
        final y = view.screenY(r), h = height(r) * z;
        if (h <= 0) continue;
        final selected = r >= selection.top && r <= selection.bottom;
        if (selected) canvas.drawRect(Rect.fromLTWH(0, y, left, h), Paint()..color = const Color(0xFFD2D2D2));
        final t = texts.get('$r', selected ? hot : ts, 0, false);
        t.paint(canvas, Offset((left - t.width) / 2, y + (h - t.height) / 2));
        canvas.drawLine(Offset(0, y + h - 0.5), Offset(left, y + h - 0.5), line);
      }
      canvas.restore();
    }
    canvas.drawLine(Offset(0, top - 0.5), Offset(size.width, top - 0.5), line);
    canvas.drawLine(Offset(left - 0.5, 0), Offset(left - 0.5, size.height), line);
  }

  @override
  bool shouldRepaint(_SheetPainter old) => true;
}
