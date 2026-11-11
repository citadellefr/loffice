import 'package:flutter/material.dart';

import '../chrome/strings.dart';

typedef Effects = List<Map<String, Object?>>;

/// The effects of the gallery the pane adds, by class: the class, preset and
/// subtype Go writes them from.
const _entrances = [('entr', 1, 0), ('entr', 10, 0), ('entr', 2, 4), ('entr', 2, 1), ('entr', 2, 8), ('entr', 2, 2), ('entr', 53, 16)];
const _exits = [('exit', 1, 0), ('exit', 10, 0)];

/// The animations of a slide as a list in the order they play, each with its
/// shape, its effect, how it starts, its delay and duration. An effect that
/// has XML of its own ("xml": what Go cannot write from its fields) is only
/// moved, started otherwise and deleted. Each edit sends the whole list.
class AnimationsPane extends StatelessWidget {
  const AnimationsPane({super.key, required this.strings, required this.effects, required this.nameOf, required this.selected, required this.readOnly, required this.onChanged, required this.onClose});

  final LofficeStrings strings;
  final Effects effects;

  /// The name of the shape of an effect, null when it has none.
  final String? Function(String id) nameOf;

  /// The shape new effects go on, null when none is selected.
  final String? selected;
  final bool readOnly;
  final ValueChanged<Effects> onChanged;
  final VoidCallback onClose;

  void _change(void Function(Effects list) edit) {
    final list = [for (final e in effects) Map.of(e)];
    edit(list);
    onChanged(list);
  }

  void _add((String, int, int) effect) {
    final (kind, preset, subtype) = effect;
    _change((list) => list.add({'target': selected, 'class': kind, 'preset': preset, if (subtype != 0) 'subtype': subtype, 'trigger': 'click', 'dur': 500}));
  }

  void _move(int from, int to) => _change((list) => list.insert(to, list.removeAt(from)));

  Widget _adder(String label, IconData icon, List<(String, int, int)> items) => PopupMenuButton<(String, int, int)>(
    tooltip: label,
    enabled: !readOnly && selected != null,
    onSelected: _add,
    itemBuilder: (_) => [for (final i in items) PopupMenuItem(value: i, child: Text(strings.animationEffect(i.$1, i.$2, i.$3)))],
    child: Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
      child: Row(mainAxisSize: MainAxisSize.min, children: [Icon(icon, size: 18), const SizedBox(width: 4), Text(label)]),
    ),
  );

  Widget _row(BuildContext context, int i) {
    final s = strings;
    final e = effects[i];
    final custom = (e['xml'] as String? ?? '').isNotEmpty;
    final target = e['target'] as String?;
    final para = e['para'] as Map<String, Object?>?;
    final name = [
      s.animationEffect(e['class'] as String? ?? '', (e['preset'] as num?)?.toInt() ?? 0, (e['subtype'] as num?)?.toInt() ?? 0),
      if (target != null) nameOf(target) ?? target,
      if (para != null) s.paragraphs((para['from'] as num).toInt(), (para['to'] as num).toInt()),
    ].join(' · ');
    final trigger = e['trigger'] as String? ?? 'click';
    return Padding(
      key: ValueKey('effect$i'),
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              SizedBox(width: 24, child: Text('${i + 1}')),
              Expanded(child: Text(name, overflow: TextOverflow.ellipsis)),
              IconButton(tooltip: s.moveUp, visualDensity: VisualDensity.compact, onPressed: readOnly || i == 0 ? null : () => _move(i, i - 1), icon: const Icon(Icons.arrow_upward, size: 18)),
              IconButton(
                tooltip: s.moveDown,
                visualDensity: VisualDensity.compact,
                onPressed: readOnly || i == effects.length - 1 ? null : () => _move(i, i + 1),
                icon: const Icon(Icons.arrow_downward, size: 18),
              ),
              IconButton(
                tooltip: s.delete,
                visualDensity: VisualDensity.compact,
                onPressed: readOnly ? null : () => _change((list) => list.removeAt(i)),
                icon: const Icon(Icons.delete_outline, size: 18),
              ),
            ],
          ),
          Row(
            children: [
              const SizedBox(width: 24),
              Expanded(
                child: DropdownButton<String>(
                  key: ValueKey('trigger$i'),
                  isDense: true,
                  isExpanded: true,
                  value: trigger,
                  items: [
                    for (final t in const ['click', 'with', 'after'])
                      DropdownMenuItem(
                        value: t,
                        child: Text(s.trigger(t), style: const TextStyle(fontSize: 12)),
                      ),
                  ],
                  onChanged: readOnly ? null : (t) => _change((list) => list[i]['trigger'] = t),
                ),
              ),
              const SizedBox(width: 8),
              _Seconds(
                key: ValueKey('delay$i'),
                label: s.delay,
                value: (e['delay'] as num?)?.toInt() ?? 0,
                onChanged: readOnly || custom ? null : (ms) => _change((list) => _set(list[i], 'delay', ms)),
              ),
              const SizedBox(width: 8),
              _Seconds(key: ValueKey('dur$i'), label: s.duration, value: (e['dur'] as num?)?.toInt() ?? 0, onChanged: readOnly || custom ? null : (ms) => _change((list) => _set(list[i], 'dur', ms))),
            ],
          ),
        ],
      ),
    );
  }

  static void _set(Map<String, Object?> effect, String key, int ms) {
    if (ms == 0) {
      effect.remove(key);
    } else {
      effect[key] = ms;
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = strings;
    return Material(
      color: Theme.of(context).colorScheme.surfaceContainerLow,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.only(left: 12),
            child: Row(
              children: [
                Expanded(child: Text(s.animationPane, style: Theme.of(context).textTheme.titleSmall)),
                IconButton(tooltip: s.close, onPressed: onClose, icon: const Icon(Icons.close)),
              ],
            ),
          ),
          Wrap(children: [_adder(s.addEntrance, Icons.login, _entrances), _adder(s.addExit, Icons.logout, _exits)]),
          const Divider(height: 1),
          Expanded(
            child: effects.isEmpty ? Padding(padding: const EdgeInsets.all(12), child: Text(s.noAnimationsHint)) : ListView(children: [for (var i = 0; i < effects.length; i++) _row(context, i)]),
          ),
        ],
      ),
    );
  }
}

/// A time in seconds, edited as text, sent in milliseconds when it is a
/// number of at least 0 and different.
class _Seconds extends StatefulWidget {
  const _Seconds({super.key, required this.label, required this.value, required this.onChanged});

  final String label;
  final int value;
  final ValueChanged<int>? onChanged;

  @override
  State<_Seconds> createState() => _SecondsState();
}

class _SecondsState extends State<_Seconds> {
  final _controller = TextEditingController();
  final _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    _show();
    _focus.addListener(() {
      if (!_focus.hasFocus) _submit();
    });
  }

  @override
  void didUpdateWidget(_Seconds old) {
    super.didUpdateWidget(old);
    if (!_focus.hasFocus) _show();
  }

  @override
  void dispose() {
    _controller.dispose();
    _focus.dispose();
    super.dispose();
  }

  void _show() => _controller.text = '${(widget.value / 1000).toStringAsFixed(2).replaceAll('.', ',')} s';

  void _submit() {
    final n = double.tryParse(_controller.text.replaceAll('s', '').replaceAll(',', '.').trim());
    final ms = n == null ? null : (n * 1000).round();
    if (ms != null && ms >= 0 && ms != widget.value) widget.onChanged?.call(ms);
    _show();
  }

  @override
  Widget build(BuildContext context) => Tooltip(
    message: widget.label,
    child: SizedBox(
      width: 64,
      height: 24,
      child: TextField(
        controller: _controller,
        focusNode: _focus,
        enabled: widget.onChanged != null,
        style: const TextStyle(fontSize: 12),
        decoration: const InputDecoration(isDense: true, border: OutlineInputBorder(), contentPadding: EdgeInsets.symmetric(horizontal: 6, vertical: 6)),
        onSubmitted: (_) => _submit(),
      ),
    ),
  );
}
