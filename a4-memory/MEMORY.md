# Mémoire du projet (export a4)

Vue générée par a4 — ne pas éditer (outils remember / forget, `a4 mem`, panneau Mémoire).

## convention

### (général)
- **m73** xlsx : les notes et fils de commentaires sont les champs de cellule "note" ({author,text,shape}) et "thread" ([{id,author,date,text,done}]), jamais ensemble ; ils suivent donc lignes et colonnes sans code propre. Le writer (xlsx/comments.go) régénère comments, threadedComments, persons, VML et rels quand les cellules changent ou que des lignes bougent ; moveParts ne touche plus ni comments ni VML. Une nouvelle clé de cellule doit aussi entrer dans cellKeys de check.go (m43). — _ag5, t15_

### agent/helpers_test.go
- **m53** (à_revérifier) Les tests de l'agent ont des golden dans agent/testdata. testdata/sample.docx est généré par buildSample (helpers_test.go) ; `GOWORK=off go test ./agent -update` les réécrit. Lancer -update seul, sans -race ni -count : un golden écrit pendant une course reste faux. — _ag5, t23_

### agent/sheets.go
- **m133** formula.ParseArea n'accepte pas une ligne ou colonne seule (« 5 », « C ») comme plage de lignes : agent/sheets.go les double (« 5:5 ») puis exige area.Rows ou area.Cols, ce qui refuse aussi une plage de cellules passée à delete_rows. — _ag2, t26_

### agent/sheets_test.go
- **m129** Tests xlsx de l'agent : attendre le recalcul du hub en interrogeant Range jusqu'à ce que la cellule ait la valeur ET la formule attendues, car la valeur seule peut déjà être la bonne avant que le Follower réécrive la formule décalée par un insert/delete. — _ag2b, t26_

### dart/lib/src/excel/formula_text.dart
- **m26** (à_revérifier) La table de traduction français/anglais des fonctions est côté Dart (_functions et _newer dans dart/lib/src/excel/formula_text.dart), pas dans le package Go formula, qui ne connaît que les noms anglais. Une nouvelle fonction Go doit y recevoir son nom français, et son préfixe _xlfn. dans _newer si elle date d'Excel 2010 ou après. La table est une bijection (le nom français est une clé inverse) : deux noms anglais ne peuvent pas partager un nom français. — _ag2, t8_
- **m99** (à_revérifier) Les noms français des fonctions Excel 365 de manipulation de tableaux : PRENDRE, EXCLURE, CHOISIRCOLS, CHOISIRLIGNES, ETENDRE, ASSEMB.V, ASSEMB.H, DANSCOL, DANSLIGNE, ORGA.COLS, ORGA.LIGNES, FRACTIONNER.TEXTE. Les pages Microsoft fr-FR laissent DROP et EXPAND non traduits ; EXCLURE et ETENDRE viennent d'autres sources et sont à vérifier avec Excel FR. — _ag1, t12_

### formula/fn_securities.go
- **m57** formula : les fonctions de titres (PRICE, YIELD, COUP*, DURATION, ACCRINT) partagent le type `bond` de formula/fn_securities.go (dates des coupons précédent/suivant, nombre de coupons, décompte des jours par base 0-4). Les fonctions escomptées passent par `Context.discounted` et `Context.yearFrac`, extrait de YEARFRAC dans fn_date.go. — _ag5, t9_

### formula/rewrite.go
- **m61** formula : une fonction récente d'Excel doit être déclarée dans `newer` (formula/rewrite.go, remplie par init ; contient en plus ANCHORARRAY et SINGLE) et dans `_newer`/`_windowed` de dart/lib/src/excel/formula_text.dart, qui doivent rester alignés ; sinon une formule saisie est enregistrée sans son préfixe _xlfn. Ne pas confondre avec `prefixes` de fn_eng.go (préfixes du système métrique). — _ag6, t6_

### formula/structured.go
- **m90** (à_revérifier) formula : les références structurées sont calculées si le Book implémente l'interface formula.Tables (Table(name), TableAt(sheet,row,col)). resolve(tb, s, pos) donne la zone, et Engine.reads s'en sert pour les dépendances. Sans Tables, l'évaluation rend ErrUnsupported. — _ag6, t17_

### formula/value.go
- **m96** (à_revérifier) Values of type TypeLambda never leave the calculation: a cell, scalar() and the engine's cellValue turn them into #CALC!. — _ag5, t11_

### tools/validate/run.sh
- **m121** tools/validate/run.sh [dossier [famille...]] lance toute la validation Open XML SDK (réécritures et éditions de opc, pptx, xlsx, legacy, docx) et prend dotnet de nix s'il manque dans le PATH ; il compile Validate.dll une fois dans <dossier>/validate. Les familles peuvent tourner en parallèle avec un même dossier ; opc (3641 fichiers) prend ~25 min, le total ~35 min. — _ag4, t29_
- **m167** Validation Open XML locale sans dotnet installé : `nix shell nixpkgs#dotnet-sdk_8 -c dotnet build tools/validate -c Release -o /tmp/v` puis `LOFFICE_EDITED=1 dotnet /tmp/v/Validate.dll corpus/files <dossier de docx réécrits>` ; corpus/files est un lien : find ne le suit pas, utiliser un glob Go. — _ag1, t36_

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

### agent/agent.go
- **m50** (à_revérifier) agent/agent.go : un client trame qui réutilise peer.Client doit reprendre la numérotation n de ses ops à partir du champ ack de la trame doc (ou vider Client). Le hub (room.apply) ignore sans répondre (ni ack ni nack) un op dont n <= acks[Client], et le client attend alors indéfiniment. — _ag5, t23_

### agent/direct.go
- **m151** xlsx : une suppression de lignes ou colonnes réécrit par le serveur (Follower, éditions sid 0) les formules des autres cellules, parfois en #REF! ; l'inverse d'un OpRem (ins + cellules) ne les rétablit pas. L'agent repose donc ces formules (state.broken, agent/direct.go) : celles dont Shift(-n) suivi de Shift(+n) ne redonne pas le texte d'origine. — _ag2, t26_

### agent/edit.go
- **m52** (à_revérifier) docx : pour supprimer le dernier paragraphe d'un flux, on supprime la marque précédente et le contenu, et la marque finale prend les attributs de la précédente. Un flux ne peut pas perdre sa marque finale (ErrNoMark), et un body ou un tc doit finir par un paragraphe. Voir removeRange dans agent/edit.go. — _ag5, t23_
- **m190** loffice agent : revises() (t36) compte l'op "delete", qui sert aussi aux slides. Le nœud docx behalf ne doit être ajouté que pour Word (!a.direct()), sinon le Check pptx refuse la suppression d'une slide (ErrReadOnly). — _ag2, t26_

### agent/helpers_test.go
- **m131** (à_revérifier) Dans les tests agent, human.edit envoie depuis la dernière version lue par le humain, pas la version courante : le hub transforme donc son op, et un Ins At=3 peut atterrir ligne 4. Consommer les frames op avec human.next("op") jusqu'à la version de l'agent avant d'envoyer une op positionnelle. — _ag2, t26_
- **m188** (à_revérifier) agent/helpers_test.go : trametest.Join attend « doc » juste après « sync » mais le hub peut envoyer une trame « eph » avant : human() attend « hello », envoie sync puis lit jusqu'à « doc » (h.next). Avant ce correctif TestBehalfNodesAreChecked échouait ~1 fois sur 3. — _ag5, t48_
- **m191** (à_revérifier) loffice agent, tests : dès que l'agent a publié un eph (sélection, éditions récentes), un humain qui rejoint la salle reçoit cet eph avant le frame doc. trametest.Join échoue alors avec « got eph frame, want doc ». Faire rejoindre les humains (room.human) avant les éditions de l'agent. — _ag2, t26_

### agent/sheets_test.go
- **m183** Revue de fusion loffice : git diff et les fichiers lus doivent venir de la tête de branche ET du résultat fusionné. Une branche a4 peut passer ses tests seule et ne plus compiler sur main (helper de test du même nom dans le même paquet) ou reposer sur une règle du Check que main a changée. Simuler la fusion dans un clone jetable (git clone puis git merge) avant le verdict. — _oa, t26_

### agent/slides.go
- **m150** pptx : les enfants d'un nœud master ne sont pas que ses layouts, les formes du masque (sp, pic…) y sont aussi (pptx/pptx.go) ; pour lister les dispositions (agent/slides.go), filtrer Type=="layout". Les fixtures testdata/pptx ont des masques sans forme et ne révèlent pas le cas : tester sur corpus/files/python-pptx (ex. mst-placeholders.pptx). — _ag2, t26_

### dart/
- **m2** (à_revérifier) dart/ Flutter et Dart ne sont pas dans le PATH de cette machine. Soit `nix shell nixpkgs#flutter -c flutter test` depuis loffice/dart (suite complète ≈ 1 min 40), soit ajouter au PATH /nix/store/x8cc2489gk0lhbgm5q3y7pxfb7v9r706-flutter-wrapped-3.47.4-sdk-links/bin (Flutter 3.47.4, Dart 3.13.3) avant flutter test / dart analyze. — _orchestra_

### dart/analysis_options.yaml
- **m6** dart format ne lit formatter.page_width de analysis_options.yaml qu'après `flutter pub get` (il lui faut .dart_tool/package_config.json) ; sans cela le réglage est ignoré silencieusement et la largeur reste 80. — _ag1, t1_

### dart/lib
- **m7** Le reformatage Dart à page_width 200 peut laisser des `if` sans accolades sur plusieurs lignes, que flutter_lints signale (curly_braces_in_flow_control_structures) : relancer flutter analyze après dart format. — _ag1, t1_

### formula/engine.go
- **m19** (à_revérifier) formula : les plages sont évaluées paresseusement, et la conversion scalaire (Context.scalar) lit les cellules après Eval. Tout état « en cours de calcul » (busy, pile) doit donc être levé après le remplissage de la valeur (fill), pas juste après Eval. — _ag5, t5_
- **m65** (à_revérifier) formula/engine.go : Calc rend dynamique toute formule tapée par le client, même scalaire (xlsx/calc.go, SetDynamic), donc un parcours de tout e.spills par cellule, formule ou lecture de plage devient quadratique : les parcours doivent porter sur e.spread (formules dont la plage dépasse leur cellule). BenchmarkFollow ne charge que des formules lues du fichier ; pour mesurer les formules saisies (Calc.set typed=true), utiliser BenchmarkFollowTyped (xlsx/calc_test.go), un collage OpCel de milliers de formules. — _ag6, t6_

### formula/eval.go
- **m172** formula : c.table rogne les colonnes d'une référence de lignes entières (drapeau Rows) à la largeur utilisée de sa propre feuille. Deux aires de même forme sur deux feuilles différentes peuvent donc donner des tables de largeurs différentes. Si un appelant fixe lui-même la forme (tables() pour COUNTIFS/SUMIF), il doit effacer Rows/Cols avant d'appeler c.table. — _ob, t33_

### formula/fn_array.go
- **m139** SORT décroissant met les vides en tête (TestArrays le fixe) : sur une colonne entière, la table compacte ne peut pas répéter sa dernière ligne, d'où sortedRows dans fn_array.go. — _ag4, t33_

### formula/fn_array_test.go
- **m100** Dans formula/eval_test.go et fn_array_test.go, evalTable applique l'intersection implicite (c.scalar) au résultat d'une plage : un test ne peut pas attendre une plage multi-cellules d'XLOOKUP ; il faut un tableau (matrice) ou une fonction. — _ag1, t12_

### formula/fn_finance2_test.go
- **m58** Les exemples de la doc Microsoft sont parfois des valeurs d'Excel arrondies à sa propre tolérance (XIRR 0.373362535 alors que la racine exacte est 0.3733625335) ou tronquées (TBILLYIELD, MIRR) : les tests comparent à la demi-unité du dernier chiffre écrit. — _ag5, t9_

### formula/fn_lambda_test.go
- **m93** (à_revérifier) In formula tests, do not name a defined name FACT (or any other built-in Excel function): the builtin is called instead of the name, and the engine reads no dependencies. — _ag5, t11_

### formula/fn_math.go
- **m173** formula : SUMPRODUCT sur des lignes entières de deux feuilles (Feuil1!1:1,Feuil2!1:1) rend #VALUE!. c.grid rogne chaque référence Rows à la largeur de sa propre feuille, puis fn_math.go compare les formes. Le défaut existe déjà sur main ; il n'est pas corrigé dans t33. — _ag4, t33_

### formula/fn_special.go
- **m27** math.Erfcinv de Go perd ses chiffres dans la queue (il calcule Erfinv(1-x)) : pour une normale inverse précise à 1e-9, affiner le résultat par Halley sur la CDF calculée avec Erfc (normInv, formula/fn_special.go). — _ag2, t8_

### formula/fn_stats.go
- **m169** formula : depuis les tables compactes, deux aires de même forme n'ont pas forcément la même longueur de Cells (c.table tient les lignes de la feuille + une vide répétée, selon R1). Pour lire plusieurs aires en parallèle (SUMIF, COUNTIFS), itérer sur Rows*Cols avec At(i/Cols, i%Cols), jamais indexer Cells[i] d'une table par l'indice d'une autre. — _ag4, t33_

### internal/xmldom/xmldom_test.go
- **m37** loffice : opc importe internal/xmldom (opc/strict.go). Un test d'internal/xmldom ou d'internal/xmltok (package interne) ne doit donc pas importer opc, sinon il y a un cycle d'import en test. Pour lire un zip dans ces tests, utiliser archive/zip. — _ag1, t7_

### mcp/office_test.go
- **m132** Un test d'annulation xlsx doit vérifier toute la grille, pas une cellule : après une annulation fausse, B1 peut valoir la bonne valeur par hasard alors que lignes et colonnes sont restées décalées. — _ag2, t26_

### pptx/transition_test.go
- **m122** `-run Edits` de pptx (CI, run.sh) n'écrit que 2 fichiers édités ; TestTransitions et TestComments écrivent aussi avec LOFFICE_EDITED (saveCorpus) mais sous des noms (comments/two.pptx) sans original : le validateur les ignore. Copier la sortie en <dossier>/poi/aptia.pptx pour la valider. — _ag4, t29_

### tools/validate/Program.cs
- **m75** Validation Open XML SDK hors CI : dotnet vit dans /nix/store/qybc3wpz3hz5daipbmnj7rgc73hn35j2-dotnet-sdk-wrapped-8.0.425/bin/dotnet (pas dans le PATH ; sinon `nix shell nixpkgs#dotnet-sdk_8 -c dotnet ...`). Écrire d'abord les classeurs avec LOFFICE_EDITED, LOFFICE_MOVED ou LOFFICE_COMMENTED (les tests de graphiques écrivent sous charts/), puis `DOTNET_CLI_HOME=/tmp/x LOFFICE_EDITED=1 dotnet run -c Release --project tools/validate -- corpus/files <dossier>` (~3 min), ou `dotnet build tools/validate -o /tmp/x` puis `dotnet /tmp/x/Validate.dll <originaux> <réécrits>` (la dll s'appelle Validate.dll). Les deux dossiers doivent porter les mêmes chemins relatifs. tools/validate/run.sh automatise l'ensemble (voir m121). — _ag5, t15_
- **m204** Les graphiques écrits par python-pptx/POI portent des c:axId négatifs, invalides pour l'Open XML SDK. Changer le type renomme l'élément de tracé (c:barChart → c:lineChart) et déplace donc ces erreurs ; tools/validate/Program.cs (Unplaced) ignore ce nom d'élément pour ne pas les compter comme nouvelles. — _ag3, t45_

### xlsx/
- **m10** Dans loffice (et dans ses worktrees a4), lancer go avec GOWORK=off : le go.work parent (/home/me/Code/Citadelle/go.work) n'inclut ni loffice ni les worktrees, et go build/vet/test échouent sinon (« directory prefix . does not contain modules listed in go.work »). `go test ./xlsx` sans -short dure ≈ 4 min : le lancer en arrière-plan ou avec un délai long. dotnet, qpdf et soffice ne sont pas installés : nix est disponible (nix shell nixpkgs#dotnet-sdk_8 etc.) pour les obtenir sans toucher au système. — _ag6, t6_

### xlsx/calc.go
- **m23** (à_revérifier) xlsx : le writer ne doit jamais réécrire le texte d'une formule lue d'un fichier, sinon TestCorpusRewrite échoue (LibreOffice écrit par exemple ISO.CEILING sans préfixe). Les préfixes _xlfn. sont posés à la saisie, par Calc via formula.Stored, et seulement pour les formules tapées par le client. — _ag6, t6_
- **m40** (à_revérifier) xlsx : Calc.Follow doit flusher après chaque ins/rem de l'édit (f.shift puis f.flush). Sinon les décalages successifs lisent le texte non décalé de l'arbre et le dernier écrase les précédents dans f.cells. Le book "moves" est posé par workbook.moved (format.go) avant calc.Follow, pour survivre à un panic du calcul. — _ag3, t28_
- **m148** (à_revérifier) Le coût d'un renommage de feuille sur 100 000 formules vient surtout de formula.Rename (réécriture des textes) et du Set de chaque formule dans Calc.rebuild, pas de la recherche des cercles (≈ 60 ms). BenchmarkRenameSheet et BenchmarkOpenFormulas (xlsx/calc_test.go) le mesurent. — _ag3, t34_
- **m175** loffice xlsx : quand des lignes ou colonnes sont supprimées (rem), le Follower (xlsx/calc.go shift) ne décale pas seulement les formules. Il décale ou retire aussi valid, cf, charts, filter, tables, print, names, le nœud looks et les attributs m/fa des cellules. Réinsérer les lignes ne répare pas ce qui pointait dans la bande supprimée : l'inverse d'un rem doit reposer ces attributs (agent state.unshifted). Le Check refuse d'écrire charts, qui part donc dans Record.Lost. — _ag2, t26_

### xlsx/chartbook_test.go
- **m203** Dans les tests xlsx du dépôt loffice, la feuille du classeur de test s'appelle « Feuille1 » et non « Sheet1 » : une référence de graphique « Sheet1!… » ne la trouve pas (ErrChartLayout dans chartbook). — _ag3, t45_

### xlsx/check.go
- **m43** (à_revérifier) xlsx : Document.Check (checkCell, xlsx/check.go) refuse toute clé de cellule qu'il ne connaît pas. Une nouvelle clé du modèle de cellule (par ex. cm) doit y recevoir son cas, sinon toute édition qui la porte est refusée (« this part of the workbook cannot be edited »). TestCorpusEdits, lancé seulement sans -short, le détecte. — _ag6, t6_

### xlsx/validation_test.go
- **m111** (à_revérifier) xlsx tests : le paquet xlsx a déjà une fonction de test `validation(fields string)` (validation_test.go) ; un nom de fonction non test `validation` dans le paquet fait échouer la compilation des tests mais pas go build. Lancer go vet ./xlsx après un renommage. Dans moveRows des tests, At est 1-based pour OpIns/OpRem sur les lignes (At:1 insère avant la ligne 1). — _ag2, t40_

## contrat

### agent/
- **m5** Contraintes de l'hôte (serveur Citadelle, Héraut) pour le MCP de loffice : voie principale = http.Handler monté par le serveur, Héraut l'appelle en HTTP depuis son conteneur (stdio = dev local seulement) ; autorisation à deux niveaux par une fonction de l'hôte à chaque requête (service appelant autorisé par l'admin, modèle Services v3, + droits de l'utilisateur « au nom de » sur ce document ; l'IA n'a aucun droit propre) ; écriture refusée si l'utilisateur n'a pas consenti ; auteur Word = nom stable de l'IA, demandeur en métadonnée du lot, UndoMine par demandeur ; aperçu→application avec version/ETag, échec sans écrasement ; délais de plusieurs dizaines de secondes ; rien d'exposé ni d'affiché si l'hôte n'active pas l'IA. — _orchestra_

### agent/direct.go
- **m130** UndoMine en édition directe : chaque utilisateur a une pile. Ses annulations s'enchaînent, la dernière d'abord, et ne sont jamais rebasées sur ses propres lots : rebaser sur eux perd de l'information, par exemple deux réécritures du même texte donnent « UnAlpha ». Seule une édition étrangère les traverse, en descendant la pile avec l'édition transformée à chaque étage (rebaseMine, rebase). — _ag2, t26_
- **m192** loffice agent : UndoMine refuse avec ErrChanged quand un humain a inséré ou supprimé, depuis, des lignes ou colonnes qui décaleraient ce que l'annulation repose tel quel : formule f, plage fa, fusion m, attributs valid/cf/filter/tables/print/charts, names (state.moves dans clash). — _ag2, t26_

### agent/edit.go
- **m152** Record.Lost (agent) liste ce qu'UndoMine a annulé sans pouvoir le refaire : les notes d'une slide et les commentaires d'autrui, que le Check pptx interdit de recréer. L'outil MCP undo_mine l'affiche avec « Not restored ». — _ag2, t26_
- **m166** (à_revérifier) loffice agent : le planner ne supprime directement que les insertions de l'agent faites pour le même utilisateur (docx.BehalfOf == lot, ou même date que le lot) ; sinon la suppression est une révision de suivi (jamais d'effacement sans trace). — _ag1, t36_

### agent/state.go
- **m51** (à_revérifier) Le paquet loffice/agent donne aux paragraphes des ids pN qui suivent leur marque, et aux tableaux des ids tN. Ces ids et les versions (jeton de session-n) ne valent que pour un même Agent : une autre session renumérote. Le Journal (Options.Journal) transmet de session en session le « au nom de » des révisions. — _ag5, t23_

### chart/edit.go
- **m141** Dans chart.Patch et refresh, une série est identifiée par son index (c:idx), pas par sa position : un client qui ajoute une série doit lui donner un index que le tracé n'a pas, sinon elle reprend l'élément de cet index avec sa mise en forme. Dart : ChartEdits.setSeries. — _ag1, t18_

### dart/lib/src/excel/formula_text.dart
- **m29** (à_revérifier) Les noms français de T.TEST et CHISQ.TEST sont T.TEST et CHISQ.TEST (non traduits) ; leurs noms de compatibilité sont TEST.STUDENT et TEST.KHIDEUX. — _ag2, t8_

### docx/behalf.go
- **m165** (à_revérifier) loffice docx : l'attribution « au nom de » d'une révision de l'agent est un nœud `behalf` (parent doc, id déterministe docx.BehalfNode(auteur, date, utilisateur)) écrit dans w:docVars de settings.xml (variable loffice.behalf.<id>) ; docx.BehalfOf(tree, auteur, date) la relit. Check refuse un nœud behalf d'un autre auteur, sa suppression et sa modification ; l'id déterministe fait refuser par le hub un doublon. Agent.date() saute les dates déjà prises. Options.Journal ne sert plus qu'à la monotonie des dates. — _ag1, t36_
- **m187** docx : à l'enregistrement, writer.liveBehalf ne garde que les nœuds behalf dont (auteur, date UTC) est porté par une révision (w:ins/w:del/…PrChange) d'une partie wordprocessingml du paquet ; settings.xml est réécrit si cet ensemble diffère de celui lu. BehalfOf compare aussi auteur et date du nœud (l'id FNV-32 seul peut collisionner). — _ag5, t48_

### formula/engine.go
- **m60** (à_revérifier) formula/engine.go : Engine.run calcule une liste de formules (all) par tour de déversement. La détection des cercles (Kahn puis Tarjan, circle, again) s'applique à all, pas à e.dirty. join remet cf.circle à nil à chaque tour. Le recalcul again ne vide pas e.filled, qui garde les cellules qu'un déversement a libérées. — _ag6, t6_
- **m147** (à_revérifier) formula.Engine.Known(circular) donne au nouvel engin les cercles de celui qu'il remplace et marque le scan fait : il sert à Calc.Follow quand l'édition ne fait que renommer des feuilles ou régler l'itération (mêmes positions, mêmes lectures), sinon Circular() refait un scan complet. — _ag3, t34_

### formula/eval.go
- **m156** formula : c.array ne remplit les lignes répétées d'une table compacte que jusqu'à maxRepeat (1<<16) cellules ou au double des cellules tenues ; au-delà il rend les lignes tenues sans la ligne répétée (comme une plage rognée à sa feuille). Une fonction qui doit garder la forme Excel d'une colonne entière doit travailler sur c.grid et rendre une table compacte (cf. EXPAND, TAKE, FILTER). — _ag4, t33_

### formula/fn_lambda.go
- **m95** (à_revérifier) A call to an unknown function in a formula is first resolved as a defined name that holds a LAMBDA (Context.named); without a name → ErrUnsupported. On the Dart side, unknown function names keep their case in both translation directions. — _ag5, t11_

### formula/rewrite.go
- **m94** (à_revérifier) formula.Stored adds _xlpm. in front of LET/LAMBDA parameters and their uses in scope (rewrite.go params()), and _xlfn. in front of LET, LAMBDA, MAP, REDUCE, SCAN, BYROW, BYCOL, MAKEARRAY, ISOMITTED; the parser strips _xlpm. from names without a sheet. — _ag5, t11_

### formula/value.go
- **m137** formula.Table peut être compacte : Cells ne tient que held() lignes, les lignes suivantes répètent la dernière tenue. Lire par At(i,j) ; ne jamais indexer Cells au-delà de held() sur une table issue de c.grid. c.grid rend une table éventuellement compacte, c.array une table dense (bornée par maxCells, rognée si la queue est vide). Une nouvelle fonction de tableau sur de grandes plages doit passer par c.grid et calculer sur les lignes tenues. — _ag4, t33_

### internal/chartpart/chartpart.go
- **m205** Graphiques docx/pptx (internal/chartpart) : le nœud porte « chart » (modèle chart.Chart, patché sur la seule partie) et « chartData » ({categories, series:[{index,name,values}]}, absent sans classeur incorporé ou pour une disposition non simple). Toute modification des données sans chartData est refusée par ErrReadOnly. En docx, le graphique est un nœud de type « chart » placé après le corps, que l'image désigne par img.chartNode. — _ag3, t45_

### mcp/tools.go
- **m113** (à_revérifier) loffice MCP : l'outil select (curseur ou présence de l'IA) reste autorisé comme une lecture (Access.Write=false), décision de l'hôte Citadelle : la présence ne modifie pas le document. Aucun tag de version loffice (v0.12.0…) ne doit être posé ni publié sans l'accord transmis par sys:main/orchestra. — _orchestra_

### opc/strict.go
- **m36** loffice opc : dans un paquet OOXML Strict (Package.Strict()), Read rend chaque partie XML traduite en Transitional (espaces de noms, types de relations, pourcentages DrawingML « 75% » → 75000), et Set/Add retraduisent en Strict. Les lecteurs et writers des formats ne voient donc que du Transitional. Table et liste des attributs pourcentage : opc/strict.go. — _ag1, t7_
- **m154** loffice opc/strict.go : la traduction Strict↔Transitional (Package.translate(contentType, data, toStrict bool)) passe par xmldom et ne touche que xmlns*, Type des Relationship, attributs uri et pourcentages DrawingML ; jamais le texte. Comparer e.Space avec l'URI du sens d'entrée (Strict à la lecture, Transitional à l'écriture), car Set sur xmlns ne met pas à jour Element.Space. — _ag6, t35_

### pptx/pptx.go
- **m38** (à_revérifier) loffice pptx : l'attribut « xml » d'une forme (XML brut hors texte) n'est pas mis à jour par OpSet avant la sauvegarde. Après une édition, l'arbre en mémoire diffère donc de sa relecture. Pour tester une réécriture, comparer la relecture à celle d'un fichier de référence qui a reçu les mêmes éditions. — _ag1, t7_

### xlsx/calc.go
- **m11** (à_revérifier) Formule dynamique Excel 365 dans l'arbre xlsx : l'ancre porte f, fa (aire déversée) et cm=1, et les cellules déversées n'ont que v. Une formule tapée par le client (OpCel avec f chaîne) est dynamique. Calc pose fa et cm, et le writer crée xl/metadata.xml (XLDAPR) dès qu'une cellule réécrite a cm et f (xlsx/metadata.go). — _ag6, t6_

### xlsx/charts.go
- **m142** checkChart (xlsx/charts.go) ne re-borne pas ce qu'un graphique garde de la version courante de l'arbre (comparaison JSON de chaque Data, titres égaux) : un graphique lu du fichier hors bornes ou une référence #REF! après suppression de lignes reste éditable. Seuls les nouveaux Data sont bornés, et maxChartRead limite les cellules lues anew par édit. — _ag1, t18_

### xlsx/check.go
- **m106** (à_revérifier) xlsx/check.go,dart/lib/src/excel/tables.dart xlsx Check (check.go, shifts > 0 && shifts < len(e)) refuse une édition qui mêle insertion ou suppression de lignes/colonnes (OpIns/OpRem) et autres changements. Un client envoie deux éditions : l'insertion seule, puis le reste (cf. insertTableEdits/totalsEdits de dart/lib/src/excel/tables.dart). Les tests Dart qui n'appliquent l'édition qu'en local (tree.apply) ne voient pas ce refus. — _ag6, t17_

### xlsx/validation.go
- **m84** (à_revérifier) xlsx : la validation de données est le champ "valid" de la feuille (ex "lists", renommé en t16 ; xlsx/validation.go, dart/lib/src/excel/validation.dart) et la mise en forme conditionnelle le champ "cf" ; tous deux sont éditables, vérifiés par checkValidations/checkConditionals. Une règle dont le JSON est inchangé (priorité ignorée) garde son élément cfRule d'origine (cellIs, containsText, extLst x14 ; xlsx/writeconditional.go) ; seules les règles neuves reçoivent un cfRule et un <dxf> écrit par styleWriter.dxf. — _ag1, t16_
- **m110** (à_revérifier) xlsx : writeValidations (xlsx/validation.go) ne retire que les éléments dataValidation que le modèle lit (eachValidation → ok) ; les non lus (type inconnu, formule manquante, etc.), dans dataValidations ou x14 extLst, restent dans le XML avec leur sqref décalé ou retiré par mover.sheet (moves.go) avant l'écriture. Le test TestValidationKept (rules_test.go) utilise rulesBookWith(t, listed, extended) pour en injecter. — _ag2, t40_

### xlsx/xlsx.go
- **m20** (à_revérifier) xlsx : l'attribut book "iterate" ({"count","delta"} ou absent) reflète calcPr iterate/iterateCount/iterateDelta, avec les valeurs par défaut d'Excel 100/0.001. L'attribut book "circular" ({sheetId:["A1"…]}) est dérivé : posé par NewCalc et Follow, l'éditeur s'en sert pour avertir. — _ag5, t5_

## décision

### agent/
- **m3** (à_revérifier) Coédition IA dans loffice : l'IA est un participant de la salle du hub (paquet Go agent, en processus via un trame.Conn en mémoire), passe par le même Check que les humains, et écrit dans Word en suivi des modifications signé de son nom (lots atomiques avec aperçu CriticMarkup, UndoMine). Le serveur MCP est écrit à la main sans dépendance (http.Handler pour un hôte + cmd/loffice-mcp en stdio). Inspiré de WordCraft, Revise et Adeu ; pourquoi : réutiliser rebase, historique et UI d'acceptation existants. — _orchestra_

### formula/eval.go
- **m138** La borne mémoire de formula : maxCells ne limite que les tableaux construits d'après une taille demandée ou une zone clairsemée ; une zone au moins à moitié remplie (crowded) est admise quelle que soit sa taille, sa mémoire étant proportionnelle au classeur. — _ag4, t33_
- **m181** t33 (formula, grandes plages) : le travail bloqué après 4 refus de revue est repris, pas abandonné, depuis a4/loffice/ag4-t33-backup, avec des invariants et des preuves systématiques : une table compacte n'est jamais énumérée sans borne, la borne vit à un seul endroit, c.array ne change pas la forme d'une référence explicite, et tout ce que main calcule sans #NUM! garde son résultat. Pourquoi : la revue trouvait de nouveaux cas à chaque round, la correction cas par cas ne convergeait pas. — _orchestra_

## préférence

### (général)
- **m1** Portée de la v1.0 de loffice fixée par l'humain : stabilité (aucun panic, aucun bug connu), fidélité des fichiers Office à l'aller-retour et à l'édition, et combler les manques fonctionnels ; l'API Go/Dart n'a pas à être figée et peut évoluer. — _orchestra_
- **m31** L'humain veut les sessions réparties entre les comptes du pool (claude1, claude2, claude3, shero) plutôt que concentrées sur un seul : l'orchestra fixe l'instance de chaque tâche selon l'usage des comptes (system_status), en mettant les tâches opus sur le compte le moins consommé. — _orchestra_
- **m198** 📌 Profils de tâches demandés par l'humain le 11/11/2026, pour économiser l'usage. Par défaut sonnet/medium ; sonnet/high pour une tâche délicate ; haiku/high, jamais haiku/medium, pour le trivial et le mécanique (renommages, déplacements purs, découpage de gros fichiers, mises en forme, petites relectures). Opus seulement par exception justifiée, quand sonnet a échoué ou qu'un bug subtil est identifié, et jamais pour une tâche longue : une tâche opus de 240 requêtes à 150 k de contexte a épuisé une fenêtre de 5 h en 40 minutes. — _orchestra_
- **m199** 📌 Économie de contexte, demandée par l'humain le 11/11/2026, pour les agents, les relecteurs, les subagents de fusion et l'orchestra. Ne jamais afficher un fichier entier de plus de 150 lignes, ni la sortie brute d'un test, d'un build ou d'un banc. Chercher avec grep -n, lire une plage (sed -n 'a,bp', ou Read avec offset/limit), envoyer les sorties longues dans un fichier filtré par grep ou tail -n 40. Ne pas relire un fichier déjà lu. — _orchestra_
- **m200** 📌 Tests allégés, chacun exécuté une seule fois (humain, 11/11/2026). Pas de suite complète : ni flutter test ou dart test sans cible, ni go test ./... du module. Lancer analyze ou vet, puis les tests ciblés des fichiers touchés et de leurs voisins, une seule fois, sans reprise « pour être sûr ». La suite complète reste aux merge_checks qu'a4 lance lui-même, sans modèle. — _orchestra_
- **m201** Charge et machine (humain, 11/11/2026). Au plus 3 à 4 agents actifs à la fois, et des tâches réparties sur plusieurs instances (paramètre instance) plutôt que toutes sur une seule. La machine (31 Go, sans swap) est partagée : tout run de navigateur ou de VM se lance dans bancs.slice (`systemd-run --user --scope -q --slice=bancs.slice -p MemoryMax=6G -p MemorySwapMax=0 -- …`). Pas de banc pour un changement sans effet sur le client. — _orchestra_

### agent/
- **m4** Coédition IA dans PowerPoint et Excel (sans suivi des modifications) : l'IA écrit directement, de façon visible (curseur et nom de l'IA, surlignage temporaire de ce qu'elle vient de modifier), avec « annuler les miennes » qui retire seulement ses éditions. Dans Word, suivi des modifications signé de son nom. — _orchestra_
- **m114** (à_revérifier) Coédition IA (décision de l'humain, 04/11) : l'IA écrit en direct dans le document même quand d'autres personnes l'ont ouvert ; aucune attente ni verrou parce que des humains sont présents dans la salle. — _orchestra_

### chart/
- **m186** (à_revérifier) Portée restante de la v1.0 de loffice, fixée par l'humain le 05/11 : fidélité seulement, soit les graphiques Excel 2016 (chartex), les objets éditables dans Word (images, champs) et les animations PowerPoint. Les formats .doc, .ppt, .xlsb et ODF sont repoussés après la 1.0. — _orchestra_
