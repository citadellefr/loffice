import 'dart:math';

import 'package:flutter/material.dart';

import '../chrome/strings.dart';

/// The table of the data of a chart as the Go package takes it: categories
/// and series, each a list of cells that are a number, a text or null.
class _Table {
  _Table(Map<String, Object?> data)
    : categories = [...?data['categories'] as List<Object?>?],
      series = [
        for (final s in data['series'] as List<Object?>? ?? const [])
          {
            ...s! as Map<String, Object?>,
            'values': [...?(s as Map<String, Object?>)['values'] as List<Object?>?],
          },
      ];

  final List<Object?> categories;
  final List<Map<String, Object?>> series;

  List<Object?> _values(int series) => this.series[series]['values']! as List<Object?>;

  int get rows => [categories.length, for (var s = 0; s < series.length; s++) _values(s).length].reduce(max);

  static void _fit(List<Object?> cells, int length) {
    while (cells.length < length) {
      cells.add(null);
    }
  }

  Object? category(int row) => row < categories.length ? categories[row] : null;

  Object? value(int series, int row) => row < _values(series).length ? _values(series)[row] : null;

  void setCategory(int row, Object? value) {
    _fit(categories, row + 1);
    categories[row] = value;
  }

  void setValue(int series, int row, Object? value) {
    _fit(_values(series), row + 1);
    _values(series)[row] = value;
  }

  void setName(int series, String? name) {
    if (name == null) {
      this.series[series].remove('name');
    } else {
      this.series[series]['name'] = name;
    }
  }

  void addRow() {
    final row = rows;
    _fit(categories, row + 1);
    for (var s = 0; s < series.length; s++) {
      _fit(_values(s), row + 1);
    }
  }

  void removeRow(int row) {
    if (row < categories.length) categories.removeAt(row);
    for (var s = 0; s < series.length; s++) {
      if (row < _values(s).length) _values(s).removeAt(row);
    }
  }

  /// Adds a series with an index no other has.
  void addSeries() {
    final index = series.map((s) => (s['index'] as num?)?.toInt() ?? 0).fold(-1, max) + 1;
    series.add({'index': index, 'values': List<Object?>.filled(rows, null, growable: true)});
  }

  void removeSeries(int series) => this.series.removeAt(series);

  Map<String, Object?> toJson() {
    final n = rows;
    _fit(categories, n);
    for (var s = 0; s < series.length; s++) {
      _fit(_values(s), n);
    }
    return {if (categories.any((c) => c != null)) 'categories': categories, 'series': series};
  }
}

Object? _cell(String text) {
  final t = text.trim();
  return t.isEmpty ? null : num.tryParse(t.replaceAll(',', '.')) ?? t;
}

String? _name(String text) => text.isEmpty ? null : text;

String _show(Object? value) => switch (value) {
  null => '',
  final double d when d == d.roundToDouble() && d.abs() < 1e15 => '${d.toInt()}',
  _ => '$value',
};

/// The data of a chart as a grid, the categories down and a series by
/// column; each edit sends the whole table. A chart whose data are not in
/// a workbook embedded with it ([data] null) only says so.
class ChartDataPane extends StatelessWidget {
  const ChartDataPane({super.key, required this.strings, required this.data, required this.readOnly, required this.onChanged, required this.onClose});

  final LofficeStrings strings;
  final Map<String, Object?>? data;
  final bool readOnly;
  final ValueChanged<Map<String, Object?>> onChanged;
  final VoidCallback onClose;

  void _change(void Function(_Table table) edit) {
    final table = _Table(data!);
    edit(table);
    onChanged(table.toJson());
  }

  @override
  Widget build(BuildContext context) {
    final s = strings;
    final data = this.data;
    final table = data == null ? null : _Table(data);
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainerLow,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.only(left: 12),
            child: Row(
              children: [
                Expanded(child: Text(s.chartData, style: Theme.of(context).textTheme.titleSmall)),
                IconButton(tooltip: s.close, onPressed: onClose, icon: const Icon(Icons.close)),
              ],
            ),
          ),
          const Divider(height: 1),
          if (table == null)
            Padding(padding: const EdgeInsets.all(12), child: Text(s.chartReadOnly))
          else
            Expanded(
              child: SingleChildScrollView(
                scrollDirection: Axis.horizontal,
                child: SingleChildScrollView(
                  padding: const EdgeInsets.all(8),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          const SizedBox(width: 36),
                          SizedBox(width: 110, child: Text(s.categoryLabels, style: Theme.of(context).textTheme.labelSmall)),
                          for (var k = 0; k < table.series.length; k++)
                            SizedBox(
                              width: 150,
                              child: Row(
                                children: [
                                  Expanded(
                                    child: _CellField(
                                      key: ValueKey('name$k'),
                                      label: s.seriesName,
                                      value: table.series[k]['name'],
                                      enabled: !readOnly,
                                      parse: _name,
                                      onChanged: (v) => _change((t) => t.setName(k, v as String?)),
                                    ),
                                  ),
                                  IconButton(tooltip: s.removeSeries, onPressed: readOnly ? null : () => _change((t) => t.removeSeries(k)), icon: const Icon(Icons.delete_outline, size: 18)),
                                ],
                              ),
                            ),
                        ],
                      ),
                      for (var i = 0; i < table.rows; i++)
                        Row(
                          children: [
                            SizedBox(
                              width: 36,
                              child: IconButton(tooltip: s.removeRow, onPressed: readOnly ? null : () => _change((t) => t.removeRow(i)), icon: const Icon(Icons.remove_circle_outline, size: 18)),
                            ),
                            SizedBox(
                              width: 110,
                              child: _CellField(key: ValueKey('cat$i'), value: table.category(i), enabled: !readOnly, parse: _cell, onChanged: (v) => _change((t) => t.setCategory(i, v))),
                            ),
                            for (var k = 0; k < table.series.length; k++)
                              SizedBox(
                                width: 150,
                                child: _CellField(key: ValueKey('cell$k.$i'), value: table.value(k, i), enabled: !readOnly, parse: _cell, onChanged: (v) => _change((t) => t.setValue(k, i, v))),
                              ),
                          ],
                        ),
                    ],
                  ),
                ),
              ),
            ),
          if (table != null)
            Wrap(
              children: [
                TextButton.icon(onPressed: readOnly ? null : () => _change((t) => t.addRow()), icon: const Icon(Icons.add), label: Text(s.addRow)),
                TextButton.icon(onPressed: readOnly ? null : () => _change((t) => t.addSeries()), icon: const Icon(Icons.add), label: Text(s.addSeries)),
              ],
            ),
        ],
      ),
    );
  }
}

/// A cell of the grid, sent when the focus leaves it.
class _CellField extends StatefulWidget {
  const _CellField({super.key, this.label, required this.value, required this.enabled, required this.parse, required this.onChanged});

  final String? label;
  final Object? value;
  final bool enabled;
  final Object? Function(String text) parse;
  final ValueChanged<Object?> onChanged;

  @override
  State<_CellField> createState() => _CellFieldState();
}

class _CellFieldState extends State<_CellField> {
  late final _text = TextEditingController(text: _show(widget.value));
  final _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    _focus.addListener(() {
      if (!_focus.hasFocus && _text.text != _show(widget.value)) widget.onChanged(widget.parse(_text.text));
    });
  }

  @override
  void didUpdateWidget(_CellField old) {
    super.didUpdateWidget(old);
    if (widget.value != old.value && !_focus.hasFocus) _text.text = _show(widget.value);
  }

  @override
  void dispose() {
    _text.dispose();
    _focus.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.all(2),
    child: TextField(
      controller: _text,
      focusNode: _focus,
      enabled: widget.enabled,
      onSubmitted: (_) => _focus.unfocus(),
      decoration: InputDecoration(labelText: widget.label, isDense: true, border: const OutlineInputBorder()),
    ),
  );
}
