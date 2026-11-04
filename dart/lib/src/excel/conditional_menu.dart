import 'package:flutter/material.dart';

import '../chrome/ribbon.dart';
import '../chrome/strings.dart';
import 'conditional.dart';
import 'formula_text.dart';
import 'rules.dart';
import 'workbook.dart';

/// The ribbon's menu of conditional formats, whose submenus open beside it.
class ConditionalMenu extends StatelessWidget {
  const ConditionalMenu({
    super.key,
    required this.strings,
    required this.enabled,
    required this.onHighlight,
    required this.onTopBottom,
    required this.onDataBar,
    required this.onColorScale,
    required this.onIconSet,
    required this.onClear,
    required this.onManage,
  });

  final LofficeStrings strings;
  final bool enabled;

  /// What a command of a submenu asks, by the ids of the strings' lists.
  final ValueChanged<String> onHighlight, onTopBottom, onDataBar, onColorScale, onIconSet;

  /// Takes the rules off the selection, or off the whole sheet.
  final ValueChanged<bool> onClear;
  final VoidCallback onManage;

  static const _menuIcons = ['3Arrows', '3TrafficLights1', '3Symbols', '3Stars', '4Arrows', '5Arrows', '5Rating', '5Quarters'];

  @override
  Widget build(BuildContext context) {
    final s = strings;
    Widget sub(String label, List<Widget> items) => SubmenuButton(menuChildren: items, child: Text(label));
    Widget item(String label, VoidCallback onPressed, {Widget? leading}) => MenuItemButton(onPressed: onPressed, leadingIcon: leading, child: Text(label));
    return MenuAnchor(
      menuChildren: [
        sub(s.highlightRules, [for (final (id, label) in s.highlightKinds) item(label, () => onHighlight(id))]),
        sub(s.topBottomRules, [for (final (id, label) in s.topBottomKinds) item(label, () => onTopBottom(id))]),
        sub(s.dataBars, [
          for (final (id, label) in s.dataBarColors) item(label, () => onDataBar(id), leading: _swatch([_hex(dataBarColors[id]!)])),
        ]),
        sub(s.colorScales, [
          for (final (id, label) in s.colorScaleSets) item(label, () => onColorScale(id), leading: _swatch([for (final c in colorScales[id]!) _hex(c)])),
        ]),
        sub(s.iconSets, [for (final id in _menuIcons) item(s.iconSetNames.firstWhere((n) => n.$1 == id).$2, () => onIconSet(id), leading: _IconPreview(id))]),
        const Divider(height: 1),
        sub(s.clearRules, [item(s.clearRulesSelection, () => onClear(false)), item(s.clearRulesSheet, () => onClear(true))]),
        item(s.manageRules, onManage),
      ],
      builder: (context, controller, _) =>
          RibbonButton(icon: const Icon(Icons.rule), label: s.conditionalFormatting, large: true, onPressed: enabled ? () => controller.isOpen ? controller.close() : controller.open() : null),
    );
  }

  static Color _hex(String rgb) => Color(0xFF000000 | int.parse(rgb, radix: 16));

  static Widget _swatch(List<Color> colors) => Container(
    width: 32,
    height: 12,
    decoration: BoxDecoration(gradient: LinearGradient(colors: colors.length == 1 ? [colors[0], colors[0].withValues(alpha: 0.15)] : colors)),
  );
}

/// The icons of a set side by side.
class _IconPreview extends StatelessWidget {
  const _IconPreview(this.set);

  final String set;

  @override
  Widget build(BuildContext context) => CustomPaint(size: Size(16.0 * int.parse(set.substring(0, 1)), 14), painter: _IconPreviewPainter(set));
}

class _IconPreviewPainter extends CustomPainter {
  _IconPreviewPainter(this.set);

  final String set;

  @override
  void paint(Canvas canvas, Size size) {
    final n = int.parse(set.substring(0, 1));
    for (var i = 0; i < n; i++) {
      paintIcon(canvas, Rect.fromLTWH(i * 16.0, 0, 14, 14), set, i);
    }
  }

  @override
  bool shouldRepaint(_IconPreviewPainter old) => old.set != set;
}

/// What a dialog of the highlight rules asks.
class HighlightRuleDialog extends StatefulWidget {
  const HighlightRuleDialog({super.key, required this.strings, required this.kind, required this.title, required this.anchor});

  final LofficeStrings strings;

  /// The command: one of the ids of the highlight and top/bottom menus.
  final String kind;
  final String title;

  /// The first cell of the selection, which the rule's formula is for.
  final String anchor;

  @override
  State<HighlightRuleDialog> createState() => _HighlightRuleDialogState();
}

class _HighlightRuleDialogState extends State<HighlightRuleDialog> {
  final _first = TextEditingController();
  final _second = TextEditingController();
  late final _rank = TextEditingController(text: '10');
  var _style = 'red';
  var _unique = false;

  @override
  void dispose() {
    _first.dispose();
    _second.dispose();
    _rank.dispose();
    super.dispose();
  }

  String get _kind => widget.kind;

  bool get _ranked => const ['top', 'bottom', 'topPercent', 'bottomPercent'].contains(_kind);

  /// A value typed as a formula in French, "=" allowed.
  static String _formula(String text) {
    final t = text.trim();
    return formulaFromFrench(t.startsWith('=') ? t.substring(1) : t);
  }

  Map<String, Object?>? _rule() {
    final (a, b) = (_first.text.trim(), _second.text.trim());
    switch (_kind) {
      case 'greaterThan' || 'lessThan' || 'equal':
        return a.isEmpty ? null : valueRule(_kind, widget.anchor, [_formula(a)], _style);
      case 'between':
        return a.isEmpty || b.isEmpty ? null : valueRule(_kind, widget.anchor, [_formula(a), _formula(b)], _style);
      case 'contains':
        return a.isEmpty ? null : containsRule(widget.anchor, a, _style);
      case 'duplicate':
        return duplicateRule(_style, unique: _unique);
      case 'above' || 'below':
        return averageRule(_style, below: _kind == 'below');
    }
    final rank = int.tryParse(_rank.text.trim());
    return rank == null || rank < 1 ? null : topRule(rank, _style, percent: _kind.endsWith('Percent'), bottom: _kind.startsWith('bottom'));
  }

  Widget _field(TextEditingController c, {String? label, bool autofocus = false}) => TextField(
    controller: c,
    autofocus: autofocus,
    decoration: InputDecoration(labelText: label, isDense: true),
    onSubmitted: (_) => _done(),
  );

  void _done() {
    final rule = _rule();
    if (rule != null) Navigator.of(context).pop(rule);
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(widget.title.replaceAll('…', '')),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(s.highlightPrompt(_kind)),
            const SizedBox(height: 8),
            if (_kind == 'between')
              Row(
                children: [
                  Expanded(child: _field(_first, autofocus: true)),
                  Padding(padding: const EdgeInsets.symmetric(horizontal: 8), child: Text(s.and)),
                  Expanded(child: _field(_second)),
                ],
              )
            else if (_kind == 'duplicate')
              DropdownButton<bool>(
                value: _unique,
                isExpanded: true,
                items: [
                  DropdownMenuItem(value: false, child: Text(s.duplicateValue)),
                  DropdownMenuItem(value: true, child: Text(s.uniqueValue)),
                ],
                onChanged: (v) => setState(() => _unique = v ?? false),
              )
            else if (_ranked)
              _field(_rank, label: s.rankCount, autofocus: true)
            else if (_kind != 'above' && _kind != 'below')
              _field(_first, autofocus: true),
            const SizedBox(height: 12),
            Text(s.formatWith),
            DropdownButton<String>(
              value: _style,
              isExpanded: true,
              items: [for (final (id, label) in s.highlightFormats) DropdownMenuItem(value: id, child: Text(label))],
              onChanged: (v) => setState(() => _style = v ?? 'red'),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: _done, child: Text(s.ok)),
      ],
    );
  }
}

/// The rules of a sheet in the order they count: each can go, move or be
/// followed by a new one written as a formula. Closes with the rules left.
class RulesManagerDialog extends StatefulWidget {
  const RulesManagerDialog({super.key, required this.strings, required this.rules, required this.area});

  final LofficeStrings strings;
  final List<Map<String, Object?>> rules;

  /// The selection, where a new rule applies by default.
  final CellArea area;

  @override
  State<RulesManagerDialog> createState() => _RulesManagerDialogState();
}

class _RulesManagerDialogState extends State<RulesManagerDialog> {
  late final _rules = [...widget.rules];

  String _describe(Map<String, Object?> rule) {
    final s = widget.strings;
    final f = rule['f'];
    final kind = switch (rule['type']) {
      'top10' => '${rule['bottom'] == true ? 'bottom' : 'top'}${rule['percent'] == true ? 'Percent' : ''}',
      'aboveAverage' => rule['below'] == true ? 'below' : 'above',
      final type => '$type',
    };
    return s.ruleDescription(kind, formula: f is List<Object?> && f.isNotEmpty ? formulaToFrench('${f.first}') : '', rank: rule['rank'] as int? ?? 10, icons: '${rule['icons']}');
  }

  void _move(int i, int by) => setState(() => _rules.insert(i + by, _rules.removeAt(i)));

  Future<void> _add() async {
    final rule = await showDialog<Map<String, Object?>>(
      context: context,
      builder: (context) => _FormulaRuleDialog(strings: widget.strings, area: widget.area),
    );
    if (rule != null) setState(() => _rules.insert(0, rule));
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(s.rulesManager),
      content: SizedBox(
        width: 560,
        height: 300,
        child: _rules.isEmpty
            ? Center(child: Text(s.noRules))
            : ListView(
                children: [
                  for (var i = 0; i < _rules.length; i++)
                    ListTile(
                      dense: true,
                      title: Text(_describe(_rules[i])),
                      subtitle: Text('${s.appliesTo} ${_rules[i]['ref']}'),
                      trailing: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          IconButton(tooltip: s.moveUp, onPressed: i == 0 ? null : () => _move(i, -1), icon: const Icon(Icons.arrow_upward, size: 18)),
                          IconButton(tooltip: s.moveDown, onPressed: i == _rules.length - 1 ? null : () => _move(i, 1), icon: const Icon(Icons.arrow_downward, size: 18)),
                          IconButton(tooltip: s.deleteRule, onPressed: () => setState(() => _rules.removeAt(i)), icon: const Icon(Icons.delete_outline, size: 18)),
                        ],
                      ),
                    ),
                ],
              ),
      ),
      actions: [
        TextButton(onPressed: _add, child: Text(s.newRule)),
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: () => Navigator.of(context).pop(_rules), child: Text(s.ok)),
      ],
    );
  }
}

/// A new rule that holds where a formula does, for an area.
class _FormulaRuleDialog extends StatefulWidget {
  const _FormulaRuleDialog({required this.strings, required this.area});

  final LofficeStrings strings;
  final CellArea area;

  @override
  State<_FormulaRuleDialog> createState() => _FormulaRuleDialogState();
}

class _FormulaRuleDialogState extends State<_FormulaRuleDialog> {
  late final _area = TextEditingController(text: widget.area.name);
  late final _formula = TextEditingController(text: '=');
  var _style = 'red';

  @override
  void dispose() {
    _area.dispose();
    _formula.dispose();
    super.dispose();
  }

  void _done() {
    final area = parseArea(_area.text.trim().toUpperCase());
    final text = _formula.text.trim().replaceFirst(RegExp('^='), '');
    if (area == null || text.isEmpty) return;
    Navigator.of(context).pop({...formulaRule(formulaFromFrench(text), _style), 'ref': area.name});
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(s.newFormulaRule),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: _area,
              decoration: InputDecoration(labelText: s.appliesTo, isDense: true),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _formula,
              autofocus: true,
              decoration: InputDecoration(labelText: s.ruleFormula, isDense: true),
              onSubmitted: (_) => _done(),
            ),
            const SizedBox(height: 12),
            DropdownButton<String>(
              value: _style,
              isExpanded: true,
              items: [for (final (id, label) in s.highlightFormats) DropdownMenuItem(value: id, child: Text(label))],
              onChanged: (v) => setState(() => _style = v ?? 'red'),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: _done, child: Text(s.ok)),
      ],
    );
  }
}
