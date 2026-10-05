import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:trame/trame.dart';

import '../chrome/strings.dart';
import 'deck.dart';
import 'slide_painter.dart';

/// A slide show: the slides full screen, one after the other, those hidden
/// left out; each comes on with its transition, and the show goes on by
/// itself after the time a slide gives.
class Slideshow extends StatefulWidget {
  const Slideshow({
    super.key,
    required this.deck,
    required this.painter,
    this.start = 0,
    this.strings = const LofficeStrings(),
    this.images,
    this.presenter = false,
  });

  final Deck deck;
  final SlidePainter painter;

  /// The slide to start from, in the deck's order.
  final int start;
  final LofficeStrings strings;

  /// Notifies when pictures arrive.
  final Listenable? images;

  /// Shows the presenter view: the slide, the next one, the notes and the
  /// time, as PowerPoint shows them on the speaker's screen.
  final bool presenter;

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

class _SlideshowState extends State<Slideshow> with SingleTickerProviderStateMixin {
  late final List<Node> _slides = [
    for (final s in widget.deck.slides) if (s.attributes['hidden'] != true) s,
  ];
  late int _at;
  int? _from;
  late final _progress = AnimationController(vsync: this, value: 1)..addStatusListener(_settled);
  Timer? _timer;
  final _focus = FocusNode();

  /// The seconds the presenter has spoken, counted while [_running].
  var _seconds = 0;
  var _running = true;
  Timer? _tick;

  @override
  void initState() {
    super.initState();
    final start = widget.deck.slides.elementAtOrNull(widget.start);
    _at = start == null ? 0 : _slides.indexWhere((s) => s.id == start.id);
    if (_at < 0) _at = 0;
    widget.images?.addListener(_repaint);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersive);
    _shown();
    if (widget.presenter) {
      _tick = Timer.periodic(const Duration(seconds: 1), (_) {
        if (_running) setState(() => _seconds++);
      });
    }
  }

  @override
  void dispose() {
    widget.images?.removeListener(_repaint);
    _timer?.cancel();
    _tick?.cancel();
    _progress.dispose();
    _focus.dispose();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    super.dispose();
  }

  void _repaint() => setState(() {});

  static Map<String, Object?> _transition(Node? slide) => slide?.attributes['transition'] as Map<String, Object?>? ?? const {};

  void _go(int to) {
    if (to < 0) return;
    if (to > _slides.length) {
      Navigator.of(context).pop();
      return;
    }
    _timer?.cancel();
    // a slide comes on with its transition going forward, at once going back
    final next = to == _at + 1 ? _transition(_slides.elementAtOrNull(to)) : const <String, Object?>{};
    final ms = next['dur'] is num ? (next['dur']! as num).toInt() : 0;
    final effect = next['effect'];
    setState(() {
      _from = _at;
      _at = to;
    });
    if (effect is String && effect.isNotEmpty && effect != 'cut' && ms > 0) {
      _progress.duration = Duration(milliseconds: ms);
      _progress.forward(from: 0);
    } else {
      _progress.value = 1;
      _shown();
    }
  }

  void _settled(AnimationStatus status) {
    if (status == AnimationStatus.completed) _shown();
  }

  /// Waits the time the slide shown gives before going on, if it gives one.
  void _shown() {
    if (_at >= _slides.length) return;
    final after = _transition(_slides[_at])['after'];
    if (after is num) _timer = Timer(Duration(milliseconds: after.toInt()), () => _go(_at + 1));
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
    final from = _from == null ? null : _slides.elementAtOrNull(_from!);
    final click = end || _transition(_slides[_at])['noClick'] != true;
    final show = GestureDetector(
      onTap: click ? () => _go(_at + 1) : null,
      onSecondaryTap: () => _go(_at - 1),
      onHorizontalDragEnd: (d) => _go(_at + ((d.primaryVelocity ?? 0) < 0 ? 1 : -1)),
      child: ColoredBox(
        color: Colors.black,
        child: end
            ? Center(child: Text(widget.strings.endOfShow, style: const TextStyle(color: Colors.white70, fontSize: 18)))
            : SizedBox.expand(
                child: CustomPaint(
                  painter: _ShowPainter(widget.painter, widget.deck, _slides[_at], from: from, transition: _transition(_slides[_at]), progress: _progress),
                ),
              ),
      ),
    );
    return Focus(autofocus: true, focusNode: _focus, onKeyEvent: _key, child: widget.presenter ? _presenterView(show) : show);
  }

  /// The slide shown with, beside it, the next one and the notes; the time
  /// above, the commands below.
  Widget _presenterView(Widget show) {
    final s = widget.strings;
    final next = _slides.elementAtOrNull(_at + 1);
    final shown = _slides.elementAtOrNull(_at);
    final notes = shown == null ? null : widget.deck.tree.children(shown.id).where((n) => n.type == 'notes').firstOrNull?.text?.text.trim();
    const light = TextStyle(color: Colors.white70);
    String two(int n) => n.toString().padLeft(2, '0');
    final time = '${_seconds >= 3600 ? '${_seconds ~/ 3600}:' : ''}${two(_seconds ~/ 60 % 60)}:${two(_seconds % 60)}';
    return Material(
      color: const Color(0xFF202020),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(children: [
          Row(children: [
            Text(time, style: light.copyWith(fontSize: 22, fontFeatures: const [FontFeature.tabularFigures()])),
            IconButton(
              tooltip: _running ? s.pauseTimer : s.resumeTimer,
              color: Colors.white70,
              icon: Icon(_running ? Icons.pause : Icons.play_arrow),
              onPressed: () => setState(() => _running = !_running),
            ),
            IconButton(tooltip: s.restartTimer, color: Colors.white70, icon: const Icon(Icons.replay), onPressed: () => setState(() => _seconds = 0)),
            const Spacer(),
            TextButton(
              style: TextButton.styleFrom(foregroundColor: Colors.white),
              onPressed: () => Navigator.of(context).pop(),
              child: Text(s.endSlideShow),
            ),
          ]),
          const SizedBox(height: 12),
          Expanded(
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Expanded(flex: 3, child: show),
              const SizedBox(width: 16),
              Expanded(
                flex: 2,
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(s.nextSlide, style: light),
                  const SizedBox(height: 6),
                  AspectRatio(
                    aspectRatio: widget.deck.size.width / widget.deck.size.height,
                    child: ColoredBox(
                      color: Colors.black,
                      child: next == null
                          ? Center(child: Text(s.endOfShow, textAlign: TextAlign.center, style: light))
                          : CustomPaint(painter: _ShowPainter(widget.painter, widget.deck, next, transition: const {}, progress: kAlwaysCompleteAnimation)),
                    ),
                  ),
                  const SizedBox(height: 16),
                  Expanded(
                    child: SingleChildScrollView(
                      child: Text(notes == null || notes.isEmpty ? s.noNotes : notes, style: light.copyWith(fontSize: 18, color: Colors.white)),
                    ),
                  ),
                ]),
              ),
            ]),
          ),
          const SizedBox(height: 8),
          Row(mainAxisAlignment: MainAxisAlignment.center, children: [
            IconButton(tooltip: s.previousSlide, color: Colors.white70, icon: const Icon(Icons.chevron_left), onPressed: () => _go(_at - 1)),
            Text(s.slideOf(math.min(_at + 1, _slides.length), _slides.length), style: light),
            IconButton(tooltip: s.nextSlide, color: Colors.white70, icon: const Icon(Icons.chevron_right), onPressed: () => _go(_at + 1)),
          ]),
        ]),
      ),
    );
  }
}

/// The transition of a slide played once, as the Transitions tab
/// previews it over the slide: [from] gives way to [slide], from black when
/// there is none.
class TransitionPreview extends StatefulWidget {
  const TransitionPreview({super.key, required this.deck, required this.painter, required this.slide, this.from, required this.onDone});

  final Deck deck;
  final SlidePainter painter;
  final Node slide;
  final Node? from;
  final VoidCallback onDone;

  @override
  State<TransitionPreview> createState() => _TransitionPreviewState();
}

class _TransitionPreviewState extends State<TransitionPreview> with SingleTickerProviderStateMixin {
  late final Map<String, Object?> _transition = _SlideshowState._transition(widget.slide);
  late final _progress = AnimationController(
    vsync: this,
    duration: Duration(milliseconds: _transition['dur'] is num ? math.max(1, (_transition['dur']! as num).toInt()) : 1),
  );

  @override
  void initState() {
    super.initState();
    _progress.forward().whenComplete(() {
      if (mounted) widget.onDone();
    });
  }

  @override
  void dispose() {
    _progress.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final size = widget.deck.size;
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Center(
        child: AspectRatio(
          aspectRatio: size.width / size.height,
          child: ClipRect(
            child: ColoredBox(
              color: Colors.black,
              child: CustomPaint(
                size: Size.infinite,
                painter: _ShowPainter(widget.painter, widget.deck, widget.slide, from: widget.from, transition: _transition, progress: _progress),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// A slide shown, coming on over the one before with its transition.
class _ShowPainter extends CustomPainter {
  _ShowPainter(this.painter, this.deck, this.slide, {this.from, required this.transition, required this.progress}) : super(repaint: progress);

  final SlidePainter painter;
  final Deck deck;
  final Node slide;
  final Node? from;
  final Map<String, Object?> transition;
  final Animation<double> progress;

  @override
  void paint(Canvas canvas, Size size) {
    final t = progress.value, from = this.from;
    if (t >= 1) return _draw(canvas, size, slide);
    void back({Offset shift = Offset.zero, double opacity = 1}) {
      if (from != null) _draw(canvas, size, from, shift: shift, opacity: opacity);
    }

    final dir = transition['dir'];
    Offset away(Object? dir) {
      final d = '${dir ?? 'l'}';
      final x = d.contains('l') ? -1.0 : (d.contains('r') ? 1.0 : 0.0);
      final y = d.contains('u') ? -1.0 : (d.contains('d') ? 1.0 : 0.0);
      return Offset(x * size.width, y * size.height);
    }

    switch (transition['effect']) {
      case 'push':
        back(shift: away(dir) * t);
        _draw(canvas, size, slide, shift: away(dir) * (t - 1));
      case 'cover':
        back();
        _draw(canvas, size, slide, shift: away(dir) * (t - 1));
      case 'pull':
        _draw(canvas, size, slide);
        back(shift: away(dir) * t);
      case 'wipe':
        final (w, h) = (size.width, size.height);
        back();
        _draw(canvas, size, slide, clip: Path()..addRect(switch (dir) {
          'r' => Rect.fromLTRB(0, 0, w * t, h),
          'u' => Rect.fromLTRB(0, h * (1 - t), w, h),
          'd' => Rect.fromLTRB(0, 0, w, h * t),
          _ => Rect.fromLTRB(w * (1 - t), 0, w, h),
        }));
      case 'split':
        final horz = transition['orient'] != 'vert';
        final c = size.center(Offset.zero);
        final band = horz ? Size(size.width, size.height * t) : Size(size.width * t, size.height);
        final edges = horz ? Size(size.width, size.height * (1 - t)) : Size(size.width * (1 - t), size.height);
        final clip = dir == 'in'
            ? (Path()
                ..fillType = PathFillType.evenOdd
                ..addRect(Offset.zero & size)
                ..addRect(Rect.fromCenter(center: c, width: edges.width, height: edges.height)))
            : (Path()..addRect(Rect.fromCenter(center: c, width: band.width, height: band.height)));
        back();
        _draw(canvas, size, slide, clip: clip);
      case 'zoom':
        back();
        _draw(canvas, size, slide, opacity: t, scale: dir == 'out' ? 1.6 - 0.6 * t : 0.4 + 0.6 * t);
      case 'fade' when transition['thruBlk'] == true:
        if (t < 0.5) {
          back(opacity: 1 - 2 * t);
        } else {
          _draw(canvas, size, slide, opacity: 2 * t - 1);
        }
      default:
        back();
        _draw(canvas, size, slide, opacity: t);
    }
  }

  /// Draws a slide fitted in [size], nothing outside it.
  void _draw(Canvas canvas, Size size, Node slide, {Offset shift = Offset.zero, double opacity = 1, double scale = 1, Path? clip}) {
    final s = deck.size;
    final fit = math.min(size.width / s.width, size.height / s.height) * scale;
    canvas.save();
    if (clip != null) canvas.clipPath(clip);
    if (opacity < 1) canvas.saveLayer(Offset.zero & size, Paint()..color = Color.fromRGBO(0, 0, 0, opacity.clamp(0, 1)));
    canvas.translate(size.width / 2 + shift.dx, size.height / 2 + shift.dy);
    canvas.scale(fit);
    canvas.translate(-s.width / 2, -s.height / 2);
    canvas.clipRect(Offset.zero & s);
    painter.paint(canvas, slide);
    if (opacity < 1) canvas.restore();
    canvas.restore();
  }

  @override
  bool shouldRepaint(_ShowPainter old) => true;
}
