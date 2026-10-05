import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import '../drawing/color.dart';
import 'builtin_styles.dart';
import 'deck.dart';
import 'table.dart';

/// The table styles of PowerPoint's gallery, drawn with the options of a
/// table: those the presentation holds, then Office's light, medium and
/// dark ones.
class TableStyleGallery extends StatelessWidget {
  const TableStyleGallery({
    super.key,
    required this.deck,
    required this.table,
    required this.strings,
    required this.onSelected,
  });

  final Deck deck;

  /// The frame of the table, whose options and colors the styles are drawn in.
  final Node table;
  final LofficeStrings strings;
  final ValueChanged<String> onSelected;

  /// The width of the gallery: seven styles a row, their margins and
  /// frames around them.
  static const width = 7 * (_thumbWidth + 12);

  static const _thumbWidth = 56.0;

  @override
  Widget build(BuildContext context) {
    final page = deck.pageOf(table);
    final colors = page == null ? const ColorContext() : deck.colorsOf(page);
    final theme = page == null ? DeckTheme(null) : deck.themeOf(page);
    final options = table.attributes['tbl'] as Map<String, Object?>? ?? const {};
    final custom = [for (final id in deck.tableStyleIds) if (!builtinTableStyles.containsKey(id)) (id, strings.customStyle)];
    final sections = <(String, List<(String, String)>)>[
      if (custom.isNotEmpty) (strings.customStyles, custom),
      for (final kind in ['Light', 'Medium', 'Dark'])
        (strings.tableStyleSection(kind), [
          for (final (family, ids) in builtinTableStyleFamilies)
            if (family.startsWith(kind) || (kind == 'Light' && family.startsWith('Themed')))
              for (final (accent, id) in ids.indexed)
                if (id.isNotEmpty) (id, strings.tableStyleName(family, accent)),
        ]),
    ];
    return SizedBox(
      width: width,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final (title, styles) in sections) ...[
            Padding(
              padding: const EdgeInsets.fromLTRB(4, 8, 4, 4),
              child: Text(title, style: Theme.of(context).textTheme.labelMedium),
            ),
            Wrap(children: [for (final (id, name) in styles) _style(context, id, name, options, colors, theme)]),
          ],
        ],
      ),
    );
  }

  Widget _style(BuildContext context, String id, String name, Map<String, Object?> options, ColorContext colors, DeckTheme theme) {
    final scheme = Theme.of(context).colorScheme;
    return Tooltip(
      message: name,
      child: InkWell(
        onTap: () => onSelected(id),
        child: Container(
          margin: const EdgeInsets.all(2),
          padding: const EdgeInsets.all(2),
          decoration: BoxDecoration(border: Border.all(color: options['style'] == id ? scheme.primary : Colors.transparent, width: 2)),
          child: CustomPaint(size: const Size(_thumbWidth, 38), painter: _StylePainter(deck.tableStyle(id), options, colors, theme)),
        ),
      ),
    );
  }
}

class _StylePainter extends CustomPainter {
  _StylePainter(this.parts, this.options, this.colors, this.theme);

  final Map<String, Map<String, Object?>> parts;
  final Map<String, Object?> options;
  final ColorContext colors;
  final DeckTheme theme;

  @override
  void paint(Canvas canvas, Size size) => TableLayout.paintStyle(canvas, size, parts, options, colors: colors, theme: theme);

  @override
  bool shouldRepaint(_StylePainter old) => old.parts != parts || old.options != options;
}
