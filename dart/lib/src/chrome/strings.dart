import '../pdf/canvas.dart' show Ignored;

/// The words of the editors, as the French version of Office has them.
class LofficeStrings {
  const LofficeStrings();

  // ribbon tabs
  String get file => 'Fichier';
  String get home => 'Accueil';
  String get insert => 'Insertion';
  String get design => 'Création';
  String get transitions => 'Transitions';
  String get slideShow => 'Diaporama';
  String get review => 'Révision';
  String get view => 'Affichage';

  // groups and commands
  String get clipboard => 'Presse-papiers';
  String get paste => 'Coller';
  String get cut => 'Couper';
  String get copy => 'Copier';
  String get slides => 'Diapositives';
  String get newSlide => 'Nouvelle diapositive';
  String get duplicateSlide => 'Dupliquer la diapositive';
  String get deleteSlide => 'Supprimer la diapositive';
  String get hideSlide => 'Masquer la diapositive';
  String get layout => 'Disposition';
  String get font => 'Police';
  String get fontSize => 'Taille de police';
  String get growFont => 'Augmenter la taille de police';
  String get shrinkFont => 'Réduire la taille de police';
  String get bold => 'Gras';
  String get italic => 'Italique';
  String get underline => 'Souligné';
  String get strikethrough => 'Barré';
  String get fontColor => 'Couleur de police';
  String get highlight => 'Couleur de surbrillance du texte';
  String get paragraph => 'Paragraphe';
  String get bullets => 'Puces';
  String get numbering => 'Numérotation';
  String get decreaseLevel => 'Diminuer le niveau de liste';
  String get increaseLevel => 'Augmenter le niveau de liste';
  String get alignLeft => 'Aligner à gauche';
  String get center => 'Centrer';
  String get alignRight => 'Aligner à droite';
  String get justify => 'Justifier';
  String get drawing => 'Dessin';
  String get shapes => 'Formes';
  String get textBox => 'Zone de texte';
  String get shapeFill => 'Remplissage de forme';
  String get shapeOutline => 'Contour de forme';
  String get noFill => 'Aucun remplissage';
  String get noOutline => 'Sans contour';
  String get arrange => 'Organiser';
  String get bringForward => 'Avancer';
  String get sendBackward => 'Reculer';
  String get bringToFront => 'Mettre au premier plan';
  String get sendToBack => 'Mettre à l’arrière-plan';
  String get startSlideShow => 'Lancer le diaporama';
  String get fromBeginning => 'À partir du début';
  String get fromCurrentSlide => 'À partir de la diapositive actuelle';
  String get presentationViews => 'Affichages des présentations';
  String get normal => 'Normal';
  String get slideSorter => 'Trieuse de diapositives';
  String get notes => 'Notes';
  String get show => 'Afficher';
  String get undo => 'Annuler';
  String get redo => 'Rétablir';
  String get delete => 'Supprimer';
  String get themeColors => 'Couleurs du thème';
  String get standardColors => 'Couleurs standard';
  String get automatic => 'Automatique';

  // Excel
  String get pageLayout => 'Mise en page';
  String get formulas => 'Formules';
  String get data => 'Données';
  String get alignment => 'Alignement';
  String get number => 'Nombre';
  String get cells => 'Cellules';
  String get editing => 'Édition';
  String get alignTop => 'Aligner en haut';
  String get alignMiddle => 'Aligner au centre';
  String get alignBottom => 'Aligner en bas';
  String get wrapText => 'Renvoyer à la ligne automatiquement';
  String get mergeAndCenter => 'Fusionner et centrer';
  String get mergeAcross => 'Fusionner sur les lignes';
  String get unmerge => 'Annuler Fusionner cellules';
  String get links => 'Liens';
  String get insertLink => 'Lien';
  String get linkAddress => 'Adresse';
  String get linkPlace => 'Emplacement dans ce classeur';
  String get linkTip => 'Info-bulle';
  String get removeLink => 'Supprimer le lien';
  String get numberFormat => 'Format de nombre';
  String get increaseDecimal => 'Ajouter une décimale';
  String get decreaseDecimal => 'Réduire les décimales';
  String get percentStyle => 'Style de pourcentage';
  String get commaStyle => 'Séparateur de milliers';
  String get accountingFormat => 'Format Nombre Comptabilité';
  String get fillColor => 'Couleur de remplissage';
  String get borders => 'Bordures';
  String get bottomBorder => 'Bordure inférieure';
  String get topBorder => 'Bordure supérieure';
  String get leftBorder => 'Bordure gauche';
  String get rightBorder => 'Bordure droite';
  String get allBorders => 'Toutes les bordures';
  String get outsideBorders => 'Bordures extérieures';
  String get thickBottomBorder => 'Bordure épaisse en bas';
  String get noBorder => 'Aucune bordure';
  String get insertCells => 'Insérer';
  String get deleteCells => 'Supprimer';
  String get insertRows => 'Insérer des lignes dans la feuille';
  String get insertColumns => 'Insérer des colonnes dans la feuille';
  String get deleteRows => 'Supprimer des lignes dans la feuille';
  String get deleteColumns => 'Supprimer des colonnes dans la feuille';
  String get insertSheet => 'Insérer une feuille';
  String get deleteSheet => 'Supprimer la feuille';
  String get renameSheet => 'Renommer';
  String get tabColor => 'Couleur d’onglet';
  String get hideSheet => 'Masquer';
  String get unhideSheet => 'Afficher';
  String get autoSum => 'Somme automatique';
  String get sortAscending => 'Trier de A à Z';
  String get sortDescending => 'Trier de Z à A';
  String get clear => 'Effacer';
  String get clearContents => 'Effacer le contenu';
  String get clearFormats => 'Effacer les formats';
  String get clearAll => 'Effacer tout';
  String get freezePanes => 'Figer les volets';
  String get unfreezePanes => 'Libérer les volets';
  String get freezeTopRow => 'Figer la ligne supérieure';
  String get freezeFirstColumn => 'Figer la première colonne';
  String get gridlines => 'Quadrillage';
  String get zoom => 'Zoom';
  String get printArea => 'Zone d’impression';
  String get setPrintArea => 'Définir';
  String get clearPrintArea => 'Annuler';
  String get insertPageBreak => 'Insérer un saut de page';
  String get removePageBreak => 'Supprimer le saut de page';
  String get resetPageBreaks => 'Rétablir tous les sauts de page';
  String get scaleToFit => 'Mise à l’échelle';
  String get scale => 'Échelle';
  String fitPages(int n) => n == 1 ? '1 page' : '$n pages';
  String get sheetOptions => 'Options de la feuille';
  String get printGridlines => 'Imprimer le quadrillage';
  String get printHeadings => 'Imprimer les en-têtes';
  List<(String, int)> get sheetPapers => const [('A4', 9), ('A3', 8), ('A5', 11), ('Lettre US', 1), ('Légal US', 5)];
  String get window => 'Fenêtre';
  String get sortAndFilter => 'Trier et filtrer';
  String get filter => 'Filtrer';
  String get createTable => 'Créer un tableau';
  String get tableRange => 'Où sont les données de votre tableau ?';
  String get tableHeaders => 'Mon tableau comporte des en-têtes';
  String get tableRangeInvalid => 'Cette plage ne peut pas devenir un tableau.';
  String get tableOverlap => 'Cette plage chevauche un autre tableau.';
  String get tableMerged => 'Cette plage contient des cellules fusionnées.';
  String get convertToRange => 'Convertir en plage';
  String get clearFilters => 'Effacer';
  String get reapply => 'Réappliquer';
  String clearFilterFrom(String column) => 'Effacer le filtre de « $column »';
  String get selectAll => '(Sélectionner tout)';
  String get blanks => '(Vides)';
  String get search => 'Rechercher';
  // conditional formats
  String get conditionalFormatting => 'Mise en forme conditionnelle';
  String get highlightRules => 'Règles de mise en surbrillance des cellules';
  String get topBottomRules => 'Règles des valeurs plus/moins élevées';
  String get dataBars => 'Barres de données';
  String get colorScales => 'Nuances de couleurs';
  String get iconSets => 'Jeux d’icônes';
  String get clearRules => 'Effacer les règles';
  String get clearRulesSelection => 'Effacer les règles des cellules sélectionnées';
  String get clearRulesSheet => 'Effacer les règles de la feuille entière';
  String get manageRules => 'Gérer les règles…';
  List<(String, String)> get highlightKinds => const [
    ('greaterThan', 'Supérieur à…'),
    ('lessThan', 'Inférieur à…'),
    ('between', 'Entre…'),
    ('equal', 'Égal à…'),
    ('contains', 'Texte qui contient…'),
    ('duplicate', 'Valeurs en double…'),
  ];
  List<(String, String)> get topBottomKinds => const [
    ('top', '10 premiers éléments…'),
    ('bottom', '10 derniers éléments…'),
    ('topPercent', '10 % premiers…'),
    ('bottomPercent', '10 % derniers…'),
    ('above', 'Au-dessus de la moyenne…'),
    ('below', 'En dessous de la moyenne…'),
  ];
  String highlightPrompt(String kind) => switch (kind) {
    'greaterThan' => 'Mettre en forme les cellules qui sont supérieures à :',
    'lessThan' => 'Mettre en forme les cellules qui sont inférieures à :',
    'between' => 'Mettre en forme les cellules qui sont comprises entre :',
    'equal' => 'Mettre en forme les cellules qui sont égales à :',
    'contains' => 'Mettre en forme les cellules qui contiennent le texte :',
    'duplicate' => 'Mettre en forme les cellules qui contiennent :',
    'top' || 'topPercent' => 'Mettre en forme les cellules qui se classent parmi les premières :',
    'bottom' || 'bottomPercent' => 'Mettre en forme les cellules qui se classent parmi les dernières :',
    'above' => 'Mettre en forme les cellules qui sont au-dessus de la moyenne',
    _ => 'Mettre en forme les cellules qui sont en dessous de la moyenne',
  };
  String get and => 'et';
  String get formatWith => 'avec :';
  String get rankCount => 'Nombre';
  String get duplicateValue => 'Valeurs en double';
  String get uniqueValue => 'Valeurs uniques';
  List<(String, String)> get highlightFormats => const [
    ('red', 'Remplissage rouge clair et texte rouge foncé'),
    ('yellow', 'Remplissage jaune et texte jaune foncé'),
    ('green', 'Remplissage vert et texte vert foncé'),
    ('redText', 'Texte rouge foncé'),
    ('redBorder', 'Bordure rouge'),
  ];
  List<(String, String)> get dataBarColors => const [('blue', 'Bleu'), ('green', 'Vert'), ('red', 'Rouge'), ('orange', 'Orange'), ('purple', 'Violet')];
  List<(String, String)> get colorScaleSets => const [
    ('gyr', 'Vert - Jaune - Rouge'),
    ('ryg', 'Rouge - Jaune - Vert'),
    ('gwr', 'Vert - Blanc - Rouge'),
    ('rwg', 'Rouge - Blanc - Vert'),
    ('bwr', 'Bleu - Blanc - Rouge'),
    ('rwb', 'Rouge - Blanc - Bleu'),
    ('wg', 'Blanc - Vert'),
    ('wr', 'Blanc - Rouge'),
  ];
  List<(String, String)> get iconSetNames => const [
    ('3Arrows', '3 flèches (colorées)'),
    ('3ArrowsGray', '3 flèches (grises)'),
    ('3Flags', '3 drapeaux'),
    ('3TrafficLights1', '3 feux tricolores (sans bordure)'),
    ('3TrafficLights2', '3 feux tricolores (avec bordure)'),
    ('3Signs', '3 signes'),
    ('3Symbols', '3 symboles (encerclés)'),
    ('3Symbols2', '3 symboles (sans cercle)'),
    ('3Stars', '3 étoiles'),
    ('3Triangles', '3 triangles'),
    ('4Arrows', '4 flèches (colorées)'),
    ('4ArrowsGray', '4 flèches (grises)'),
    ('4RedToBlack', '4 cercles du rouge au noir'),
    ('4Rating', '4 notes'),
    ('4TrafficLights', '4 feux tricolores'),
    ('5Arrows', '5 flèches (colorées)'),
    ('5ArrowsGray', '5 flèches (grises)'),
    ('5Rating', '5 notes'),
    ('5Quarters', '5 quarts'),
    ('5Boxes', '5 cases'),
  ];
  String get rulesManager => 'Gestionnaire de règles de mise en forme conditionnelle';
  String get noRules => 'Aucune règle dans cette feuille.';
  String get appliesTo => 'S’applique à';
  String get newRule => 'Nouvelle règle…';
  String get newFormulaRule => 'Nouvelle règle : utiliser une formule';
  String get ruleFormula => 'Formule (relative à la première cellule)';
  String get deleteRule => 'Supprimer la règle';
  String get moveUp => 'Monter';
  String get moveDown => 'Descendre';
  String ruleDescription(String kind, {String formula = '', int rank = 10, String icons = ''}) => switch (kind) {
    'formula' => 'Formule : =$formula',
    'top' => '$rank premiers éléments',
    'bottom' => '$rank derniers éléments',
    'topPercent' => '$rank % premiers',
    'bottomPercent' => '$rank % derniers',
    'above' => 'Au-dessus de la moyenne',
    'below' => 'En dessous de la moyenne',
    'duplicateValues' => 'Valeurs en double',
    'uniqueValues' => 'Valeurs uniques',
    'colorScale' => 'Nuance de couleurs',
    'dataBar' => 'Barre de données',
    _ => 'Jeu d’icônes : ${iconSetNames.where((n) => n.$1 == icons).map((n) => n.$2).firstOrNull ?? icons}',
  };

  // data validation
  String get validationOptions => 'Options';
  String get validationPrompt => 'Message de saisie';
  String get validationAlert => 'Alerte d’erreur';
  String get allow => 'Autoriser';
  String get dataTools => 'Outils de données';
  String get dataOperator => 'Données';
  List<(String, String)> get validationTypes => const [
    ('', 'Tout'),
    ('whole', 'Nombre entier'),
    ('decimal', 'Décimal'),
    ('list', 'Liste'),
    ('date', 'Date'),
    ('time', 'Heure'),
    ('textLength', 'Longueur du texte'),
    ('custom', 'Personnalisé'),
  ];
  List<(String, String)> get validationOperators => const [
    ('between', 'entre'),
    ('notBetween', 'pas entre'),
    ('equal', 'égal à'),
    ('notEqual', 'différent de'),
    ('greaterThan', 'supérieur à'),
    ('lessThan', 'inférieur à'),
    ('greaterThanOrEqual', 'supérieur ou égal à'),
    ('lessThanOrEqual', 'inférieur ou égal à'),
  ];
  String get minimum => 'Minimum';
  String get maximum => 'Maximum';
  String get value => 'Valeur';
  String get source => 'Source (valeurs séparées par des points-virgules, ou =plage)';
  String get formula => 'Formule';
  String get ignoreBlank => 'Ignorer si vide';
  String get inCellDropdown => 'Liste déroulante dans la cellule';
  String get showAlert => 'Afficher une alerte si des données non valides sont saisies';
  String get alertStyle => 'Style';
  List<(String, String)> get alertStyles => const [('stop', 'Arrêt'), ('warning', 'Avertissement'), ('information', 'Information')];
  String get title => 'Titre';
  String get message => 'Message';
  String get clearAllValidation => 'Effacer tout';
  String invalidValue(String type, String op, String first, String second) {
    final kind = validationTypes.where((t) => t.$1 == type).map((t) => t.$2.toLowerCase()).firstOrNull;
    final how = validationOperators.where((o) => o.$1 == op).map((o) => o.$2).firstOrNull;
    if (kind == null || how == null || type == 'custom' || type == 'list') return notInList;
    return 'Valeur attendue : $kind $how $first${op == 'between' || op == 'notBetween' ? ' $and $second' : ''}.';
  }

  String get insertFunction => 'Insérer une fonction';
  String get functionLibrary => 'Bibliothèque de fonctions';
  String get nameBox => 'Zone Nom';
  String get sheetName => 'Feuil';
  String get newSheetName => 'Nom de la feuille';
  String get ok => 'OK';
  String get cancel => 'Annuler';
  String get average => 'Moyenne';
  String get countLabel => 'Nb (non vides)';
  String get sum => 'Somme';
  String get columnWidth => 'Largeur de colonne';
  String get rowHeight => 'Hauteur de ligne';
  String get hide => 'Masquer';
  String get unhide => 'Afficher';
  String get chartSheet => 'Le graphique de cette feuille ne peut pas s’afficher ici.';
  String get charts => 'Graphiques';
  String get chartDesign => 'Création de graphique';
  String get chartFormat => 'Format';
  String get chartTypeGroup => 'Type';
  String get changeChartType => 'Modifier le type de graphique';
  List<(String, String)> get chartTypes => const [
    ('column', 'Histogramme groupé'),
    ('bar', 'Barres groupées'),
    ('line', 'Courbes'),
    ('pie', 'Secteurs'),
    ('scatter', 'Nuage de points'),
    ('area', 'Aires'),
  ];
  String get chartData => 'Données';
  String get selectData => 'Sélectionner les données';
  String get chartLayouts => 'Dispositions du graphique';
  String get chartTitle => 'Titre du graphique';
  String get showChartTitle => 'Afficher le titre';
  String get chartTitleText => 'Texte (vide : titre automatique)';
  String get legend => 'Légende';
  List<(String, String)> get legendPositions => const [('none', 'Aucune'), ('r', 'Droite'), ('l', 'Gauche'), ('t', 'Haut'), ('b', 'Bas')];
  String get axisTitles => 'Titres des axes';
  String get categoryAxis => 'Axe des catégories';
  String get valueAxis => 'Axe des valeurs';
  String get seriesName => 'Nom de la série';
  String get seriesValues => 'Valeurs de la série';
  String get categoryLabels => 'Étiquettes des catégories';
  String get addSeries => 'Ajouter une série';
  String get removeSeries => 'Supprimer la série';
  String get addRow => 'Ajouter une ligne';
  String get removeRow => 'Supprimer la ligne';
  String get chartReadOnly => 'Les données de ce graphique sont en lecture seule : il n’a pas de classeur incorporé.';
  String get deleteChart => 'Supprimer le graphique';
  String get calculating => 'Calcul…';
  String get dataValidation => 'Validation des données';
  String get notInList => 'Cette valeur ne correspond pas aux restrictions de validation des données définies pour cette cellule.';
  String get continueQuestion => 'Continuer ?';
  String get yes => 'Oui';
  String get no => 'Non';
  String deleteSheetConfirm(String name) => 'Supprimer la feuille « $name » ? Ses données seront perdues.';
  List<(String, String)> get numberFormats => const [
    ('General', 'Standard'),
    ('#,##0.00', 'Nombre'),
    ('#,##0.00 "€"', 'Monétaire'),
    ('_-* #,##0.00 "€"_-;-* #,##0.00 "€"_-;_-* "-"?? "€"_-;_-@_-', 'Comptabilité'),
    ('dd/mm/yyyy', 'Date courte'),
    ('dddd d mmmm yyyy', 'Date longue'),
    ('hh:mm:ss', 'Heure'),
    ('0.00%', 'Pourcentage'),
    ('# ?/?', 'Fraction'),
    ('0.00E+00', 'Scientifique'),
    ('@', 'Texte'),
  ];

  // Word
  String get styles => 'Styles';
  String get layoutTab => 'Mise en page';
  String get margins => 'Marges';
  String get orientation => 'Orientation';
  String get portrait => 'Portrait';
  String get landscape => 'Paysage';
  String get size => 'Taille';
  String get columns => 'Colonnes';
  String get breaks => 'Sauts de page';
  String get pageBreak => 'Saut de page';
  String get pageSetup => 'Mise en page';
  String get pages => 'Pages';
  String get tables => 'Tableaux';
  String get insertTable => 'Insérer un tableau';
  String tableSize(int columns, int rows) => 'Tableau $columns x $rows';
  String get insertRowAbove => 'Insérer une ligne au-dessus';
  String get insertRowBelow => 'Insérer une ligne en dessous';
  String get insertColumnLeft => 'Insérer une colonne à gauche';
  String get insertColumnRight => 'Insérer une colonne à droite';
  String get deleteRow => 'Supprimer la ligne';
  String get deleteColumn => 'Supprimer la colonne';
  String get deleteTable => 'Supprimer le tableau';
  String get lineSpacing => 'Interligne et espacement de paragraphe';
  String get addSpaceBefore => 'Ajouter un espace avant le paragraphe';
  String get removeSpaceBefore => 'Supprimer l’espace avant le paragraphe';
  String get addSpaceAfter => 'Ajouter un espace après le paragraphe';
  String get removeSpaceAfter => 'Supprimer l’espace après le paragraphe';
  String get increaseIndent => 'Augmenter le retrait';
  String get decreaseIndent => 'Diminuer le retrait';
  String get clearFormatting => 'Effacer toute la mise en forme';
  String get superscript => 'Exposant';
  String get subscript => 'Indice';
  String pageOf(int n, int of) => 'Page $n sur $of';
  String words(int n) => n == 1 ? '1 mot' : '$n mots';
  String get illustrations => 'Illustrations';
  String get pictures => 'Images';
  String get pictureFailed => 'L’image n’a pas pu être ajoutée';
  String get find => 'Rechercher';
  String get replace => 'Remplacer';
  String get replaceAll => 'Remplacer tout';
  String get replaceWith => 'Remplacer par';
  String get findNext => 'Suivant';
  String get findPrevious => 'Précédent';
  String get matchCase => 'Respecter la casse';
  String results(int n) => n == 0 ? 'Aucun résultat' : (n == 1 ? '1 résultat' : '$n résultats');
  String resultOf(int n, int of) => 'Résultat $n sur $of';
  String pageCount(int n) => n == 1 ? '1 page' : '$n pages';
  String get headerFooter => 'En-tête et pied de page';
  String get header => 'En-tête';
  String get footer => 'Pied de page';
  String get pageNumber => 'Numéro de page';
  String get closeHeaderFooter => 'Fermer l’en-tête et le pied de page';
  String get normalMargins => 'Normales';
  String get narrowMargins => 'Étroites';
  String get moderateMargins => 'Modérées';
  String get wideMargins => 'Larges';
  String get onePage => 'Une page';
  String get pageWidth => 'Largeur de la page';
  String get comments => 'Commentaires';
  String get newComment => 'Nouveau commentaire';
  String get previousComment => 'Précédent';
  String get nextComment => 'Suivant';
  String get showComments => 'Afficher les commentaires';
  String get noComments => 'Aucun commentaire';
  String get noCommentsHint => 'Sélectionnez du texte, puis choisissez Nouveau commentaire.';
  String get newNote => 'Nouvelle note';
  String get editNote => 'Modifier la note';
  String get noSheetCommentsHint => 'Choisissez Nouveau commentaire ou Nouvelle note pour en ajouter à la cellule active.';
  String get noSlideCommentsHint => 'Choisissez Nouveau commentaire pour en ajouter un à cette diapositive.';
  String get startConversation => 'Commencer une conversation';
  String get replyHint => 'Répondre';
  String get post => 'Publier';
  String get save => 'Enregistrer';
  String get resolved => 'Résolu';
  String get resolveThread => 'Résoudre le thread';
  String get resolve => 'Résoudre';
  String get reopen => 'Rouvrir';
  String get editComment => 'Modifier le commentaire';
  String get deleteComment => 'Supprimer le commentaire';
  String get deleteNote => 'Supprimer la note';
  String get deleteThread => 'Supprimer le thread';
  String get moreActions => 'Autres actions';
  String get unknownAuthor => 'Auteur inconnu';
  String get commentedTextGone => 'Le texte commenté a été supprimé.';
  String replies(int n) => n == 1 ? '1 réponse' : '$n réponses';
  String get tracking => 'Suivi';
  String get trackChanges => 'Suivi des modifications';
  String trackChangesState(bool on) => 'Suivi des modifications : ${on ? 'activé' : 'désactivé'}';
  String get untracked => 'Vos modifications ne peuvent pas être suivies : le serveur ne vous a pas donné de nom.';
  String get changes => 'Modifications';
  String get accept => 'Accepter';
  String get reject => 'Refuser';
  String get acceptAndNext => 'Accepter et passer à la suivante';
  String get acceptThis => 'Accepter cette modification';
  String get acceptAll => 'Accepter toutes les modifications';
  String get acceptAllAndStop => 'Accepter toutes les modifications et arrêter le suivi';
  String get rejectAndNext => 'Refuser et passer à la suivante';
  String get rejectThis => 'Refuser la modification';
  String get rejectAll => 'Refuser toutes les modifications';
  String get rejectAllAndStop => 'Refuser toutes les modifications et arrêter le suivi';
  String acceptAllBy(String author) => 'Accepter toutes les modifications de $author';
  String rejectAllBy(String author) => 'Refuser toutes les modifications de $author';
  String get revisionsPane => 'Volet des révisions';
  String get noRevisions => 'Aucune modification suivie';
  String get acceptBatch => 'Accepter le lot';
  String get rejectBatch => 'Refuser le lot';
  String batchOf(String name, DateTime? date) => 'Lot de $name${date == null ? '' : ', ${commentDate(date.toLocal())}'}';
  String get previousChange => 'Précédent';
  String get nextChange => 'Suivant';
  String settleRevision(bool accept, bool deleted) => '${accept ? 'Accepter' : 'Refuser'} ${deleted ? 'la suppression' : 'l’insertion'}';
  String revision(bool deleted, String author, DateTime? date) => '${deleted ? 'Supprimé' : 'Inséré'} : $author${date == null ? '' : ', ${commentDate(date.toLocal())}'}';
  String commentDate(DateTime d) {
    const months = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.'];
    final now = DateTime.now();
    final time = '${d.hour.toString().padLeft(2, '0')}:${d.minute.toString().padLeft(2, '0')}';
    if (d.year == now.year && d.month == now.month && d.day == now.day) return 'Aujourd’hui, $time';
    return '${d.day} ${months[d.month - 1]} ${d.year}, $time';
  }

  List<(String, int, int)> get paperSizes => const [('A4', 11906, 16838), ('A5', 8391, 11906), ('Lettre US', 12240, 15840), ('Légal US', 12240, 20160)];

  /// The name Word shows for a style, those it defines in French.
  String styleName(String name) => _styleNames[name.toLowerCase()] ?? _heading(name) ?? name;

  String? _heading(String name) {
    final m = RegExp(r'^heading (\d)$', caseSensitive: false).firstMatch(name);
    if (m != null) return 'Titre ${m[1]}';
    final t = RegExp(r'^toc (\d)$', caseSensitive: false).firstMatch(name);
    if (t != null) return 'TM ${t[1]}';
    return null;
  }

  static const _styleNames = {
    'normal': 'Normal',
    'title': 'Titre',
    'subtitle': 'Sous-titre',
    'quote': 'Citation',
    'intense quote': 'Citation intense',
    'list paragraph': 'Paragraphe de liste',
    'no spacing': 'Sans interligne',
    'subtle emphasis': 'Accentuation légère',
    'emphasis': 'Accentuation',
    'intense emphasis': 'Accentuation intense',
    'strong': 'Élevé',
    'subtle reference': 'Référence légère',
    'intense reference': 'Référence intense',
    'book title': 'Titre du livre',
    'caption': 'Légende',
    'header': 'En-tête',
    'footer': 'Pied de page',
    'toc heading': 'En-tête de table des matières',
    'hyperlink': 'Lien hypertexte',
    'table grid': 'Grille du tableau',
    'footnote text': 'Note de bas de page',
    'default paragraph font': 'Police par défaut',
    'list bullet': 'Liste à puces',
    'list number': 'Liste numérotée',
  };

  // backstage
  String get info => 'Informations';
  String get close => 'Fermer';
  String get back => 'Retour';
  String get exportPdf => 'Exporter en PDF';
  String get exporting => 'Export en cours…';
  String exportFailed(String reason) => 'Échec de l’export en PDF : $reason';
  String get nothingToPrint => 'Aucune feuille du classeur n’a de quoi imprimer.';
  String exportMissing(String characters) => 'Aucune police de l’export ne dessine ces caractères, laissés en blanc : $characters';
  String exportIgnored(String what) => 'L’export en PDF a laissé de côté : $what';
  String ignoredLabel(Ignored what) => switch (what) {
    Ignored.colorFilter => 'des filtres de couleur',
    Ignored.imageFilter => 'des filtres d’image',
    Ignored.blendMode => 'des modes de fusion',
    Ignored.maskFilter => 'des effets de flou',
    Ignored.shader => 'des remplissages',
    Ignored.picture || Ignored.paragraph || Ignored.vertices || Ignored.atlas || Ignored.other => 'des éléments graphiques',
  };

  /// What to tell once a PDF is exported, or null when nothing was left aside.
  String? exportNotice(Set<String> missing, Set<Ignored> ignored) {
    final notices = [
      if (missing.isNotEmpty) exportMissing(missing.join(' ')),
      if (ignored.isNotEmpty) exportIgnored({for (final i in Ignored.values.where(ignored.contains)) ignoredLabel(i)}.join(', ')),
    ];
    return notices.isEmpty ? null : notices.join('\n');
  }

  String get editedBy => 'Modifié par';

  // status
  String slideOf(int n, int of) => 'Diapositive $n sur $of';
  String get language => 'Français (France)';
  String get saved => 'Enregistré';
  String get saving => 'Enregistrement…';
  String get offline => 'Hors connexion : vos modifications seront envoyées au retour de la connexion';
  String get connecting => 'Connexion…';
  String saveFailed(String reason) => 'Échec de l’enregistrement : $reason';
  String get readOnly => 'Lecture seule';
  String get csvNotice => 'Fichier CSV : seules les valeurs affichées sont enregistrées, sans formules ni mise en forme';
  String get retry => 'Réessayer';
  String get notesPrompt => 'Cliquez pour ajouter des notes';
  String refused(String reason) => 'Modification refusée : $reason';

  // placeholders
  String prompt(String kind) => switch (kind) {
    'title' => 'Cliquez pour ajouter un titre',
    'ctrTitle' => 'Cliquez pour ajouter un titre',
    'subTitle' => 'Cliquez pour ajouter un sous-titre',
    'dt' || 'ftr' || 'sldNum' || 'hdr' => '',
    _ => 'Cliquez pour ajouter du texte',
  };
  String get table => 'Tableau';
  String get rowsAndColumns => 'Lignes et colonnes';
  String get insertAbove => 'Insérer au-dessus';
  String get insertBelow => 'Insérer en dessous';
  String get insertLeft => 'Insérer à gauche';
  String get insertRight => 'Insérer à droite';
  String get merge => 'Fusionner';
  String get mergeCells => 'Fusionner les cellules';
  String get splitCells => 'Fractionner les cellules';
  String get cellSize => 'Taille de la cellule';
  String get height => 'Hauteur';
  String get width => 'Largeur';
  String get numberOfColumns => 'Nombre de colonnes';
  String get numberOfRows => 'Nombre de lignes';
  String get tableStyleOptions => 'Options de style de tableau';
  String get tableDesign => 'Création de tableau';
  String get centerVertically => 'Centrer verticalement';
  String get textDirection => 'Orientation du texte';
  String direction(String vert) => switch (vert) {
    'vert' => 'Rotation de 90° de tout le texte',
    'vert270' => 'Rotation de 270° de tout le texte',
    _ => 'Horizontal',
  };
  String get tableStyles => 'Styles de tableau';
  String get customStyles => 'Personnalisé';
  String get customStyle => 'Style personnalisé';
  String tableStyleSection(String kind) => switch (kind) {
    'Light' => 'Clair',
    'Medium' => 'Moyen',
    _ => 'Foncé',
  };

  /// The name of a built-in table style: [family] as Office names it in
  /// English, "Medium Style 2", and an accent from 1 to 6, 0 for none.
  String tableStyleName(String family, int accent) {
    if (accent == 0 && family == 'Themed Style 1') return 'Aucun style, aucune grille';
    if (accent == 0 && family == 'Themed Style 2') return 'Aucun style, grille du tableau';
    final kind = switch (family.split(' ').first) {
      'Themed' => 'Style à thème',
      'Light' => 'Style clair',
      'Medium' => 'Style moyen',
      _ => 'Style foncé',
    };
    final name = '$kind ${family.split(' ').last}';
    if (accent == 0) return name;
    return family == 'Dark Style 2' ? '$name - Accentuation $accent/Accentuation ${accent + 1}' : '$name - Accentuation $accent';
  }

  String get shading => 'Trame de fond';
  String get drawBorders => 'Dessiner des bordures';
  String get penWeight => 'Épaisseur du stylo';
  String get penColor => 'Couleur du stylo';
  String border(String which) => switch (which) {
    'bottom' => 'Bordure inférieure',
    'top' => 'Bordure supérieure',
    'left' => 'Bordure gauche',
    'right' => 'Bordure droite',
    'none' => 'Aucune bordure',
    'all' => 'Toutes les bordures',
    'outside' => 'Bordures extérieures',
    'inside' => 'Bordures intérieures',
    'insideH' => 'Bordure horizontale intérieure',
    'insideV' => 'Bordure verticale intérieure',
    'tl2br' => 'Bordure diagonale descendante',
    _ => 'Bordure diagonale montante',
  };
  String get headerRow => 'Ligne d’en-tête';
  String get totalRow => 'Ligne Total';
  String get bandedRows => 'Lignes à bandes';
  String get firstColumn => 'Première colonne';
  String get lastColumn => 'Dernière colonne';
  String get bandedColumns => 'Colonnes à bandes';
  String get chart => 'Graphique';
  String get diagram => 'SmartArt';
  String get object => 'Objet';
  String get shapeName => 'Forme';
  String get textBoxName => 'ZoneTexte';
  String get preview => 'Aperçu';
  String get transitionToThisSlide => 'Transition vers cette diapositive';
  String get effectOptions => 'Options d’effet';
  String get timing => 'Minutage';
  String get duration => 'Durée';
  String get applyToAll => 'Appliquer partout';
  String get onMouseClick => 'Manuellement';
  String get advanceAfter => 'Après';
  String transition(String effect) => switch (effect) {
    '' => 'Aucune',
    'cut' => 'Couper',
    'fade' => 'Fondu',
    'push' => 'Pousser',
    'wipe' => 'Balayer',
    'split' => 'Fractionner',
    'pull' => 'Découvrir',
    'cover' => 'Couvrir',
    'zoom' => 'Zoom',
    _ => 'Autre',
  };

  /// An option of a transition: a direction the slide goes in, "l" coming
  /// from the right, an axis and way, "vert out", or how it fades.
  String effectOption(String option) => switch (option) {
    'l' => 'De la droite',
    'r' => 'De la gauche',
    'u' => 'Du bas',
    'd' => 'Du haut',
    'lu' => 'Du coin inférieur droit',
    'ru' => 'Du coin inférieur gauche',
    'ld' => 'Du coin supérieur droit',
    'rd' => 'Du coin supérieur gauche',
    'in' => 'Avant',
    'out' => 'Arrière',
    'vert out' => 'Verticale vers l’extérieur',
    'vert in' => 'Verticale vers l’intérieur',
    'horz out' => 'Horizontale vers l’extérieur',
    'horz in' => 'Horizontale vers l’intérieur',
    'thruBlk' => 'Par le noir',
    _ => 'En douceur',
  };
  String get presenterView => 'Mode Présentateur';
  String get nextSlide => 'Diapositive suivante';
  String get previousSlide => 'Diapositive précédente';
  String get endSlideShow => 'Arrêter le diaporama';
  String get pauseTimer => 'Suspendre le minuteur';
  String get resumeTimer => 'Reprendre le minuteur';
  String get restartTimer => 'Redémarrer le minuteur';
  String get noNotes => 'Aucune note.';
  String get endOfShow => 'Fin du diaporama, cliquez pour quitter.';

  // keywords after an @
  String get ask => 'Demander';
  String get askHint => 'La réponse remplace la question';
  String get writeQuestion => 'Écrivez votre question, puis Entrée';
  String get thinking => 'Réflexion en cours…';
  String get noResult => 'Aucun résultat';
  String get searchFailed => 'La recherche a échoué';
  String get answerFailed => 'La réponse n’est pas venue';
  String get answerLost => 'La réponse n’a plus où s’écrire';
  String searchIn(String keyword) => 'Rechercher dans $keyword…';
  String questionFor(String keyword) => 'Votre question pour $keyword…';
}
