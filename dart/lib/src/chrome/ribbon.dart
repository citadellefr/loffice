import 'package:flutter/material.dart';

import 'strings.dart';

/// A tab of the ribbon: its groups of commands, in order.
class RibbonTab {
  const RibbonTab(this.label, this.groups);

  final String label;
  final List<RibbonGroup> groups;
}

/// A group of a tab, named under its commands as Office does.
class RibbonGroup {
  const RibbonGroup(this.label, this.items);

  final String label;
  final List<Widget> items;
}

/// The ribbon of Office: File first, which opens the backstage, then the
/// tabs; the commands of the tab chosen below. On a phone, as Office there,
/// the tab is chosen from a list and its commands are icons in one row
/// that scrolls, which may be folded away.
class Ribbon extends StatefulWidget {
  const Ribbon({
    super.key,
    required this.fileLabel,
    required this.tabs,
    required this.onFile,
    this.leading = const [],
    this.trailing = const [],
    this.initialTab = 0,
  });

  final String fileLabel;
  final List<RibbonTab> tabs;
  final VoidCallback onFile;

  /// The quick access toolbar, before the tabs.
  final List<Widget> leading;

  /// What follows the tabs: who is here, the state of the document.
  final List<Widget> trailing;
  final int initialTab;

  @override
  State<Ribbon> createState() => _RibbonState();
}

/// The width below which the editors are laid out for a phone.
const phoneWidth = 600.0;

class _RibbonState extends State<Ribbon> {
  late var _tab = widget.initialTab;
  var _open = true;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, box) => box.maxWidth < phoneWidth ? _compact(context) : _full(context));
  }

  Widget _compact(BuildContext context) {
    final theme = Theme.of(context);
    final tab = widget.tabs[_tab.clamp(0, widget.tabs.length - 1)];
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            height: 44,
            child: Row(
              children: [
                ...widget.leading,
                PopupMenuButton<int>(
                  tooltip: '',
                  onSelected: (i) {
                    if (i < 0) return widget.onFile();
                    setState(() {
                      _tab = i;
                      _open = true;
                    });
                  },
                  itemBuilder: (_) => [
                    PopupMenuItem(value: -1, child: Text(widget.fileLabel)),
                    for (var i = 0; i < widget.tabs.length; i++) PopupMenuItem(value: i, child: Text(widget.tabs[i].label)),
                  ],
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                    child: Row(children: [
                      Text(tab.label, style: TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: theme.colorScheme.primary)),
                      Icon(Icons.arrow_drop_down, color: theme.colorScheme.primary),
                    ]),
                  ),
                ),
                const Spacer(),
                ...widget.trailing,
                IconButton(
                  icon: Icon(_open ? Icons.expand_less : Icons.expand_more),
                  onPressed: () => setState(() => _open = !_open),
                ),
              ],
            ),
          ),
          if (_open)
            Container(
              height: 48,
              decoration: BoxDecoration(
                color: theme.colorScheme.surface,
                border: Border(bottom: BorderSide(color: theme.dividerColor)),
              ),
              child: _Compact(
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  children: [
                    for (final (i, g) in tab.groups.indexed) ...[
                      if (i > 0) const VerticalDivider(width: 9, indent: 8, endIndent: 8),
                      for (final item in g.items) Center(child: item),
                    ],
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _full(BuildContext context) {
    final theme = Theme.of(context);
    final tab = widget.tabs[_tab.clamp(0, widget.tabs.length - 1)];
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            height: 36,
            child: Row(
              children: [
                ...widget.leading,
                Expanded(
                  child: ListView(
                    scrollDirection: Axis.horizontal,
                    children: [
                      _TabButton(label: widget.fileLabel, selected: false, file: true, onTap: widget.onFile),
                      for (var i = 0; i < widget.tabs.length; i++)
                        _TabButton(label: widget.tabs[i].label, selected: i == _tab, onTap: () => setState(() => _tab = i)),
                    ],
                  ),
                ),
                ...widget.trailing,
              ],
            ),
          ),
          Container(
            height: 96,
            decoration: BoxDecoration(
              color: theme.colorScheme.surface,
              border: Border(bottom: BorderSide(color: theme.dividerColor)),
            ),
            child: ListView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 4),
              children: [
                for (final g in tab.groups) _Group(group: g),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Tells the commands below to show as on a phone: icons alone, large
/// enough for a finger.
class _Compact extends InheritedWidget {
  const _Compact({required super.child});

  static bool of(BuildContext context) => context.dependOnInheritedWidgetOfExactType<_Compact>() != null;

  @override
  bool updateShouldNotify(_Compact oldWidget) => false;
}

class _TabButton extends StatelessWidget {
  const _TabButton({required this.label, required this.selected, required this.onTap, this.file = false});

  final String label;
  final bool selected;
  final bool file;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return InkWell(
      onTap: onTap,
      child: Container(
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: file ? scheme.primary : null,
          border: selected ? Border(bottom: BorderSide(color: scheme.primary, width: 3)) : null,
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            color: file ? scheme.onPrimary : (selected ? scheme.primary : scheme.onSurface),
            fontWeight: selected ? FontWeight.w600 : FontWeight.normal,
          ),
        ),
      ),
    );
  }
}

class _Group extends StatelessWidget {
  const _Group({required this.group});

  final RibbonGroup group;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      padding: const EdgeInsets.fromLTRB(6, 4, 6, 2),
      decoration: BoxDecoration(border: Border(right: BorderSide(color: theme.dividerColor))),
      child: Column(
        children: [
          Expanded(
            child: Wrap(
              direction: Axis.vertical,
              spacing: 2,
              runSpacing: 2,
              children: group.items,
            ),
          ),
          Text(group.label, style: theme.textTheme.labelSmall?.copyWith(color: theme.hintColor)),
        ],
      ),
    );
  }
}

/// A command of the ribbon: a small icon button, or a large one with its
/// label below.
class RibbonButton extends StatelessWidget {
  const RibbonButton({
    super.key,
    required this.icon,
    required this.label,
    this.onPressed,
    this.selected = false,
    this.large = false,
    this.shortcut,
  });

  final Widget icon;
  final String label;
  final VoidCallback? onPressed;
  final bool selected;
  final bool large;

  /// The keys that do the same, in the tooltip.
  final String? shortcut;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final tip = shortcut == null ? label : '$label ($shortcut)';
    final color = onPressed == null ? scheme.onSurface.withValues(alpha: 0.38) : scheme.onSurface;
    final content = _Compact.of(context)
        ? SizedBox(width: 40, height: 40, child: Center(child: IconTheme(data: IconThemeData(size: 22, color: color), child: icon)))
        : large
        ? SizedBox(
            width: 64,
            height: 66,
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                IconTheme(data: IconThemeData(size: 28, color: color), child: icon),
                const SizedBox(height: 2),
                Text(label, maxLines: 2, textAlign: TextAlign.center, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 11, color: color)),
              ],
            ),
          )
        : SizedBox(width: 28, height: 22, child: Center(child: IconTheme(data: IconThemeData(size: 18, color: color), child: icon)));
    return Tooltip(
      message: tip,
      child: Material(
        color: selected ? scheme.primary.withValues(alpha: 0.15) : Colors.transparent,
        borderRadius: BorderRadius.circular(3),
        child: InkWell(borderRadius: BorderRadius.circular(3), onTap: onPressed, child: content),
      ),
    );
  }
}

/// A command that opens a menu of choices.
class RibbonMenu<T> extends StatelessWidget {
  const RibbonMenu({
    super.key,
    required this.icon,
    required this.label,
    required this.items,
    required this.onSelected,
    this.large = false,
    this.enabled = true,
  });

  final Widget icon;
  final String label;
  final List<PopupMenuEntry<T>> items;
  final ValueChanged<T> onSelected;
  final bool large;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return PopupMenuButton<T>(
      tooltip: label,
      enabled: enabled,
      onSelected: onSelected,
      itemBuilder: (_) => items,
      child: IgnorePointer(
        child: RibbonButton(icon: icon, label: label, large: large, onPressed: enabled ? () {} : null),
      ),
    );
  }
}

/// A choice among values, as the font and size boxes of the ribbon.
class RibbonDropdown extends StatelessWidget {
  const RibbonDropdown({
    super.key,
    required this.label,
    required this.value,
    required this.values,
    required this.onChanged,
    this.width = 120,
    this.display,
  });

  final String label;
  final String? value;
  final List<String> values;
  final ValueChanged<String>? onChanged;
  final double width;
  final String Function(String)? display;

  @override
  Widget build(BuildContext context) {
    final show = display ?? (v) => v;
    return Tooltip(
      message: label,
      child: SizedBox(
        width: _Compact.of(context) ? width.clamp(0, 110) : width,
        height: _Compact.of(context) ? 36 : 24,
        child: PopupMenuButton<String>(
          enabled: onChanged != null,
          onSelected: onChanged,
          itemBuilder: (_) => [for (final v in values) PopupMenuItem(value: v, height: 32, child: Text(show(v)))],
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6),
            decoration: BoxDecoration(border: Border.all(color: Theme.of(context).dividerColor), borderRadius: BorderRadius.circular(2)),
            child: Row(
              children: [
                Expanded(child: Text(value == null ? '' : show(value!), overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 12))),
                const Icon(Icons.arrow_drop_down, size: 16),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// The colors a color command offers: the theme's with lighter and darker
/// shades, and Office's standard ones. The value chosen is a color as the
/// Go package writes it, null for none or automatic.
class ColorPalette extends StatelessWidget {
  const ColorPalette({
    super.key,
    required this.theme,
    required this.onSelected,
    required this.themeLabel,
    required this.standardLabel,
    this.noneLabel,
  });

  /// The colors of the theme, by the name of their scheme.
  final Map<String, Color> theme;
  final ValueChanged<Map<String, Object?>?> onSelected;
  final String themeLabel;
  final String standardLabel;
  final String? noneLabel;

  static const schemes = ['lt1', 'dk1', 'lt2', 'dk2', 'accent1', 'accent2', 'accent3', 'accent4', 'accent5', 'accent6'];
  static const standard = ['C00000', 'FF0000', 'FFC000', 'FFFF00', '92D050', '00B050', '00B0F0', '0070C0', '002060', '7030A0'];

  /// The shades Office offers for a color of the theme, lightest first.
  static List<List<List<Object>>> shades(String scheme) {
    final dark = scheme == 'dk1' || scheme == 'dk2';
    if (scheme == 'lt1') {
      return [
        [['lumMod', 95000]], [['lumMod', 85000]], [['lumMod', 75000]], [['lumMod', 65000]], [['lumMod', 50000]],
      ];
    }
    if (dark) {
      return [
        [['lumMod', 50000], ['lumOff', 50000]], [['lumMod', 65000], ['lumOff', 35000]], [['lumMod', 75000], ['lumOff', 25000]],
        [['lumMod', 85000], ['lumOff', 15000]], [['lumMod', 95000], ['lumOff', 5000]],
      ];
    }
    return [
      [['lumMod', 20000], ['lumOff', 80000]], [['lumMod', 40000], ['lumOff', 60000]], [['lumMod', 60000], ['lumOff', 40000]],
      [['lumMod', 75000]], [['lumMod', 50000]],
    ];
  }

  /// The color a value of the palette stands for, with the colors of the
  /// theme.
  static Color resolve(Map<String, Color> theme, Map<String, Object?> value) {
    final rgb = value['rgb'];
    if (rgb is String) return Color(0xFF000000 | (int.tryParse(rgb, radix: 16) ?? 0));
    final base = theme[value['scheme']] ?? const Color(0xFF808080);
    final mods = value['mods'];
    return mods is List ? _shade(base, [for (final m in mods) (m as List).cast<Object>()]) : base;
  }

  static Color _shade(Color base, List<List<Object>> mods) {
    final hsl = HSLColor.fromColor(base);
    var l = hsl.lightness;
    for (final m in mods) {
      final v = (m[1] as int) / 100000;
      l = m[0] == 'lumMod' ? l * v : l + v;
    }
    return hsl.withLightness(l.clamp(0, 1)).toColor();
  }

  @override
  Widget build(BuildContext context) {
    Widget swatch(Color color, Map<String, Object?> value) => InkWell(
      onTap: () => onSelected(value),
      child: Container(
        width: 18,
        height: 18,
        margin: const EdgeInsets.all(1),
        decoration: BoxDecoration(color: color, border: Border.all(color: const Color(0x33000000))),
      ),
    );
    return Padding(
      padding: const EdgeInsets.all(8),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(themeLabel, style: Theme.of(context).textTheme.labelMedium),
          Row(children: [
            for (final s in schemes) swatch(theme[s] ?? const Color(0xFF808080), {'scheme': s}),
          ]),
          const SizedBox(height: 4),
          for (var row = 0; row < 5; row++)
            Row(children: [
              for (final s in schemes)
                swatch(_shade(theme[s] ?? const Color(0xFF808080), shades(s)[row]), {'scheme': s, 'mods': shades(s)[row]}),
            ]),
          const SizedBox(height: 8),
          Text(standardLabel, style: Theme.of(context).textTheme.labelMedium),
          Row(children: [
            for (final rgb in standard) swatch(Color(0xFF000000 | int.parse(rgb, radix: 16)), {'rgb': rgb}),
          ]),
          if (noneLabel != null)
            TextButton(onPressed: () => onSelected(null), child: Text(noneLabel!)),
        ],
      ),
    );
  }
}

/// An option of the ribbon that is on or off, as the table style options
/// of Office.
class RibbonCheck extends StatelessWidget {
  const RibbonCheck({super.key, required this.label, required this.value, this.onChanged});

  final String label;
  final bool value;
  final ValueChanged<bool>? onChanged;

  @override
  Widget build(BuildContext context) {
    final color = onChanged == null ? Theme.of(context).colorScheme.onSurface.withValues(alpha: 0.38) : null;
    return InkWell(
      onTap: onChanged == null ? null : () => onChanged!(!value),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        SizedBox(
          width: 22,
          height: 20,
          child: Checkbox(
            value: value,
            onChanged: onChanged == null ? null : (v) => onChanged!(v ?? false),
            visualDensity: VisualDensity.compact,
            materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
          ),
        ),
        const SizedBox(width: 4),
        Text(label, style: TextStyle(fontSize: 12, color: color)),
        const SizedBox(width: 6),
      ]),
    );
  }
}

/// The grid Office offers to pick the size of a new table.
class TableSizeGrid extends StatefulWidget {
  const TableSizeGrid({super.key, required this.strings, required this.onSelected});

  final LofficeStrings strings;
  final void Function(int columns, int rows) onSelected;

  @override
  State<TableSizeGrid> createState() => _TableSizeGridState();
}

class _TableSizeGridState extends State<TableSizeGrid> {
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
