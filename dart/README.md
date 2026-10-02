# loffice

The Flutter client of [L'Office](https://github.com/citadellefr/loffice): Office
documents edited together in real time, offline edits included, against the
Go server.

```dart
import 'package:loffice/loffice.dart';

final session = DocSession(
  webSocketConnector((clientId) async => Uri.parse('wss://example.com/doc?client=$clientId')),
)..start();

PlainTextEditor(session: session, node: 'body');
// or, for a presentation, with its pictures fetched from the host
PresentationEditor(session: session, media: (name) => fetchPicture(name));
```

A session shows local edits at once and rebases them over those of others;
`undo` and `redo` revert this person's edits only. See the
[repository README](https://github.com/citadellefr/loffice#readme) for the
server side and the protocol.
