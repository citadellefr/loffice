import 'dart:convert';

import 'package:trame/trame.dart';

import 'print.dart';
import 'sheet_comments.dart';
import 'workbook.dart';

/// A change made to the format of a cell, at [row] and [col] of the area
/// formatted: its style as the Go package writes it, changed.
typedef StyleChange = Map<String, Object?> Function(Map<String, Object?> style, int row, int col);

const _linkColor = {'rgb': '0563C1'};

const _cleared = {'v': null, 'f': null, 'e': null, 'rich': null};

/// The address a link goes to for what someone typed: the web for a name,
/// mail for an address, null for nothing.
String? linkAddress(String text) {
  text = text.trim();
  if (text.isEmpty) return null;
  if (RegExp('^[a-zA-Z][a-zA-Z0-9+.-]*:').hasMatch(text)) return text;
  return text.contains('@') && !text.contains('/') ? 'mailto:$text' : 'https://$text';
}

/// The edits a spreadsheet editor makes, as the Go package takes them.
class SheetEdits {
  SheetEdits(this.book);

  final Workbook book;

  Tree get tree => book.tree;

  /// The style of the cell format [id], as the file gives it.
  Map<String, Object?> styleOf(String? id) {
    final json = tree[id ?? 'x0']?.attributes['style'];
    return json is Map<String, Object?> ? json : const {};
  }

  /// The format a cell shows in: its own, or its row's, or its column's.
  String? styleAt(Node sheet, int row, int col) {
    final grid = sheet.grid ?? const Grid.empty();
    return grid.cell(row, col)?['s'] as String? ?? grid.cell(row, 0)?['s'] as String? ?? grid.cell(0, col)?['s'] as String?;
  }

  /// The id of the format [base] becomes with [changed]: one the workbook
  /// has already when it has the same, else a new one, whose creation is
  /// added to [created].
  String derive(String? base, Map<String, Object?> changed, List<Change> created) {
    final origin = base == null ? (tree['x0'] == null ? null : 'x0') : (styleOf(base)['base'] as String? ?? base);
    final style = {...changed}..remove('base');
    _prune(style);
    for (final n in tree.children('')) {
      if (n.type != 'xf') continue;
      final other = {...?(n.attributes['style'] as Map<String, Object?>?)}..remove('base');
      if (jsonEquals(other, style)) return n.id;
    }
    for (final c in created) {
      final other = {...?(c.attributes['style'] as Map<String, Object?>?)}..remove('base');
      if (jsonEquals(other, style)) return c.id;
    }
    final id = 'f${randomId()}';
    created.add(
      Change.create(
        Node(
          id: id,
          type: 'xf',
          key: 'V',
          attributes: {
            'style': {'base': ?origin, ...style},
          },
        ),
      ),
    );
    return id;
  }

  /// Removes the empty parts of a style, as the Go package leaves them out.
  static void _prune(Map<String, Object?> m) {
    for (final k in m.keys.toList()) {
      final v = m[k];
      if (v is Map<String, Object?>) {
        _prune(v);
        if (v.isEmpty) m.remove(k);
      } else if (v == null || v == false || v == '' || v == 0 && k != 'theme') {
        m.remove(k);
      }
    }
  }

  /// Formats the cells of an area of [sheet]; [key] tells which cells
  /// change alike, by default all of them. Whole rows and columns take the
  /// format too, for the cells typed there later.
  Edit format(Node sheet, CellArea area, StyleChange change, {Object? Function(int row, int col)? key}) {
    final created = <Change>[];
    final ids = <(String?, Object?), String>{};
    String idFor(String? current, int row, int col) {
      final k = (current, key?.call(row, col));
      return ids[k] ??= derive(current, change(_copy(styleOf(current)), row, col), created);
    }

    final grid = sheet.grid ?? const Grid.empty();
    final cells = <Cell>[];
    if (area.wholeCols || area.wholeRows) {
      if (area.wholeCols && area.wholeRows) {
        for (var c = area.left; c <= area.right && c <= _lastCol(sheet); c++) {
          cells.add(Cell(0, c, {'s': idFor(grid.cell(0, c)?['s'] as String?, 0, c)}));
        }
      } else if (area.wholeCols) {
        for (var c = area.left; c <= area.right; c++) {
          cells.add(Cell(0, c, {'s': idFor(grid.cell(0, c)?['s'] as String?, 0, c)}));
        }
      } else {
        for (var r = area.top; r <= area.bottom; r++) {
          cells.add(Cell(r, 0, {'s': idFor(grid.cell(r, 0)?['s'] as String?, r, 0)}));
        }
      }
      for (final c in grid.rows(area.top, area.bottom)) {
        if (c.row == 0 || c.col == 0 || !area.contains(c.row, c.col)) continue;
        cells.add(Cell(c.row, c.col, {'s': idFor(c.fields['s'] as String?, c.row, c.col)}));
      }
    } else {
      for (var r = area.top; r <= area.bottom; r++) {
        for (var c = area.left; c <= area.right; c++) {
          cells.add(Cell(r, c, {'s': idFor(styleAt(sheet, r, c), r, c)}));
        }
      }
    }
    return Edit([...created, Change.cells(sheet.id, cells)]);
  }

  int _lastCol(Node sheet) => book.layout(sheet).usedCols;

  static Map<String, Object?> _copy(Map<String, Object?> m) => jsonDecode(jsonEncode(m)) as Map<String, Object?>;

  /// Sets values typed into cells, and the format they call for when the
  /// cell shows numbers in General.
  Edit setCells(Node sheet, List<(int, int, Map<String, Object?>)> values, {String? format}) {
    final created = <Change>[];
    final cells = <Cell>[];
    for (final (r, c, fields) in values) {
      final f = {...fields};
      if (format != null) {
        final current = styleAt(sheet, r, c);
        final fmt = styleOf(current)['fmt'];
        if (fmt == null || fmt == 'General') {
          f['s'] = derive(current, {..._copy(styleOf(current)), 'fmt': format}, created);
        }
      }
      cells.add(Cell(r, c, f));
    }
    return Edit([...created, Change.cells(sheet.id, cells)]);
  }

  /// Clears what the cells of an area hold, their formats kept.
  Edit clear(Node sheet, CellArea area) {
    final grid = sheet.grid ?? const Grid.empty();
    final cells = [
      for (final c in grid.rows(area.top, area.bottom))
        if (c.row > 0 && c.col > 0 && area.contains(c.row, c.col) && (c.fields['v'] != null || c.fields['f'] != null || c.fields['e'] != null)) Cell(c.row, c.col, _cleared),
    ];
    return cells.isEmpty ? Edit() : Edit([Change.cells(sheet.id, cells)]);
  }

  /// Clears the formats of the cells of an area.
  Edit clearFormats(Node sheet, CellArea area) {
    final grid = sheet.grid ?? const Grid.empty();
    final cells = [
      for (final c in grid.rows(area.top, area.bottom))
        if (c.row > 0 && c.col > 0 && area.contains(c.row, c.col) && c.fields['s'] != null) Cell(c.row, c.col, const {'s': null}),
    ];
    return cells.isEmpty ? Edit() : Edit([Change.cells(sheet.id, cells)]);
  }

  /// Sets the width of columns or the height of rows, in pixels.
  Edit resize(Node sheet, String dim, List<int> indexes, double pixels) {
    if (dim == dimCols) {
      return Edit([
        Change.cells(sheet.id, [
          for (final c in indexes) Cell(0, c, pixels <= 0 ? {'hide': true} : {'w': double.parse((pixels / 7).toStringAsFixed(2)), 'cw': true, 'hide': null}),
        ]),
      ]);
    }
    return Edit([
      Change.cells(sheet.id, [
        for (final r in indexes) Cell(r, 0, pixels <= 0 ? {'hide': true} : {'h': double.parse((pixels * 3 / 4).toStringAsFixed(2)), 'ch': true, 'hide': null}),
      ]),
    ]);
  }

  /// Merges an area into its first cell, which keeps its value; the others
  /// are cleared, as Excel does. With [across] each row of the area is
  /// merged on its own.
  Edit merge(Node sheet, CellArea area, {bool center = true, bool across = false}) {
    final grid = sheet.grid ?? const Grid.empty();
    final firsts = [
      if (across)
        for (var r = area.top; r <= area.bottom; r++) CellArea(r, area.left, r, area.right)
      else
        area,
    ];
    final cells = <Cell>[
      for (final a in firsts)
        Cell(a.top, a.left, {
          'm': [a.rows, a.cols],
        }),
    ];
    for (final c in grid.rows(area.top, area.bottom)) {
      if (c.col == 0 || !area.contains(c.row, c.col) || c.col == area.left && (across || c.row == area.top)) continue;
      cells.add(Cell(c.row, c.col, {..._cleared, 'link': null, 'm': null}));
    }
    final edit = Edit([Change.cells(sheet.id, cells)]);
    if (!center) return edit;
    final aligned = format(
      sheet,
      across ? CellArea(area.top, area.left, area.bottom, area.left) : CellArea.cell(area.top, area.left),
      (s, _, _) => {
        ...s,
        'align': {...?(s['align'] as Map<String, Object?>?), 'h': 'center'},
      },
    );
    return edit.compose(aligned);
  }

  /// Splits the merged areas an area holds.
  Edit unmerge(Node sheet, CellArea area) {
    final cells = [
      for (final m in book.layout(sheet).merges)
        if (m.intersects(area)) Cell(m.top, m.left, const {'m': null}),
    ];
    return cells.isEmpty ? Edit() : Edit([Change.cells(sheet.id, cells)]);
  }

  // links

  /// Makes a cell lead to [link], a Link of the Go package, or no longer
  /// lead anywhere for null. A cell with no value shows where the link
  /// goes, in the format of links; one that had a link and loses it loses
  /// that format.
  Edit setLink(Node sheet, int row, int col, Map<String, Object?>? link) {
    final fields = sheet.grid?.cell(row, col);
    final had = fields?['link'] != null;
    final value = <String, Object?>{'link': link};
    if (link != null && fields?['v'] == null && fields?['f'] == null) value['v'] = link['url'] ?? link['loc'];
    final edit = Edit([
      Change.cells(sheet.id, [Cell(row, col, value)]),
    ]);
    final current = {...?(styleOf(styleAt(sheet, row, col))['font'] as Map<String, Object?>?)};
    final looks = jsonEquals(current['color'], _linkColor) && current['u'] == 'single';
    if (link != null ? had : !had || !looks) return edit;
    final looked = format(sheet, CellArea.cell(row, col), (s, _, _) {
      final font = {...?(s['font'] as Map<String, Object?>?)};
      if (link != null) {
        font
          ..['color'] = _linkColor
          ..['u'] = 'single';
      } else {
        font
          ..remove('color')
          ..remove('u');
      }
      return {...s, 'font': font};
    });
    return edit.compose(looked);
  }

  // printing

  /// Changes how a sheet prints: the settings of its "print" given, those
  /// set to null left out.
  Edit printSetup(Node sheet, Map<String, Object?> changes) {
    final setup = PrintSetup.of(sheet).change(changes);
    return Edit([
      Change.set(sheet.id, attributes: {'print': setup.isEmpty ? null : setup}),
    ]);
  }

  /// Sets what a sheet prints to an area, or to all it holds for null.
  Edit printArea(Node sheet, CellArea? area) {
    final all = tree['book']?.attributes['names'];
    final names = [
      for (final n in all is List<Object?> ? all : const <Object?>[])
        if (!(n is Map<String, Object?> && n['sheet'] == sheet.id && '${n['name']}'.toLowerCase() == printAreaName.toLowerCase())) n,
    ];
    if (area != null) {
      final name = "'${'${sheet.attributes['name']}'.replaceAll("'", "''")}'";
      final ref = '$name!\$${columnName(area.left)}\$${area.top}:\$${columnName(area.right)}\$${area.bottom}';
      names.add({'name': printAreaName, 'ref': ref, 'sheet': sheet.id});
    }
    return Edit([
      Change.set('book', attributes: {'names': names.isEmpty ? null : names}),
    ]);
  }

  /// Starts a page over the row and left of the column of a cell, or
  /// removes the breaks there when [remove].
  Edit pageBreak(Node sheet, int row, int col, {bool remove = false}) {
    final setup = PrintSetup.of(sheet);
    List<int> changed(List<int> breaks, int at) =>
        ({...breaks}
              ..remove(at)
              ..addAll([if (!remove && at > 1) at]))
            .toList()
          ..sort();
    return printSetup(sheet, {'rows': changed(setup.rowBreaks, row), 'cols': changed(setup.colBreaks, col)});
  }

  // conditional formats and validations

  /// The conditional formats of a sheet, the first the one that counts.
  List<Map<String, Object?>> conditionals(Node sheet) => _rules(sheet, 'cf');

  /// Sets the conditional formats of a sheet, numbering them as they come.
  Edit setConditionals(Node sheet, List<Map<String, Object?>> rules) {
    final numbered = [
      for (var i = 0; i < rules.length; i++) {...rules[i], 'pri': i + 1},
    ];
    return Edit([
      Change.set(sheet.id, attributes: {'cf': numbered.isEmpty ? null : numbered}),
    ]);
  }

  /// Adds a conditional format for the cells of an area, before the others.
  Edit addConditional(Node sheet, CellArea area, Map<String, Object?> rule) => setConditionals(sheet, [
    {...rule, 'ref': area.name},
    ...conditionals(sheet),
  ]);

  /// Takes the conditional formats off the cells of an area, or off the
  /// whole sheet for null: a rule that holds for other cells stays for them.
  Edit clearConditionals(Node sheet, CellArea? area) => setConditionals(sheet, area == null ? const [] : _cut(conditionals(sheet), area, _moveRule));

  /// Sets what the cells of an area take, in place of what they took, no
  /// rule for any value for null.
  Edit validate(Node sheet, CellArea area, Map<String, Object?>? rule) {
    final all = [
      ..._cut(_rules(sheet, 'valid'), area, _moveValidation),
      if (rule != null) {...rule, 'ref': area.name},
    ];
    return Edit([
      Change.set(sheet.id, attributes: {'valid': all.isEmpty ? null : all}),
    ]);
  }

  static List<Map<String, Object?>> _rules(Node sheet, String key) {
    final all = sheet.attributes[key];
    return [
      for (final r in all is List<Object?> ? all : const <Object?>[])
        if (r is Map<String, Object?>) r,
    ];
  }

  /// The rules without the cells of [hole]: a rule loses the part of its
  /// areas in the hole, and goes when none is left. Their formulas are
  /// written for the first cell of the areas, so [move] follows it when
  /// it changes.
  static List<Map<String, Object?>> _cut(List<Map<String, Object?>> rules, CellArea hole, Map<String, Object?> Function(Map<String, Object?> rule, int rows, int cols) move) {
    final out = <Map<String, Object?>>[];
    for (final rule in rules) {
      final before = '${rule['ref']}'.split(' ')..removeWhere((t) => t.isEmpty);
      final kept = <String>[];
      for (final token in before) {
        final area = parseArea(token);
        kept.addAll(area == null ? [token] : [for (final a in area.minus(hole)) a.name]);
      }
      if (kept.isEmpty) continue;
      final (was, now) = (parseArea(before.first), parseArea(kept.first));
      final moved = was == null || now == null ? rule : move(rule, now.top - was.top, now.left - was.left);
      out.add({...moved, 'ref': kept.join(' ')});
    }
    return out;
  }

  static Map<String, Object?> _moveRule(Map<String, Object?> rule, int rows, int cols) {
    final f = rule['f'];
    return f is! List<Object?>
        ? rule
        : {
            ...rule,
            'f': [for (final x in f) x is String ? moveFormula(x, rows, cols) : x],
          };
  }

  static Map<String, Object?> _moveValidation(Map<String, Object?> rule, int rows, int cols) => {
    ...rule,
    for (final k in ['f1', 'f2'])
      if (rule[k] is String) k: moveFormula(rule[k]! as String, rows, cols),
  };

  // sheets

  /// A sheet named [name], after [after] or last.
  Change newSheet(String name, {Node? after}) {
    final sheets = book.sheets;
    final i = after == null ? sheets.length - 1 : sheets.indexWhere((s) => s.id == after.id);
    final before = i >= 0 ? sheets[i].key : '';
    final next = i + 1 < sheets.length ? sheets[i + 1].key : '';
    return Change.create(Node(id: randomId(), type: 'sheet', parent: 'book', key: keyBetween(before, next), attributes: {'name': name}, grid: const Grid.empty()));
  }

  /// A name no sheet has: Feuil2, Feuil3…
  String freeName(String prefix) {
    final names = {for (final s in book.sheets) '${s.attributes['name']}'.toLowerCase()};
    for (var n = book.sheets.length + 1; ; n++) {
      if (!names.contains('$prefix$n'.toLowerCase())) return '$prefix$n';
    }
  }

  /// Moves a sheet to [to] among the sheets.
  Change moveSheet(Node sheet, int to) {
    final others = [
      for (final s in book.sheets)
        if (s.id != sheet.id) s,
    ];
    to = to.clamp(0, others.length);
    final before = to > 0 ? others[to - 1].key : '';
    final after = to < others.length ? others[to].key : '';
    return Change.set(sheet.id, key: keyBetween(before, after));
  }

  // filling

  /// Fills [to] from [from], which it extends down, up, right or left:
  /// numbers go on as a series when there are several, text ending with a
  /// number counts on, dates go on day by day, formulas move.
  Edit fill(Node sheet, CellArea from, CellArea to) {
    final grid = sheet.grid ?? const Grid.empty();
    final vertical = to.rows > from.rows;
    final forward = vertical ? to.bottom > from.bottom : to.right > from.right;
    final k = vertical ? from.rows : from.cols;
    final m = (vertical ? to.rows : to.cols) - k;
    final cells = <Cell>[];
    for (final line in vertical ? [for (var c = from.left; c <= from.right; c++) c] : [for (var r = from.top; r <= from.bottom; r++) r]) {
      (int, int) at(int i) => vertical ? (from.top + i, line) : (line, from.left + i);
      final fields = [for (var i = 0; i < k; i++) grid.cell(at(i).$1, at(i).$2) ?? const <String, Object?>{}];
      final numbers = [
        for (final f in fields)
          if (f['v'] is num && f['f'] == null) (f['v']! as num).toDouble(),
      ];
      final series = numbers.length == k && k > 1;
      final step = series ? (numbers.last - numbers.first) / (k - 1) : 0.0;
      for (var j = 0; j < m; j++) {
        // the source cell this one repeats, and how far from it
        final i = forward ? j % k : k - 1 - j % k;
        final (r, c) = forward ? (vertical ? (from.bottom + 1 + j, line) : (line, from.right + 1 + j)) : (vertical ? (from.top - 1 - j, line) : (line, from.left - 1 - j));
        final (sr, sc) = at(i);
        final f = fields[i];
        final turns = j ~/ k + 1;
        final out = <String, Object?>{..._cleared, 's': f['s']};
        if (f['f'] is String) {
          out['f'] = moveFormula(f['f']! as String, r - sr, c - sc);
        } else if (series) {
          out['v'] = _clean(numbers.first + step * (forward ? k + j : -1 - j));
        } else if (f['v'] is String) {
          out['v'] = _counted(f['v']! as String, forward ? turns : -turns);
        } else if (f['v'] is num && k == 1 && book.style(f['s'] as String?).format.isDate) {
          out['v'] = (f['v']! as num) + (forward ? turns : -turns);
        } else {
          out['v'] = f['v'];
          out['e'] = f['e'];
        }
        cells.add(Cell(r, c, out));
      }
    }
    return cells.isEmpty ? Edit() : Edit([Change.cells(sheet.id, cells)]);
  }

  static num _clean(double n) => n == n.roundToDouble() && n.abs() < 1e15 ? n.round() : double.parse(n.toStringAsPrecision(15));

  /// Text ending with a number, counted on by [by]: "Trimestre 1" makes
  /// "Trimestre 2".
  static String _counted(String text, int by) {
    final m = RegExp(r'^(.*?)(\d+)$').firstMatch(text);
    if (m == null) return text;
    final n = int.parse(m[2]!) + by;
    return '${m[1]}${n < 0 ? 0 : n}';
  }

  // copying

  /// The cells of an area, to paste elsewhere.
  Clip copy(Node sheet, CellArea area) {
    final grid = sheet.grid ?? const Grid.empty();
    return Clip(area, [
      for (final c in grid.rows(area.top, area.bottom))
        if (c.row > 0 && c.col > 0 && area.contains(c.row, c.col))
          Cell(
            c.row - area.top,
            c.col - area.left,
            {...c.fields}
              ..remove('m')
              ..remove('src')
              ..removeWhere((k, _) => commentFields.contains(k)),
          ),
    ]);
  }

  /// Pastes cells copied at a cell: the area they cover is cleared first,
  /// their formulas move by as much as they did.
  Edit paste(Node sheet, Clip clip, int row, int col) {
    final grid = sheet.grid ?? const Grid.empty();
    final target = CellArea(row, col, row + clip.area.rows - 1, col + clip.area.cols - 1);
    final out = <(int, int), Map<String, Object?>>{};
    for (final c in grid.rows(target.top, target.bottom)) {
      if (c.col > 0 && target.contains(c.row, c.col)) out[(c.row, c.col)] = {..._cleared, 's': null};
    }
    for (final c in clip.cells) {
      final r = row + c.row, k = col + c.col;
      if (r > maxRows || k > maxCols) continue;
      final f = {..._cleared, 's': null, ...c.fields};
      if (f['f'] is String) f['f'] = moveFormula(f['f']! as String, row - clip.area.top, col - clip.area.left);
      out[(r, k)] = f;
    }
    return Edit([
      Change.cells(sheet.id, [for (final e in out.entries) Cell(e.key.$1, e.key.$2, e.value)]),
    ]);
  }

  /// Sorts the rows of an area by the values of column [by].
  Edit sort(Node sheet, CellArea area, int by, {bool descending = false}) {
    final grid = sheet.grid ?? const Grid.empty();
    final rows = [
      for (var r = area.top; r <= area.bottom; r++) (r, [for (var c = area.left; c <= area.right; c++) grid.cell(r, c) ?? const <String, Object?>{}]),
    ];
    int rank(Object? v) => v is num ? 0 : (v is String ? 1 : (v is bool ? 2 : 3));
    rows.sort((a, b) {
      final x = a.$2[by - area.left]['v'], y = b.$2[by - area.left]['v'];
      if (x == null || y == null) return x == null ? (y == null ? 0 : 1) : -1;
      var r = rank(x).compareTo(rank(y));
      if (r == 0) {
        if (x is num && y is num) r = x.compareTo(y);
        if (x is String && y is String) r = x.toLowerCase().compareTo(y.toLowerCase());
        if (x is bool && y is bool) r = (x ? 1 : 0).compareTo(y ? 1 : 0);
      }
      return descending ? -r : r;
    });
    final cells = <Cell>[];
    for (var i = 0; i < rows.length; i++) {
      final (from, fields) = rows[i];
      final r = area.top + i;
      for (var j = 0; j < fields.length; j++) {
        final f = {..._cleared, 's': null, ...fields[j]}
          ..remove('m')
          ..removeWhere((k, _) => commentFields.contains(k));
        if (f['f'] is String) f['f'] = moveFormula(f['f']! as String, r - from, 0);
        cells.add(Cell(r, area.left + j, f));
      }
    }
    return Edit([Change.cells(sheet.id, cells)]);
  }
}

/// Cells copied from an area, placed relative to its first cell.
class Clip {
  const Clip(this.area, this.cells);

  final CellArea area;
  final List<Cell> cells;
}

/// A formula copied [rows] down and [cols] right: its relative references
/// move, those that leave the sheet become #REF!.
String moveFormula(String f, int rows, int cols) {
  if (rows == 0 && cols == 0) return f;
  final b = StringBuffer();
  var i = 0;
  final ref = RegExp(r'(\$?)([A-Za-z]{1,3})(\$?)([0-9]{1,7})');
  while (i < f.length) {
    final c = f[i];
    if (c == '"' || c == "'") {
      var j = i + 1;
      while (j < f.length && !(f[j] == c && (j + 1 >= f.length || f[j + 1] != c))) {
        j += f[j] == c ? 2 : 1;
      }
      b.write(f.substring(i, (j + 1).clamp(0, f.length)));
      i = j + 1;
      continue;
    }
    final m = ref.matchAsPrefix(f, i);
    final prev = i > 0 ? f.codeUnitAt(i - 1) : 0;
    final boundary = !(prev >= 0x41 && prev <= 0x5A || prev >= 0x61 && prev <= 0x7A || prev >= 0x30 && prev <= 0x39 || prev == 0x5F || prev == 0x2E);
    if (m != null && boundary) {
      final end = m.end;
      final next = end < f.length ? f[end] : '';
      if (!RegExp(r'[A-Za-z0-9_(.]').hasMatch(next)) {
        var col = 0;
        for (final u in m[2]!.toUpperCase().codeUnits) {
          col = col * 26 + u - 0x40;
        }
        var row = int.parse(m[4]!);
        if (m[3]!.isEmpty) row += rows;
        if (m[1]!.isEmpty) col += cols;
        if (row < 1 || col < 1 || row > maxRows || col > maxCols) {
          b.write('#REF!');
        } else {
          b.write('${m[1]}${columnName(col)}${m[3]}$row');
        }
        i = end;
        continue;
      }
    }
    b.write(c);
    i++;
  }
  return b.toString();
}
