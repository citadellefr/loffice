import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import 'tables.dart';
import 'workbook.dart';

/// Asks which cells make a table of [sheet], [area] at first, and whether
/// their first row holds the headers. Null when cancelled.
Future<(CellArea, bool)?> showTableDialog(BuildContext context, {required Workbook book, required Node sheet, required CellArea area, LofficeStrings strings = const LofficeStrings()}) =>
    showDialog<(CellArea, bool)>(
      context: context,
      builder: (_) => _TableDialog(book: book, sheet: sheet, area: area, strings: strings),
    );

class _TableDialog extends StatefulWidget {
  const _TableDialog({required this.book, required this.sheet, required this.area, required this.strings});

  final Workbook book;
  final Node sheet;
  final CellArea area;
  final LofficeStrings strings;

  @override
  State<_TableDialog> createState() => _TableDialogState();
}

class _TableDialogState extends State<_TableDialog> {
  late final _range = TextEditingController(text: widget.area.name);
  var _headers = true;
  String? _problem;

  LofficeStrings get _s => widget.strings;

  @override
  void dispose() {
    _range.dispose();
    super.dispose();
  }

  void _accept() {
    final area = parseArea(_range.text.trim().toUpperCase());
    final problem = tableProblem(widget.book, widget.sheet, area, headers: _headers);
    if (problem == null) {
      Navigator.of(context).pop((area!, _headers));
      return;
    }
    setState(
      () => _problem = switch (problem) {
        TableProblem.range => _s.tableRangeInvalid,
        TableProblem.overlap => _s.tableOverlap,
        TableProblem.merged => _s.tableMerged,
      },
    );
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text(_s.createTable),
    content: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        TextField(
          controller: _range,
          autofocus: true,
          decoration: InputDecoration(labelText: _s.tableRange, errorText: _problem),
          onChanged: (_) => setState(() => _problem = null),
          onSubmitted: (_) => _accept(),
        ),
        CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          controlAffinity: ListTileControlAffinity.leading,
          title: Text(_s.tableHeaders),
          value: _headers,
          onChanged: (v) => setState(() => _headers = v ?? true),
        ),
      ],
    ),
    actions: [
      TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(_s.cancel)),
      FilledButton(onPressed: _accept, child: Text(_s.ok)),
    ],
  );
}
