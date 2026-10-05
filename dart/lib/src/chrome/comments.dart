import 'package:trame/trame.dart';

/// A comment: a "comment" node, saying what its text says.
class DocComment {
  DocComment(this.node, this.text);

  final Node node;

  /// What it says, its paragraphs one a line.
  final String text;

  String get id => node.id;
  String get author => node.attributes['author'] as String? ?? '';
  String get initials => node.attributes['initials'] as String? ?? initialsOf(author);
  DateTime? get date => DateTime.tryParse(node.attributes['date'] as String? ?? '')?.toLocal();
  bool get done => node.attributes['done'] == true;
  String? get parent => node.attributes['parent'] as String?;
}

/// A comment and the answers to it.
class CommentThread {
  CommentThread(this.root);

  final DocComment root;
  final replies = <DocComment>[];

  String get id => root.id;
  bool get done => root.done;
  Iterable<DocComment> get all => [root, ...replies];

  /// Whether what the comment was about is still there.
  bool get anchored => true;
}

/// The initials a name gives: the first letter of its first two words.
String initialsOf(String name) =>
    name.split(RegExp(r'\s+')).where((w) => w.isNotEmpty).take(2).map((w) => String.fromCharCode(w.runes.first).toUpperCase()).join();
