import 'package:flutter/material.dart';

import '../chrome/strings.dart';
import 'chart_edits.dart';

InputDecoration _field(String label) => InputDecoration(labelText: label, isDense: true, border: const OutlineInputBorder());

/// The title of a chart: whether it has one and its text, empty for the
/// one made of its series. Closes with both, or null to leave things as
/// they are.
class ChartTitleDialog extends StatefulWidget {
  const ChartTitleDialog({super.key, required this.strings, required this.shown, required this.text});

  final LofficeStrings strings;
  final bool shown;
  final String text;

  @override
  State<ChartTitleDialog> createState() => _ChartTitleDialogState();
}

class _ChartTitleDialogState extends State<ChartTitleDialog> {
  late var _shown = widget.shown;
  late final _text = TextEditingController(text: widget.text);

  @override
  void dispose() {
    _text.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(s.chartTitle),
      content: SizedBox(
        width: 360,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CheckboxListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              controlAffinity: ListTileControlAffinity.leading,
              value: _shown,
              title: Text(s.showChartTitle),
              onChanged: (v) => setState(() => _shown = v ?? false),
            ),
            TextField(controller: _text, enabled: _shown, autofocus: true, decoration: _field(s.chartTitleText)),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: () => Navigator.of(context).pop((_shown, _text.text)), child: Text(s.ok)),
      ],
    );
  }
}

/// The titles of the two axes of a chart, empty for none. Closes with the
/// one of the categories and the one of the values, or null.
class AxisTitlesDialog extends StatefulWidget {
  const AxisTitlesDialog({super.key, required this.strings, required this.categories, required this.values});

  final LofficeStrings strings;
  final String categories;
  final String values;

  @override
  State<AxisTitlesDialog> createState() => _AxisTitlesDialogState();
}

class _AxisTitlesDialogState extends State<AxisTitlesDialog> {
  late final _categories = TextEditingController(text: widget.categories);
  late final _values = TextEditingController(text: widget.values);

  @override
  void dispose() {
    _categories.dispose();
    _values.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(s.axisTitles),
      content: SizedBox(
        width: 360,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(controller: _categories, autofocus: true, decoration: _field(s.categoryAxis)),
            const SizedBox(height: 12),
            TextField(controller: _values, decoration: _field(s.valueAxis)),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: () => Navigator.of(context).pop((_categories.text, _values.text)), child: Text(s.ok)),
      ],
    );
  }
}

class _SeriesRow {
  _SeriesRow(this.source, String name, String values) : name = TextEditingController(text: name), values = TextEditingController(text: values);

  final int? source;
  final TextEditingController name;
  final TextEditingController values;

  SeriesInput get input => SeriesInput(source, name.text, values.text);

  void dispose() {
    name.dispose();
    values.dispose();
  }
}

/// The data of a chart: its series, each with a name and the cells of its
/// values, and the cells of the categories they share. Closes with them,
/// or null.
class ChartDataDialog extends StatefulWidget {
  const ChartDataDialog({super.key, required this.strings, required this.series, required this.categories});

  final LofficeStrings strings;
  final List<SeriesInput> series;
  final String categories;

  @override
  State<ChartDataDialog> createState() => _ChartDataDialogState();
}

class _ChartDataDialogState extends State<ChartDataDialog> {
  late final _rows = [for (final s in widget.series) _SeriesRow(s.source, s.name, s.values)];
  late final _categories = TextEditingController(text: widget.categories);

  @override
  void dispose() {
    for (final r in _rows) {
      r.dispose();
    }
    _categories.dispose();
    super.dispose();
  }

  void _remove(_SeriesRow row) {
    setState(() => _rows.remove(row));
    row.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return AlertDialog(
      title: Text(s.selectData),
      content: SizedBox(
        width: 520,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              for (final row in _rows)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(controller: row.name, decoration: _field(s.seriesName)),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: TextField(controller: row.values, decoration: _field(s.seriesValues)),
                      ),
                      IconButton(tooltip: s.removeSeries, onPressed: () => _remove(row), icon: const Icon(Icons.delete_outline)),
                    ],
                  ),
                ),
              TextButton.icon(onPressed: () => setState(() => _rows.add(_SeriesRow(null, '', ''))), icon: const Icon(Icons.add), label: Text(s.addSeries)),
              const SizedBox(height: 8),
              TextField(controller: _categories, decoration: _field(s.categoryLabels)),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
        FilledButton(onPressed: () => Navigator.of(context).pop(([for (final r in _rows) r.input], _categories.text)), child: Text(s.ok)),
      ],
    );
  }
}
