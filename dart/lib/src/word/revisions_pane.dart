import 'package:flutter/material.dart';

import '../chrome/ai.dart';
import '../chrome/comments.dart';
import '../chrome/strings.dart';
import 'paragraph.dart';
import 'revisions.dart';

/// Tracked changes the pane settles together: one change, or the batch an
/// AI made at once, with the comments it wrote for it.
class RevisionGroup {
  const RevisionGroup(this.revisions, {this.batch = false, this.comments = const []});

  final List<WordRevision> revisions;

  /// Whether the group is a batch of the AI: all its changes of a date.
  final bool batch;
  final List<DocComment> comments;

  String get author => revisions.first.author;
  DateTime? get date => revisions.first.date;

  /// Whether a change by [author] dated [date] is one the group settles.
  bool owns(String author, String date) => author == this.author && DateTime.tryParse(date) == this.date;
}

/// [revisions] as the pane lists them: those of [ai] a group for each date,
/// with the comments of [comments] it wrote then, the others one by one.
/// Without an AI each change is a group of its own.
List<RevisionGroup> groupRevisions(List<WordRevision> revisions, {AiParticipant? ai, List<DocComment> comments = const []}) {
  final out = <RevisionGroup>[];
  final batches = <DateTime?, List<WordRevision>>{};
  for (final r in revisions) {
    if (ai == null || r.author != ai.name) {
      out.add(RevisionGroup([r]));
    } else if (batches[r.date] case final batch?) {
      batch.add(r);
    } else {
      final batch = batches[r.date] = [r];
      out.add(RevisionGroup(batch, batch: true, comments: [for (final c in comments) if (c.author == ai.name && c.parent == null && _same(c.date, r.date)) c]));
    }
  }
  return out;
}

bool _same(DateTime? a, DateTime? b) => a == null ? b == null : b != null && a.isAtSameMomentAs(b);

/// The tracked changes of a document beside its pages, the batches of the
/// AI grouped with their comments.
class RevisionsPane extends StatelessWidget {
  const RevisionsPane({
    super.key,
    required this.groups,
    required this.ai,
    required this.readOnly,
    required this.textOf,
    required this.onSelect,
    required this.onSettle,
    required this.onClose,
    this.strings = const LofficeStrings(),
  });

  final List<RevisionGroup> groups;
  final AiParticipant? ai;
  final bool readOnly;

  /// What a change inserts or deletes.
  final String Function(WordRevision revision) textOf;
  final ValueChanged<WordRevision> onSelect;
  final void Function(RevisionGroup group, bool accept) onSettle;
  final VoidCallback onClose;
  final LofficeStrings strings;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 4, 8),
            child: Row(
              children: [
                Icon(Icons.track_changes, size: 18, color: theme.colorScheme.primary),
                const SizedBox(width: 8),
                Expanded(child: Text(strings.revisionsPane, style: theme.textTheme.titleSmall)),
                if (groups.isNotEmpty) Text('${groups.length}', style: theme.textTheme.labelMedium),
                IconButton(tooltip: strings.close, iconSize: 18, onPressed: onClose, icon: const Icon(Icons.close)),
              ],
            ),
          ),
          const Divider(height: 1),
          Expanded(
            child: groups.isEmpty
                ? Center(child: Text(strings.noRevisions, style: theme.textTheme.titleSmall))
                : ListView(padding: const EdgeInsets.all(12), children: [for (final g in groups) _card(theme, g)]),
          ),
        ],
      ),
    );
  }

  Widget _card(ThemeData theme, RevisionGroup g) {
    final scheme = theme.colorScheme;
    final isAi = g.batch;
    final color = isAi ? aiColor : revisionColor(g.author);
    final title = isAi ? strings.batchOf(aiLabel(g.author), g.date) : strings.revision(g.revisions.first.deleted, g.author, g.date);
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Material(
        color: scheme.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: BorderSide(color: isAi ? aiColor : scheme.outlineVariant),
        ),
        clipBehavior: Clip.antiAlias,
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  if (isAi) ...[const Icon(Icons.auto_awesome, size: 16, color: aiColor), const SizedBox(width: 6)],
                  Expanded(child: Text(title, style: theme.textTheme.labelLarge?.copyWith(color: color))),
                ],
              ),
              for (final c in g.comments)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(c.text, style: theme.textTheme.bodyMedium?.copyWith(fontStyle: FontStyle.italic)),
                ),
              for (final r in g.revisions)
                InkWell(
                  onTap: () => onSelect(r),
                  child: Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      '${r.deleted ? '−' : '+'} ${textOf(r)}',
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.bodySmall?.copyWith(decoration: r.deleted ? TextDecoration.lineThrough : null),
                    ),
                  ),
                ),
              if (!readOnly)
                Align(
                  alignment: Alignment.centerRight,
                  child: Wrap(
                    children: [
                      TextButton(onPressed: () => onSettle(g, true), child: Text(isAi ? strings.acceptBatch : strings.accept)),
                      TextButton(onPressed: () => onSettle(g, false), child: Text(isAi ? strings.rejectBatch : strings.reject)),
                    ],
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
