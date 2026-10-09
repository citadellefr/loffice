import 'dart:async';

import 'package:flutter/material.dart';

/// What the host shows over a link: a card of what it leads to, or null for
/// nothing.
typedef LinkCard = Widget? Function(Uri uri);

/// The card of the host over a link of an editor. Under a pointer it shows
/// once the pointer rests on the link, is only read, and leaves with it;
/// under a finger it is what gets tapped, and anything else puts it away.
class LinkCards {
  LinkCards(this._context, this._card);

  final BuildContext Function() _context;
  final LinkCard? Function() _card;

  Uri? _pointed;
  Timer? _resting;
  OverlayEntry? _entry;

  /// Tells which link the pointer is over at [global], null for none.
  void point(Uri? uri, Offset global) {
    if (uri == _pointed) return;
    hide();
    _pointed = uri;
    if (uri == null) return;
    _resting = Timer(const Duration(milliseconds: 400), () => _show(uri, global, touch: false));
  }

  /// Shows the card of a link a finger tapped at [global]; false when the
  /// host has none, and the tap is the editor's.
  bool tap(Uri uri, Offset global) => _show(uri, global, touch: true);

  bool _show(Uri uri, Offset at, {required bool touch}) {
    final context = _context();
    final card = context.mounted ? _card()?.call(uri) : null;
    if (card == null) return false;
    hide();
    _pointed = uri;
    final placed = CustomSingleChildLayout(delegate: _CardPlace(at), child: card);
    final entry = _entry = OverlayEntry(
      builder: (context) => touch
          ? Listener(
              behavior: HitTestBehavior.translucent,
              onPointerUp: (_) => WidgetsBinding.instance.addPostFrameCallback((_) => hide()),
              child: placed,
            )
          : IgnorePointer(child: placed),
    );
    Overlay.of(context).insert(entry);
    return true;
  }

  void hide() {
    _resting?.cancel();
    _entry?.remove();
    _entry?.dispose();
    _entry = null;
    _pointed = null;
  }
}

/// Places the card under where its link was pointed at, or over it when
/// there is no room under, and inside the screen.
class _CardPlace extends SingleChildLayoutDelegate {
  const _CardPlace(this.target);

  final Offset target;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) => constraints.loosen();

  @override
  Offset getPositionForChild(Size size, Size childSize) =>
      positionDependentBox(size: size, childSize: childSize, target: target, preferBelow: true, verticalOffset: 14);

  @override
  bool shouldRelayout(_CardPlace old) => old.target != target;
}
