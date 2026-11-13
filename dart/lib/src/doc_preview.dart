import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

import 'excel/page_painter.dart';
import 'excel/print.dart';
import 'excel/workbook.dart';
import 'fonts.dart';
import 'powerpoint/deck.dart';
import 'powerpoint/slide_painter.dart';
import 'word/blocks.dart' show ParaCache;
import 'word/document.dart';
import 'word/layout.dart';
import 'word/page_painter.dart';
import 'word/paragraph.dart';

/// The kind of document a tree holds.
enum DocKind { word, deck, book }

/// A still picture of a document: its first page, slide or sheet, drawn as
/// wide as the room it is given, with no selection and no way to edit. The
/// tree is only read.
class DocPreview extends StatefulWidget {
  const DocPreview({required this.tree, required this.kind, this.media, super.key});

  final Tree tree;
  final DocKind kind;

  /// Fetches the pictures of the document; one that is missing or not yet
  /// loaded leaves its place blank.
  final MediaFetcher? media;

  @override
  State<DocPreview> createState() => _DocPreviewState();
}

class _DocPreviewState extends State<DocPreview> {
  MediaCache? _media;
  PrintedPage? _page;

  @override
  void initState() {
    super.initState();
    _media = _cache();
    if (!OfficeFonts.settled) OfficeFonts.load().whenComplete(_load);
  }

  @override
  void didUpdateWidget(DocPreview old) {
    super.didUpdateWidget(old);
    if (old.media != widget.media) {
      _media?.dispose();
      _media = _cache();
    }
    if (old.media != widget.media || !identical(old.tree, widget.tree) || old.kind != widget.kind) _page = null;
  }

  @override
  void dispose() {
    _media?.dispose();
    super.dispose();
  }

  MediaCache? _cache() => widget.media == null ? null : MediaCache(widget.media!);

  void _load() {
    if (mounted) setState(() {});
  }

  PrintedPage _lay() {
    final tree = widget.tree;
    final media = _media;
    ui.Image? images(String m) => media?[m];
    switch (widget.kind) {
      case DocKind.word:
        final doc = WordDocument(tree);
        final context = WordContext(doc);
        final page = WordLayout(doc, context, ParaCache()).pages.firstOrNull;
        if (page == null) return const PrintedPage(Size(612, 792), _blank);
        final painter = PagePainter(images: images, context: context);
        return PrintedPage(page.size, (canvas) => painter.paint(canvas, page));
      case DocKind.deck:
        final deck = Deck(tree);
        final slide = deck.slides.firstOrNull;
        if (slide == null) return PrintedPage(deck.size, _blank);
        final painter = SlidePainter(deck, images: images);
        return PrintedPage(deck.size, (canvas) => painter.paint(canvas, slide));
      case DocKind.book:
        return workbookPages(Workbook(tree)).firstOrNull ?? PrintedPage(const PrintSetup({}).page, _blank);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!OfficeFonts.settled) return const SizedBox.shrink();
    final page = _page ??= _lay();
    return FittedBox(
      child: SizedBox.fromSize(
        size: page.size,
        child: CustomPaint(painter: _PagePaint(page.paint, _media)),
      ),
    );
  }
}

void _blank(Canvas canvas) => canvas.drawPaint(Paint()..color = const Color(0xFFFFFFFF));

class _PagePaint extends CustomPainter {
  _PagePaint(this.draw, MediaCache? media) : super(repaint: media);

  final void Function(Canvas canvas) draw;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.clipRect(Offset.zero & size);
    draw(canvas);
  }

  @override
  bool shouldRepaint(_PagePaint old) => !identical(old.draw, draw);
}
