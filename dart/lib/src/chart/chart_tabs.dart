import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

import '../chrome/ribbon.dart';
import '../chrome/strings.dart';
import '../excel/chart_dialogs.dart' show ChartTitleDialog;
import 'chart.dart';
import 'chart_edits.dart';

List<PopupMenuEntry<ChartType>> chartTypeItems(LofficeStrings s, ChartType? current) => [
  for (final (key, label) in s.chartTypes) CheckedPopupMenuItem(value: ChartType.values.byName(key), checked: ChartType.values.byName(key) == current, child: Text(label)),
];

/// The tabs of the chart a node of a document holds, selected: its type and
/// data, its title and legend. [onChange] has a change made to the chart
/// as it is when it takes it.
List<RibbonTab> chartTabs(
  BuildContext context,
  LofficeStrings s,
  Node node, {
  required bool editable,
  required ValueChanged<void Function(Map<String, Object?> chart)> onChange,
  required VoidCallback onData,
}) {
  final spec = ChartSpec.fromJson(node.attributes['chart']);
  if (spec == null) return const [];
  final plot = spec.plots.firstOrNull;
  Future<void> title() async {
    final picked = await showDialog<(bool, String)>(
      context: context,
      builder: (context) => ChartTitleDialog(strings: s, shown: spec.title != null, text: spec.title?.text ?? ''),
    );
    if (picked != null && context.mounted) onChange((chart) => ChartChanges.setTitle(chart, picked.$1 ? picked.$2 : null));
  }

  return [
    RibbonTab(s.chartDesign, [
      RibbonGroup(s.chartTypeGroup, [
        RibbonMenu<ChartType>(
          icon: const Icon(Icons.bar_chart),
          label: s.changeChartType,
          large: true,
          enabled: editable && plot != null,
          items: chartTypeItems(s, plot == null ? null : ChartType.of(plot)),
          onSelected: (type) => onChange((chart) => ChartChanges.retype(chart, type)),
        ),
      ]),
      RibbonGroup(s.chartData, [RibbonButton(icon: const Icon(Icons.table_chart_outlined), label: s.selectData, large: true, onPressed: onData)]),
    ]),
    RibbonTab(s.chartFormat, [
      RibbonGroup(s.chartLayouts, [
        RibbonButton(icon: const Icon(Icons.title), label: s.chartTitle, large: true, selected: spec.title != null, onPressed: editable ? title : null),
        RibbonMenu<String>(
          icon: const Icon(Icons.format_list_bulleted),
          label: s.legend,
          large: true,
          enabled: editable,
          items: [for (final (key, label) in s.legendPositions) CheckedPopupMenuItem(value: key, checked: key == (spec.legend?.position ?? 'none'), child: Text(label))],
          onSelected: (pos) => onChange((chart) => ChartChanges.setLegend(chart, pos == 'none' ? null : pos)),
        ),
      ]),
    ]),
  ];
}
