# Mémoire du projet (export a4)

Vue générée par a4 — ne pas éditer (outils remember / forget, `a4 mem`, panneau Mémoire).

## convention

### (général)
- **m73** xlsx : les notes et fils de commentaires sont les champs de cellule "note" ({author,text,shape}) et "thread" ([{id,author,date,text,done}]), jamais ensemble ; ils suivent donc lignes et colonnes sans code propre. Le writer (xlsx/comments.go) régénère comments, threadedComments, persons, VML et rels quand les cellules changent ou que des lignes bougent ; moveParts ne touche plus ni comments ni VML. Une nouvelle clé de cellule doit aussi entrer dans cellKeys de check.go (m43). — _ag5, t15_

### agent/helpers_test.go
- **m53** (à_revérifier) Les tests de l'agent ont des golden dans agent/testdata. testdata/sample.docx est généré par buildSample (helpers_test.go) ; `GOWORK=off go test ./agent -update` les réécrit. Lancer -update seul, sans -race ni -count : un golden écrit pendant une course reste faux. — _ag5, t23_

### dart/lib/src/excel/formula_text.dart
- **m26** (à_revérifier) La table de traduction français/anglais des fonctions est côté Dart (_functions et _newer dans dart/lib/src/excel/formula_text.dart), pas dans le package Go formula, qui ne connaît que les noms anglais. Une nouvelle fonction Go doit y recevoir son nom français, et son préfixe _xlfn. dans _newer si elle date d'Excel 2010 ou après. La table est une bijection (le nom français est une clé inverse) : deux noms anglais ne peuvent pas partager un nom français. — _ag2, t8_
- **m99** (à_revérifier) Les noms français des fonctions Excel 365 de manipulation de tableaux : PRENDRE, EXCLURE, CHOISIRCOLS, CHOISIRLIGNES, ETENDRE, ASSEMB.V, ASSEMB.H, DANSCOL, DANSLIGNE, ORGA.COLS, ORGA.LIGNES, FRACTIONNER.TEXTE. Les pages Microsoft fr-FR laissent DROP et EXPAND non traduits ; EXCLURE et ETENDRE viennent d'autres sources et sont à vérifier avec Excel FR. — _ag1, t12_

### formula/fn_securities.go
- **m57** formula : les fonctions de titres (PRICE, YIELD, COUP*, DURATION, ACCRINT) partagent le type `bond` de formula/fn_securities.go (dates des coupons précédent/suivant, nombre de coupons, décompte des jours par base 0-4). Les fonctions escomptées passent par `Context.discounted` et `Context.yearFrac`, extrait de YEARFRAC dans fn_date.go. — _ag5, t9_

### formula/rewrite.go
- **m61** (à_revérifier) formula : une fonction récente d'Excel doit être déclarée dans `newer` (formula/rewrite.go, remplie par init ; contient en plus ANCHORARRAY et SINGLE) et dans `_newer`/`_windowed` de dart/lib/src/excel/formula_text.dart, qui doivent rester alignés ; sinon une formule saisie est enregistrée sans son préfixe _xlfn. Ne pas confondre avec `prefixes` de fn_eng.go (préfixes du système métrique). — _ag6, t6_

### formula/structured.go
- **m90** (à_revérifier) formula : les références structurées sont calculées si le Book implémente l'interface formula.Tables (Table(name), TableAt(sheet,row,col)). resolve(tb, s, pos) donne la zone, et Engine.reads s'en sert pour les dépendances. Sans Tables, l'évaluation rend ErrUnsupported. — _ag6, t17_

### formula/value.go
- **m96** (à_revérifier) Values of type TypeLambda never leave the calculation: a cell, scalar() and the engine's cellValue turn them into #CALC!. — _ag5, t11_

### xlsx/links.go
- **m78** (à_revérifier) xlsx : les liens hypertexte sont le champ `link` de la cellule ({"url"|"loc","tip"}), lu par Document.links (xlsx/links.go) qui retire de la feuille les hyperlinks éditables ; ceux sur plus de 1024 cellules ou à adresse non canonique restent dans le XML de la feuille. Une fusion vérifiée par merges.apply (xlsx/merges.go) compare aux fusions de l'arbre. — _ag3, t14_

### xlsx/tables.go
- **m89** (à_revérifier) xlsx : les tableaux (ListObjects) sont l'attribut "tables" de leur feuille. Un tableau d'ID 0 ou inconnu est créé à la sauvegarde (writetables.go), et un tableau lu absent de l'attribut voit sa part supprimée. Le décalage par insertion ou suppression de lignes et colonnes est fait par le suivi serveur (Calc.Follow, tablefollow.go, Table.shifted). Le mover de moves.go ne décale plus que les tableaux que l'arbre ne porte pas. — _ag6, t17_

### xlsx/tables_test.go
- **m107** Pour tester une édition xlsx comme le serveur la traite : moveRows (Check, Apply, Moved) puis Calc.Follow(e, nil). Le helper sent() de xlsx/tables_test.go le fait ; apply() de edit_test.go ne lance ni Moved ni Follow. — _ag6, t17_

## piège

### (général)
- **m21** La machine n'a pas unzip. Pour lire un xlsx du corpus, utiliser python3 -I avec zipfile. — _ag5, t5_
- **m28** Dans zsh, `grep --include=*.go` échoue (« no matches found ») : mettre le glob entre guillemets. — _ag2, t8_
- **m74** xlsx notes : l'index "shape" d'une note est son rang dans l'ordre (ligne, colonne) du fichier lu, pas l'ordre de comments.xml, sinon une réécriture (qui émet dans l'ordre des cellules) change l'arbre relu (TestCorpusRewrite). Un fil lu dans un fichier a une note fantôme tc={GUID} qu'on ne lit pas comme une note. — _ag5, t15_
- **m75** Validation dotnet hors CI : dotnet vit dans /nix/store/qybc3wpz3hz5daipbmnj7rgc73hn35j2-dotnet-sdk-wrapped-8.0.425/bin/dotnet (pas dans le PATH). Écrire d'abord les classeurs avec LOFFICE_EDITED, LOFFICE_MOVED ou LOFFICE_COMMENTED, puis `DOTNET_CLI_HOME=/tmp/x LOFFICE_EDITED=1 dotnet run --project tools/validate -- corpus/files <dossier>` (~3 min), ou `dotnet build tools/validate -o /tmp/x` puis `dotnet /tmp/x/Validate.dll <originaux> <réécrits>` (la dll s'appelle Validate.dll ; les dossiers doivent porter les mêmes noms de fichiers). — _ag5, t15_

### agent/agent.go
- **m50** (à_revérifier) agent/agent.go : un client trame qui réutilise peer.Client doit reprendre la numérotation n de ses ops à partir du champ ack de la trame doc (ou vider Client). Le hub (room.apply) ignore sans répondre (ni ack ni nack) un op dont n <= acks[Client], et le client attend alors indéfiniment. — _ag5, t23_

### agent/edit.go
- **m52** (à_revérifier) docx : pour supprimer le dernier paragraphe d'un flux, on supprime la marque précédente et le contenu, et la marque finale prend les attributs de la précédente. Un flux ne peut pas perdre sa marque finale (ErrNoMark), et un body ou un tc doit finir par un paragraphe. Voir removeRange dans agent/edit.go. — _ag5, t23_

### dart/
- **m2** Flutter n'est pas dans le PATH sur cette machine : il est dans le store nix, /nix/store/x8cc2489gk0lhbgm5q3y7pxfb7v9r706-flutter-wrapped-3.47.4-sdk-links/bin (Flutter 3.47.4, Dart 3.13.3). L'ajouter au PATH avant flutter test / dart analyze dans loffice/dart. — _orchestra_

### dart/analysis_options.yaml
- **m6** dart format ne lit formatter.page_width de analysis_options.yaml qu'après `flutter pub get` (il lui faut .dart_tool/package_config.json) ; sans cela le réglage est ignoré silencieusement et la largeur reste 80. — _ag1, t1_

### dart/lib
- **m7** Le reformatage Dart à page_width 200 peut laisser des `if` sans accolades sur plusieurs lignes, que flutter_lints signale (curly_braces_in_flow_control_structures) : relancer flutter analyze après dart format. — _ag1, t1_

### formula/engine.go
- **m19** (à_revérifier) formula : les plages sont évaluées paresseusement, et la conversion scalaire (Context.scalar) lit les cellules après Eval. Tout état « en cours de calcul » (busy, pile) doit donc être levé après le remplissage de la valeur (fill), pas juste après Eval. — _ag5, t5_
- **m65** (à_revérifier) formula/engine.go : Calc rend dynamique toute formule tapée par le client, même scalaire (xlsx/calc.go, SetDynamic), donc un parcours de tout e.spills par cellule, formule ou lecture de plage devient quadratique : les parcours doivent porter sur e.spread (formules dont la plage dépasse leur cellule). BenchmarkFollow ne charge que des formules lues du fichier ; pour mesurer les formules saisies (Calc.set typed=true), utiliser BenchmarkFollowTyped (xlsx/calc_test.go), un collage OpCel de milliers de formules. — _ag6, t6_

### formula/fn_array_test.go
- **m100** Dans formula/eval_test.go et fn_array_test.go, evalTable applique l'intersection implicite (c.scalar) au résultat d'une plage : un test ne peut pas attendre une plage multi-cellules d'XLOOKUP ; il faut un tableau (matrice) ou une fonction. — _ag1, t12_

### formula/fn_finance2_test.go
- **m58** Les exemples de la doc Microsoft sont parfois des valeurs d'Excel arrondies à sa propre tolérance (XIRR 0.373362535 alors que la racine exacte est 0.3733625335) ou tronquées (TBILLYIELD, MIRR) : les tests comparent à la demi-unité du dernier chiffre écrit. — _ag5, t9_

### formula/fn_lambda_test.go
- **m93** In formula tests, do not name a defined name FACT (or any other built-in Excel function): the builtin is called instead of the name, and the engine reads no dependencies. — _ag5, t11_

### formula/fn_special.go
- **m27** math.Erfcinv de Go perd ses chiffres dans la queue (il calcule Erfinv(1-x)) : pour une normale inverse précise à 1e-9, affiner le résultat par Halley sur la CDF calculée avec Erfc (normInv, formula/fn_special.go). — _ag2, t8_

### internal/xmldom/xmldom_test.go
- **m37** (à_revérifier) loffice : opc importe internal/xmldom (opc/strict.go). Un test d'internal/xmldom ou d'internal/xmltok (package interne) ne doit donc pas importer opc, sinon il y a un cycle d'import en test. Pour lire un zip dans ces tests, utiliser archive/zip. — _ag1, t7_

### xlsx/
- **m10** Dans loffice (et dans ses worktrees a4), lancer go avec GOWORK=off : le go.work parent (/home/me/Code/Citadelle/go.work) n'inclut ni loffice ni les worktrees, et go build/vet/test échouent sinon (« directory prefix . does not contain modules listed in go.work »). `go test ./xlsx` sans -short dure ≈ 4 min : le lancer en arrière-plan ou avec un délai long. dotnet, qpdf et soffice ne sont pas installés : nix est disponible (nix shell nixpkgs#dotnet-sdk_8 etc.) pour les obtenir sans toucher au système. — _ag6, t6_

### xlsx/calc.go
- **m23** (à_revérifier) xlsx : le writer ne doit jamais réécrire le texte d'une formule lue d'un fichier, sinon TestCorpusRewrite échoue (LibreOffice écrit par exemple ISO.CEILING sans préfixe). Les préfixes _xlfn. sont posés à la saisie, par Calc via formula.Stored, et seulement pour les formules tapées par le client. — _ag6, t6_
- **m40** (à_revérifier) xlsx : Calc.Follow doit flusher après chaque ins/rem de l'édit (f.shift puis f.flush). Sinon les décalages successifs lisent le texte non décalé de l'arbre et le dernier écrase les précédents dans f.cells. Le book "moves" est posé par workbook.moved (format.go) avant calc.Follow, pour survivre à un panic du calcul. — _ag3, t28_

### xlsx/check.go
- **m43** xlsx : Document.Check (checkCell, xlsx/check.go) refuse toute clé de cellule qu'il ne connaît pas. Une nouvelle clé du modèle de cellule (par ex. cm) doit y recevoir son cas, sinon toute édition qui la porte est refusée (« this part of the workbook cannot be edited »). TestCorpusEdits, lancé seulement sans -short, le détecte. — _ag6, t6_

### xlsx/validation_test.go
- **m111** xlsx tests : le paquet xlsx a déjà une fonction de test `validation(fields string)` (validation_test.go) ; un nom de fonction non test `validation` dans le paquet fait échouer la compilation des tests mais pas go build. Lancer go vet ./xlsx après un renommage. Dans moveRows des tests, At est 1-based pour OpIns/OpRem sur les lignes (At:1 insère avant la ligne 1). — _ag2, t40_

## contrat

### agent/
- **m5** (à_revérifier) Contraintes de l'hôte (serveur Citadelle, Héraut) pour le MCP de loffice : voie principale = http.Handler monté par le serveur, Héraut l'appelle en HTTP depuis son conteneur (stdio = dev local seulement) ; autorisation à deux niveaux par une fonction de l'hôte à chaque requête (service appelant autorisé par l'admin, modèle Services v3, + droits de l'utilisateur « au nom de » sur ce document ; l'IA n'a aucun droit propre) ; écriture refusée si l'utilisateur n'a pas consenti ; auteur Word = nom stable de l'IA, demandeur en métadonnée du lot, UndoMine par demandeur ; aperçu→application avec version/ETag, échec sans écrasement ; délais de plusieurs dizaines de secondes ; rien d'exposé ni d'affiché si l'hôte n'active pas l'IA. — _orchestra_

### agent/state.go
- **m51** Le paquet loffice/agent donne aux paragraphes des ids pN qui suivent leur marque, et aux tableaux des ids tN. Ces ids et les versions (jeton de session-n) ne valent que pour un même Agent : une autre session renumérote. Le Journal (Options.Journal) transmet de session en session le « au nom de » des révisions. — _ag5, t23_

### dart/lib/src/excel/formula_text.dart
- **m29** (à_revérifier) Les noms français de T.TEST et CHISQ.TEST sont T.TEST et CHISQ.TEST (non traduits) ; leurs noms de compatibilité sont TEST.STUDENT et TEST.KHIDEUX. — _ag2, t8_

### formula/engine.go
- **m60** (à_revérifier) formula/engine.go : Engine.run calcule une liste de formules (all) par tour de déversement. La détection des cercles (Kahn puis Tarjan, circle, again) s'applique à all, pas à e.dirty. join remet cf.circle à nil à chaque tour. Le recalcul again ne vide pas e.filled, qui garde les cellules qu'un déversement a libérées. — _ag6, t6_

### formula/fn_lambda.go
- **m95** (à_revérifier) A call to an unknown function in a formula is first resolved as a defined name that holds a LAMBDA (Context.named); without a name → ErrUnsupported. On the Dart side, unknown function names keep their case in both translation directions. — _ag5, t11_

### formula/rewrite.go
- **m94** (à_revérifier) formula.Stored adds _xlpm. in front of LET/LAMBDA parameters and their uses in scope (rewrite.go params()), and _xlfn. in front of LET, LAMBDA, MAP, REDUCE, SCAN, BYROW, BYCOL, MAKEARRAY, ISOMITTED; the parser strips _xlpm. from names without a sheet. — _ag5, t11_

### mcp/tools.go
- **m113** loffice MCP : l'outil select (curseur ou présence de l'IA) reste autorisé comme une lecture (Access.Write=false), décision de l'hôte Citadelle : la présence ne modifie pas le document. Aucun tag de version loffice (v0.12.0…) ne doit être posé ni publié sans l'accord transmis par sys:main/orchestra. — _orchestra_

### opc/strict.go
- **m36** (à_revérifier) loffice opc : dans un paquet OOXML Strict (Package.Strict()), Read rend chaque partie XML traduite en Transitional (espaces de noms, types de relations, pourcentages DrawingML « 75% » → 75000), et Set/Add retraduisent en Strict. Les lecteurs et writers des formats ne voient donc que du Transitional. Table et liste des attributs pourcentage : opc/strict.go. — _ag1, t7_

### pptx/pptx.go
- **m38** (à_revérifier) loffice pptx : l'attribut « xml » d'une forme (XML brut hors texte) n'est pas mis à jour par OpSet avant la sauvegarde. Après une édition, l'arbre en mémoire diffère donc de sa relecture. Pour tester une réécriture, comparer la relecture à celle d'un fichier de référence qui a reçu les mêmes éditions. — _ag1, t7_

### xlsx/calc.go
- **m11** (à_revérifier) Formule dynamique Excel 365 dans l'arbre xlsx : l'ancre porte f, fa (aire déversée) et cm=1, et les cellules déversées n'ont que v. Une formule tapée par le client (OpCel avec f chaîne) est dynamique. Calc pose fa et cm, et le writer crée xl/metadata.xml (XLDAPR) dès qu'une cellule réécrite a cm et f (xlsx/metadata.go). — _ag6, t6_

### xlsx/check.go
- **m106** xlsx Check (check.go) refuse une édition qui mêle une insertion ou suppression de lignes/colonnes (OpIns/OpRem) avec d'autres changements. Un client qui doit insérer une ligne puis changer les tableaux ou les cellules envoie deux éditions : l'insertion seule, puis le reste (cf. dart/lib/src/excel/tables.dart insertTableEdits/totalsEdits). — _ag6, t17_

### xlsx/validation.go
- **m84** xlsx : la validation de données est le champ "valid" de la feuille (ex "lists", renommé en t16 ; xlsx/validation.go, dart/lib/src/excel/validation.dart) et la mise en forme conditionnelle le champ "cf" ; tous deux sont éditables, vérifiés par checkValidations/checkConditionals. Une règle dont le JSON est inchangé (priorité ignorée) garde son élément cfRule d'origine (cellIs, containsText, extLst x14 ; xlsx/writeconditional.go) ; seules les règles neuves reçoivent un cfRule et un <dxf> écrit par styleWriter.dxf. — _ag1, t16_
- **m110** xlsx : writeValidations (xlsx/validation.go) ne retire que les éléments dataValidation que le modèle lit (eachValidation → ok) ; les non lus (type inconnu, formule manquante, etc.), dans dataValidations ou x14 extLst, restent dans le XML avec leur sqref décalé ou retiré par mover.sheet (moves.go) avant l'écriture. Le test TestValidationKept (rules_test.go) utilise rulesBookWith(t, listed, extended) pour en injecter. — _ag2, t40_

### xlsx/xlsx.go
- **m20** (à_revérifier) xlsx : l'attribut book "iterate" ({"count","delta"} ou absent) reflète calcPr iterate/iterateCount/iterateDelta, avec les valeurs par défaut d'Excel 100/0.001. L'attribut book "circular" ({sheetId:["A1"…]}) est dérivé : posé par NewCalc et Follow, l'éditeur s'en sert pour avertir. — _ag5, t5_

## décision

### agent/
- **m3** (à_revérifier) Coédition IA dans loffice : l'IA est un participant de la salle du hub (paquet Go agent, en processus via un trame.Conn en mémoire), passe par le même Check que les humains, et écrit dans Word en suivi des modifications signé de son nom (lots atomiques avec aperçu CriticMarkup, UndoMine). Le serveur MCP est écrit à la main sans dépendance (http.Handler pour un hôte + cmd/loffice-mcp en stdio). Inspiré de WordCraft, Revise et Adeu ; pourquoi : réutiliser rebase, historique et UI d'acceptation existants. — _orchestra_

## préférence

### (général)
- **m1** Portée de la v1.0 de loffice fixée par l'humain : stabilité (aucun panic, aucun bug connu), fidélité des fichiers Office à l'aller-retour et à l'édition, et combler les manques fonctionnels ; l'API Go/Dart n'a pas à être figée et peut évoluer. — _orchestra_
- **m31** L'humain veut les sessions réparties entre les comptes du pool (claude1, claude2, claude3, shero) plutôt que concentrées sur un seul : l'orchestra fixe l'instance de chaque tâche selon l'usage des comptes (system_status), en mettant les tâches opus sur le compte le moins consommé. — _orchestra_

### agent/
- **m4** (à_revérifier) Coédition IA dans PowerPoint et Excel (sans suivi des modifications) : l'IA écrit directement, de façon visible (curseur et nom de l'IA, surlignage temporaire de ce qu'elle vient de modifier), avec « annuler les miennes » qui retire seulement ses éditions. Dans Word, suivi des modifications signé de son nom. — _orchestra_
- **m114** Coédition IA (décision de l'humain, 04/11) : l'IA écrit en direct dans le document même quand d'autres personnes l'ont ouvert ; aucune attente ni verrou parce que des humains sont présents dans la salle. — _orchestra_
