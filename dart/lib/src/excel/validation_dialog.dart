import 'package:flutter/material.dart';

import '../chrome/strings.dart';
import 'formula_text.dart';
import 'validation.dart';

/// The data validation dialog: what cells take, the message shown on
/// selecting them and the alert for what they do not take. Closes with
/// the validation as the Go package writes it, without its "ref", an
/// empty one for no validation at all, or null to leave things as they
/// are.
class ValidationDialog extends StatefulWidget {
  const ValidationDialog({super.key, required this.strings, this.current, this.date1904 = false});

  final LofficeStrings strings;

  /// The validation of the active cell, which the dialog starts from.
  final Validation? current;
  final bool date1904;

  @override
  State<ValidationDialog> createState() => _ValidationDialogState();
}

class _ValidationDialogState extends State<ValidationDialog> {
  late var _type = widget.current?.type ?? '';
  late var _op = widget.current?.op ?? 'between';
  late var _blank = widget.current?.blank ?? true;
  late var _arrow = widget.current?.arrow ?? true;
  late var _alert = widget.current == null ? true : widget.current!.error != null;
  late var _style = widget.current?.error ?? 'stop';
  late final _first = TextEditingController(text: _show(widget.current?.first));
  late final _second = TextEditingController(text: _show(widget.current?.second));
  late final _promptTitle = TextEditingController(text: widget.current?.promptTitle);
  late final _prompt = TextEditingController(text: widget.current?.prompt);
  late final _errorTitle = TextEditingController(text: widget.current?.errorTitle);
  late final _errorText = TextEditingController(text: widget.current?.errorText);

  @override
  void dispose() {
    for (final c in [_first, _second, _promptTitle, _prompt, _errorTitle, _errorText]) {
      c.dispose();
    }
    super.dispose();
  }

  bool get _compares => const ['whole', 'decimal', 'date', 'time', 'textLength'].contains(_type);

  bool get _two => _compares && (_op == 'between' || _op == 'notBetween');

  /// A bound or a list as it is typed: the values of a list apart by
  /// semicolons, the cells that hold them after "=".
  String _show(String? f) {
    if (f == null || f.isEmpty) return '';
    if (_type == 'list') {
      final quoted = f.length >= 2 && f.startsWith('"') && f.endsWith('"');
      return quoted ? f.substring(1, f.length - 1).replaceAll('""', '"').replaceAll(',', ';') : '=${formulaToFrench(f)}';
    }
    return boundText(f, type: _type, date1904: widget.date1904);
  }

  String _formula(String text) {
    final t = text.trim();
    if (_type != 'list') return boundFormula(t, date1904: widget.date1904);
    if (t.startsWith('=')) return formulaFromFrench(t.substring(1));
    return '"${t.replaceAll('"', '""').replaceAll(';', ',')}"';
  }

  void _changeType(String type) {
    setState(() {
      if ((type == 'list') != (_type == 'list')) _first.clear();
      _type = type;
    });
  }

  void _done() {
    final needs = _type.isNotEmpty;
    if (needs && (_first.text.trim().isEmpty || _two && _second.text.trim().isEmpty)) return;
    Navigator.of(context).pop({
      if (needs) ...{
        'type': _type,
        if (_compares && _op != 'between') 'op': _op,
        'f1': _formula(_first.text),
        if (_two) 'f2': _formula(_second.text),
        if (_blank) 'blank': true,
        if (_type == 'list' && !_arrow) 'noArrow': true,
        if (_alert) 'err': _style,
      },
      if (_errorTitle.text.isNotEmpty) 'errTitle': _errorTitle.text,
      if (_errorText.text.isNotEmpty) 'errText': _errorText.text,
      if (_promptTitle.text.isNotEmpty) 'promptTitle': _promptTitle.text,
      if (_prompt.text.isNotEmpty) 'prompt': _prompt.text,
    });
  }

  Widget _text(TextEditingController c, String label, {int? max, int lines = 1}) => Padding(
    padding: const EdgeInsets.only(top: 8),
    child: TextField(
      controller: c,
      maxLength: max,
      maxLines: lines,
      decoration: InputDecoration(labelText: label, isDense: true),
    ),
  );

  Widget _choice(String label, String value, List<(String, String)> choices, ValueChanged<String> onChanged) => InputDecorator(
    decoration: InputDecoration(labelText: label, isDense: true),
    child: DropdownButtonHideUnderline(
      child: DropdownButton<String>(
        value: value,
        isExpanded: true,
        isDense: true,
        items: [for (final (id, text) in choices) DropdownMenuItem(value: id, child: Text(text))],
        onChanged: (v) => onChanged(v ?? value),
      ),
    ),
  );

  Widget _options(LofficeStrings s) => ListView(
    children: [
      const SizedBox(height: 8),
      _choice(s.allow, _type, s.validationTypes, _changeType),
      if (_compares) ...[const SizedBox(height: 8), _choice(s.dataOperator, _op, s.validationOperators, (v) => setState(() => _op = v))],
      if (_type.isNotEmpty) _text(_first, _type == 'list' ? s.source : (_type == 'custom' ? s.formula : (_two ? s.minimum : s.value))),
      if (_two) _text(_second, s.maximum),
      if (_type.isNotEmpty)
        CheckboxListTile(
          dense: true,
          contentPadding: EdgeInsets.zero,
          controlAffinity: ListTileControlAffinity.leading,
          value: _blank,
          title: Text(s.ignoreBlank),
          onChanged: (v) => setState(() => _blank = v ?? false),
        ),
      if (_type == 'list')
        CheckboxListTile(
          dense: true,
          contentPadding: EdgeInsets.zero,
          controlAffinity: ListTileControlAffinity.leading,
          value: _arrow,
          title: Text(s.inCellDropdown),
          onChanged: (v) => setState(() => _arrow = v ?? true),
        ),
    ],
  );

  Widget _promptTab(LofficeStrings s) => ListView(children: [_text(_promptTitle, s.title, max: 32), _text(_prompt, s.message, max: 255, lines: 3)]);

  Widget _alertTab(LofficeStrings s) => ListView(
    children: [
      CheckboxListTile(
        dense: true,
        contentPadding: EdgeInsets.zero,
        controlAffinity: ListTileControlAffinity.leading,
        value: _alert,
        title: Text(s.showAlert),
        onChanged: (v) => setState(() => _alert = v ?? false),
      ),
      if (_alert) _choice(s.alertStyle, _style, s.alertStyles, (v) => setState(() => _style = v)),
      _text(_errorTitle, s.title, max: 32),
      _text(_errorText, s.message, max: 255, lines: 3),
    ],
  );

  @override
  Widget build(BuildContext context) {
    final s = widget.strings;
    return DefaultTabController(
      length: 3,
      child: AlertDialog(
        title: Text(s.dataValidation),
        content: SizedBox(
          width: 440,
          height: 380,
          child: Column(
            children: [
              TabBar(
                tabs: [
                  Tab(text: s.validationOptions),
                  Tab(text: s.validationPrompt),
                  Tab(text: s.validationAlert),
                ],
              ),
              Expanded(child: TabBarView(children: [_options(s), _promptTab(s), _alertTab(s)])),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(const <String, Object?>{}), child: Text(s.clearAllValidation)),
          TextButton(onPressed: () => Navigator.of(context).pop(), child: Text(s.cancel)),
          FilledButton(onPressed: _done, child: Text(s.ok)),
        ],
      ),
    );
  }
}
