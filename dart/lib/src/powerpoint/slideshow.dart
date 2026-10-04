import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import 'deck.dart';
import 'slide_painter.dart';

/// A slide show: the slides full screen, one after the other, those hidden
/// left out; each fades into the next.
class Slideshow extends StatefulWidget {
  const Slideshow({
    super.key,
    required this.deck,
    required this.painter,
    this.start = 0,
    this.strings = const LofficeStrings(),
    this.images,
  });

  final Deck deck;
  final SlidePainter painter;

  /// The slide to start from, in the deck's order.
  final int start;
  final LofficeStrings strings;

  /// Notifies when pictures arrive.
  final Listenable? images;

  /// Shows a slide show over everything, until it ends.
  static Future<void> show(BuildContext context, Slideshow show) => Navigator.of(context).push(
    PageRouteBuilder<void>(
      opaque: true,
      fullscreenDialog: true,
      pageBuilder: (_, _, _) => show,
      transitionsBuilder: (_, animation, _, child) => FadeTransition(opacity: animation, child: child),
    ),
  );

  @override
  State<Slideshow> createState() => _SlideshowState();
}

class _SlideshowState extends State<Slideshow> {
  late final List<Node> _slides = [
    for (final s in widget.deck.slides) if (s.attributes['hidden'] != true) s,
  ];
  late int _at;
  final _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    final start = widget.deck.slides.elementAtOrNull(widget.start);
    _at = start == null ? 0 : _slides.indexWhere((s) => s.id == start.id);
    if (_at < 0) _at = 0;
    widget.images?.addListener(_repaint);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersive);
  }

  @override
  void dispose() {
    widget.images?.removeListener(_repaint);
    _focus.dispose();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    super.dispose();
  }

  void _repaint() => setState(() {});

  void _go(int to) {
    if (to < 0) return;
    if (to > _slides.length) {
      Navigator.of(context).pop();
      return;
    }
    setState(() => _at = to);
  }

  static final _forward = {
    LogicalKeyboardKey.arrowRight, LogicalKeyboardKey.arrowDown, LogicalKeyboardKey.space, //
    LogicalKeyboardKey.enter, LogicalKeyboardKey.pageDown, LogicalKeyboardKey.keyN,
  };
  static final _backward = {
    LogicalKeyboardKey.arrowLeft, LogicalKeyboardKey.arrowUp, LogicalKeyboardKey.backspace, //
    LogicalKeyboardKey.pageUp, LogicalKeyboardKey.keyP,
  };

  KeyEventResult _key(FocusNode _, KeyEvent e) {
    if (e is KeyUpEvent) return KeyEventResult.ignored;
    final key = e.logicalKey;
    if (key == LogicalKeyboardKey.escape) {
      Navigator.of(context).pop();
    } else if (_forward.contains(key)) {
      _go(_at + 1);
    } else if (_backward.contains(key)) {
      _go(_at - 1);
    } else if (key == LogicalKeyboardKey.home) {
      _go(0);
    } else if (key == LogicalKeyboardKey.end) {
      _go(_slides.length - 1);
    } else {
      return KeyEventResult.ignored;
    }
    return KeyEventResult.handled;
  }

  @override
  Widget build(BuildContext context) {
    final end = _at >= _slides.length;
    return Focus(
      autofocus: true,
      focusNode: _focus,
      onKeyEvent: _key,
      child: GestureDetector(
        onTap: () => _go(_at + 1),
        onSecondaryTap: () => _go(_at - 1),
        onHorizontalDragEnd: (d) => _go(_at + ((d.primaryVelocity ?? 0) < 0 ? 1 : -1)),
        child: ColoredBox(
          color: Colors.black,
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 350),
            child: end
                ? Center(
                    key: const ValueKey('end'),
                    child: Text(widget.strings.endOfShow, style: const TextStyle(color: Colors.white70, fontSize: 18)),
                  )
                : SizedBox.expand(
                    key: ValueKey(_slides[_at].id),
                    child: CustomPaint(painter: _ShowPainter(widget.painter, widget.deck, _slides[_at])),
                  ),
          ),
        ),
      ),
    );
  }
}

class _ShowPainter extends CustomPainter {
  _ShowPainter(this.painter, this.deck, this.slide);

  final SlidePainter painter;
  final Deck deck;
  final Node slide;

  @override
  void paint(Canvas canvas, Size size) {
    final s = deck.size;
    final scale = (size.width / s.width) < (size.height / s.height) ? size.width / s.width : size.height / s.height;
    canvas.translate((size.width - s.width * scale) / 2, (size.height - s.height * scale) / 2);
    canvas.scale(scale);
    painter.paint(canvas, slide);
  }

  @override
  bool shouldRepaint(_ShowPainter old) => true;
}
