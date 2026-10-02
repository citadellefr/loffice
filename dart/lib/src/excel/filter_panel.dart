import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../chrome/strings.dart';
import 'filter.dart';

enum FilterAction { sortAscending, sortDescending, clear, show }

/// What the menu of a filter's arrow was asked: to sort, to clear the
/// column's filter, or to show some of its values.
class FilterChoice {
  const FilterChoice(this.action, [this.column]);

  final FilterAction action;

  /// The values to show, null for all of them.
  final FilterColumn? column;
}

/// Opens the menu of a filter's arrow under [at], for a column named
/// [name] whose cells show [values], and blanks when [blanks].
Future<FilterChoice?> showFilterPanel(
  BuildContext context, {
  required Rect at,
  required String name,
  required List<String> values,
  required bool blanks,
  FilterColumn? current,
  LofficeStrings strings = const LofficeStrings(),
}) {
  final screen = MediaQuery.sizeOf(context);
  const width = 280.0, height = 440.0;
  return showDialog<FilterChoice>(
    context: context,
    barrierColor: Colors.transparent,
    builder: (_) => Stack(children: [
      Positioned(
        left: math.max(0, math.min(at.left, screen.width - width)),
        top: math.max(0, math.min(at.bottom, screen.height - height)),
        width: width,
        height: math.min(height, screen.height),
        child: Material(
          elevation: 8,
          child: _FilterPanel(name: name, values: values, blanks: blanks, current: current, strings: strings),
        ),
      ),
    ]),
  );
}

class _FilterPanel extends StatefulWidget {
  const _FilterPanel({required this.name, required this.values, required this.blanks, required this.current, required this.strings});

  final String name;
  final List<String> values;
  final bool blanks;
  final FilterColumn? current;
  final LofficeStrings strings;

  @override
  State<_FilterPanel> createState() => _FilterPanelState();
}

class _FilterPanelState extends State<_FilterPanel> {
  late final Set<String> _shown;
  late bool _blank;
  var _search = '';

  LofficeStrings get _s => widget.strings;

  @override
  void initState() {
    super.initState();
    final c = widget.current;
    final filtered = c != null && c.kept == 0;
    _shown = filtered ? {...widget.values.where(c.values.contains)} : {...widget.values};
    _blank = filtered ? c.blank : true;
  }

  bool get _all => _shown.length == widget.values.length && (_blank || !widget.blanks);

  bool get _none => _shown.isEmpty && (!_blank || !widget.blanks);

  void _pop(FilterChoice c) => Navigator.pop(context, c);

  @override
  Widget build(BuildContext context) {
    final search = _search.toLowerCase();
    final listed = [for (final v in widget.values) if (v.toLowerCase().contains(search)) v];
    Widget item(IconData icon, String label, VoidCallback? onTap) => ListTile(
      dense: true,
      leading: Icon(icon, size: 18),
      title: Text(label),
      enabled: onTap != null,
      onTap: onTap,
    );
    return Column(children: [
      item(Icons.arrow_downward, _s.sortAscending, () => _pop(const FilterChoice(FilterAction.sortAscending))),
      item(Icons.arrow_upward, _s.sortDescending, () => _pop(const FilterChoice(FilterAction.sortDescending))),
      const Divider(height: 1),
      item(Icons.filter_alt_off_outlined, _s.clearFilterFrom(widget.name),
          widget.current == null ? null : () => _pop(const FilterChoice(FilterAction.clear))),
      Padding(
        padding: const EdgeInsets.all(8),
        child: TextField(
          autofocus: true,
          decoration: InputDecoration(isDense: true, hintText: _s.search, prefixIcon: const Icon(Icons.search, size: 18), border: const OutlineInputBorder()),
          onChanged: (v) => setState(() => _search = v),
        ),
      ),
      Expanded(
        child: ListView(children: [
          if (search.isEmpty)
            CheckboxListTile(
              dense: true,
              controlAffinity: ListTileControlAffinity.leading,
              tristate: true,
              value: _all ? true : (_none ? false : null),
              title: Text(_s.selectAll),
              onChanged: (_) => setState(() {
                final all = !_all;
                _shown.clear();
                if (all) _shown.addAll(widget.values);
                _blank = all;
              }),
            ),
          for (final v in listed)
            CheckboxListTile(
              dense: true,
              controlAffinity: ListTileControlAffinity.leading,
              value: _shown.contains(v),
              title: Text(v),
              onChanged: (on) => setState(() => on == true ? _shown.add(v) : _shown.remove(v)),
            ),
          if (widget.blanks && search.isEmpty)
            CheckboxListTile(
              dense: true,
              controlAffinity: ListTileControlAffinity.leading,
              value: _blank,
              title: Text(_s.blanks),
              onChanged: (on) => setState(() => _blank = on == true),
            ),
        ]),
      ),
      Padding(
        padding: const EdgeInsets.all(8),
        child: Row(mainAxisAlignment: MainAxisAlignment.end, children: [
          FilledButton(
            onPressed: _none
                ? null
                : () => _pop(_all
                      ? const FilterChoice(FilterAction.clear)
                      : FilterChoice(FilterAction.show, FilterColumn(values: {..._shown}, blank: _blank && widget.blanks))),
            child: Text(_s.ok),
          ),
          const SizedBox(width: 8),
          OutlinedButton(onPressed: () => Navigator.pop(context), child: Text(_s.cancel)),
        ]),
      ),
    ]);
  }
}
