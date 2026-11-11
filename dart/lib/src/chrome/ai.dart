import 'dart:async';

import 'package:flutter/material.dart';
import 'package:trame/trame.dart';

/// The AI participant of a room, as the server says it: the id it joined
/// with, which tells its peer from a person's, and the name that signs its
/// revisions and comments. Without one the editors show nothing of an AI.
class AiParticipant {
  const AiParticipant({required this.id, required this.name});

  final String id;
  final String name;

  bool isPeer(DocPeer peer) => peer.id == id;
}

/// What marks the AI's cursor, presence and edits from a person's.
const aiColor = Color(0xFF7B3FE4);

/// The name of the AI as its cursor labels it.
String aiLabel(String name) => '✦ $name';

/// The color of a peer's cursor: the AI's own, or [person]'s.
Color peerColor(AiParticipant? ai, DocPeer peer, Color person) => ai != null && ai.isPeer(peer) ? aiColor : person;

/// The label of a peer's cursor.
String peerLabel(AiParticipant? ai, DocPeer peer) => ai != null && ai.isPeer(peer) ? aiLabel(ai.name) : peer.name;

/// What the AI changed lately, which an editor draws a few seconds: a node,
/// or the cells of a sheet, from top, left to bottom, right.
class AiMark {
  AiMark(this.node, this.cells, this.at);

  final String node;
  final List<int>? cells;
  final int at;

  var _age = 0;

  String get key => '$node$cells@$at';
}

/// The edits the AI says it made lately, as they reach the room, each kept
/// [life] and fading at the end: the editors draw them over what changed.
class AiMarks extends ChangeNotifier {
  AiMarks(this.session, this.ai, {this.life = const Duration(seconds: 4)}) {
    _previous = session.onShared;
    session.onShared = _shared;
  }

  static const _tick = Duration(milliseconds: 100);
  static const _remembered = 256;

  final DocSession session;
  final AiParticipant ai;
  final Duration life;

  void Function(DocPeer, Map<String, Object?>)? _previous;
  final _marks = <AiMark>[];
  final _seen = <String>{};
  Timer? _timer;

  int get _ticks => life.inMilliseconds ~/ _tick.inMilliseconds;

  List<AiMark> get marks => List.unmodifiable(_marks);

  /// How much of [mark] still shows: whole for most of its life.
  double opacity(AiMark mark) => ((_ticks - mark._age) / (_ticks * 0.4)).clamp(0.0, 1.0);

  void _shared(DocPeer peer, Map<String, Object?> data) {
    _previous?.call(peer, data);
    final recent = data['recent'];
    if (!ai.isPeer(peer) || recent is! List<Object?>) return;
    var fresh = false;
    for (final raw in recent) {
      if (raw is! Map<String, Object?> || raw['n'] is! String || raw['at'] is! int) continue;
      final c = raw['c'];
      final cells = c is List<Object?> && c.length == 4 && c.every((v) => v is int) ? c.cast<int>() : null;
      final mark = AiMark(raw['n']! as String, cells, raw['at']! as int);
      if (_seen.add(mark.key)) {
        if (_seen.length > _remembered) _seen.remove(_seen.first);
        _marks.add(mark);
        fresh = true;
      }
    }
    if (!fresh) return;
    _timer ??= Timer.periodic(_tick, (_) => _step());
    notifyListeners();
  }

  void _step() {
    for (final m in _marks) {
      m._age++;
    }
    _marks.removeWhere((m) => m._age >= _ticks);
    if (_marks.isEmpty) {
      _timer?.cancel();
      _timer = null;
    }
    notifyListeners();
  }

  @override
  void dispose() {
    _timer?.cancel();
    if (session.onShared == _shared) session.onShared = _previous;
    super.dispose();
  }
}
