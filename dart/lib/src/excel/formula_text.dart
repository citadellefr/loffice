/// The names of functions in the French version of Excel, by their names
/// in files. Those marked with a prefix are newer than the file format:
/// files call them "_xlfn.XLOOKUP".
const _functions = <String, String>{
  'ABS': 'ABS',
  'ACCRINT': 'INTERET.ACC',
  'ACCRINTM': 'INTERET.ACC.MAT',
  'ACOS': 'ACOS',
  'ACOSH': 'ACOSH',
  'ADDRESS': 'ADRESSE',
  'AGGREGATE': 'AGREGAT',
  'AND': 'ET',
  'ARABIC': 'CHIFFRE.ARABE',
  'AREAS': 'ZONES',
  'ARRAYTOTEXT': 'TABLEAUENTEXTE',
  'ASC': 'ASC',
  'ASIN': 'ASIN',
  'ASINH': 'ASINH',
  'ATAN': 'ATAN',
  'ATAN2': 'ATAN2',
  'ATANH': 'ATANH',
  'AVEDEV': 'ECART.MOYEN',
  'AVERAGE': 'MOYENNE',
  'AVERAGEA': 'AVERAGEA',
  'AVERAGEIF': 'MOYENNE.SI',
  'AVERAGEIFS': 'MOYENNE.SI.ENS',
  'BESSELJ': 'BESSELJ',
  'BETA.DIST': 'LOI.BETA.N',
  'BETADIST': 'LOI.BETA',
  'BIN2DEC': 'BINDEC',
  'BIN2HEX': 'BINHEX',
  'BIN2OCT': 'BINOCT',
  'BINOM.DIST': 'LOI.BINOMIALE.N',
  'BINOMDIST': 'LOI.BINOMIALE',
  'BITAND': 'BITET',
  'BITLSHIFT': 'BITDECALG',
  'BITOR': 'BITOU',
  'BITRSHIFT': 'BITDECALD',
  'BITXOR': 'BITOUEXCLUSIF',
  'CEILING': 'PLAFOND',
  'CEILING.MATH': 'PLAFOND.MATH',
  'CEILING.PRECISE': 'PLAFOND.PRECIS',
  'CELL': 'CELLULE',
  'CHAR': 'CAR',
  'CHIDIST': 'LOI.KHIDEUX',
  'CHISQ.DIST': 'LOI.KHIDEUX.N',
  'CHISQ.DIST.RT': 'LOI.KHIDEUX.DROITE',
  'CHISQ.TEST': 'CHISQ.TEST',
  'CHITEST': 'TEST.KHIDEUX',
  'CHOOSE': 'CHOISIR',
  'CLEAN': 'EPURAGE',
  'CODE': 'CODE',
  'COLUMN': 'COLONNE',
  'COLUMNS': 'COLONNES',
  'COMBIN': 'COMBIN',
  'COMBINA': 'COMBINA',
  'CONCAT': 'CONCAT',
  'CONCATENATE': 'CONCATENER',
  'CONFIDENCE': 'INTERVALLE.CONFIANCE',
  'CONFIDENCE.NORM': 'INTERVALLE.CONFIANCE.NORMAL',
  'CONFIDENCE.T': 'INTERVALLE.CONFIANCE.STUDENT',
  'CONVERT': 'CONVERT',
  'CORREL': 'COEFFICIENT.CORRELATION',
  'COS': 'COS',
  'COSH': 'COSH',
  'COUNT': 'NB',
  'COUNTA': 'NBVAL',
  'COUNTBLANK': 'NB.VIDE',
  'COUNTIF': 'NB.SI',
  'COUNTIFS': 'NB.SI.ENS',
  'COUPDAYBS': 'NB.JOURS.COUPON.PREC',
  'COUPDAYS': 'NB.JOURS.COUPON',
  'COUPDAYSNC': 'NB.JOURS.COUPON.SUIV',
  'COUPNCD': 'DATE.COUPON.SUIV',
  'COUPNUM': 'NB.COUPONS',
  'COUPPCD': 'DATE.COUPON.PREC',
  'COVAR': 'COVARIANCE',
  'COVARIANCE.P': 'COVARIANCE.PEARSON',
  'COVARIANCE.S': 'COVARIANCE.STANDARD',
  'CUMIPMT': 'CUMUL.INTER',
  'CUMPRINC': 'CUMUL.PRINCPER',
  'DATE': 'DATE',
  'DATEDIF': 'DATEDIF',
  'DATEVALUE': 'DATEVAL',
  'DAY': 'JOUR',
  'DAYS': 'JOURS',
  'DAYS360': 'JOURS360',
  'DB': 'DB',
  'DDB': 'DDB',
  'DEC2BIN': 'DECBIN',
  'DEC2HEX': 'DECHEX',
  'DEC2OCT': 'DECOCT',
  'DEGREES': 'DEGRES',
  'DELTA': 'DELTA',
  'DEVSQ': 'SOMME.CARRES.ECARTS',
  'DISC': 'TAUX.ESCOMPTE',
  'DOLLAR': 'DEVISE',
  'DOLLARDE': 'PRIX.DEC',
  'DOLLARFR': 'PRIX.FRAC',
  'DURATION': 'DUREE',
  'EDATE': 'MOIS.DECALER',
  'EFFECT': 'TAUX.EFFECTIF',
  'EOMONTH': 'FIN.MOIS',
  'ERF': 'ERF',
  'ERFC': 'ERFC',
  'ERROR.TYPE': 'TYPE.ERREUR',
  'EVEN': 'PAIR',
  'EXACT': 'EXACT',
  'EXP': 'EXP',
  'EXPON.DIST': 'LOI.EXPONENTIELLE.N',
  'EXPONDIST': 'LOI.EXPONENTIELLE',
  'F.DIST': 'LOI.F.N',
  'F.DIST.RT': 'LOI.F.DROITE',
  'FACT': 'FACT',
  'FACTDOUBLE': 'FACTDOUBLE',
  'FALSE': 'FAUX',
  'FDIST': 'LOI.F',
  'FIND': 'TROUVE',
  'FILTER': 'FILTRE',
  'FIXED': 'CTXT',
  'FLOOR': 'PLANCHER',
  'FLOOR.MATH': 'PLANCHER.MATH',
  'FLOOR.PRECISE': 'PLANCHER.PRECIS',
  'FORECAST': 'PREVISION',
  'FORECAST.LINEAR': 'PREVISION.LINEAIRE',
  'FORMULATEXT': 'FORMULETEXTE',
  'FREQUENCY': 'FREQUENCE',
  'FV': 'VC',
  'FVSCHEDULE': 'VC.PAIEMENTS',
  'GAMMA.DIST': 'LOI.GAMMA.N',
  'GAMMADIST': 'LOI.GAMMA',
  'GCD': 'PGCD',
  'GEOMEAN': 'MOYENNE.GEOMETRIQUE',
  'GESTEP': 'SUP.SEUIL',
  'GROWTH': 'CROISSANCE',
  'HARMEAN': 'MOYENNE.HARMONIQUE',
  'HEX2BIN': 'HEXBIN',
  'HEX2DEC': 'HEXDEC',
  'HEX2OCT': 'HEXOCT',
  'HLOOKUP': 'RECHERCHEH',
  'HOUR': 'HEURE',
  'HYPERLINK': 'LIEN_HYPERTEXTE',
  'IF': 'SI',
  'IFERROR': 'SIERREUR',
  'IFNA': 'SI.NON.DISP',
  'IFS': 'SI.CONDITIONS',
  'INDEX': 'INDEX',
  'INDIRECT': 'INDIRECT',
  'INFO': 'INFORMATIONS',
  'INT': 'ENT',
  'INTERCEPT': 'ORDONNEE.ORIGINE',
  'INTRATE': 'TAUX.INTERET',
  'IPMT': 'INTPER',
  'IRR': 'TRI',
  'ISBLANK': 'ESTVIDE',
  'ISERR': 'ESTERR',
  'ISERROR': 'ESTERREUR',
  'ISEVEN': 'EST.PAIR',
  'ISFORMULA': 'ESTFORMULE',
  'ISLOGICAL': 'ESTLOGIQUE',
  'ISNA': 'ESTNA',
  'ISNONTEXT': 'ESTNONTEXTE',
  'ISNUMBER': 'ESTNUM',
  'ISO.CEILING': 'ISO.PLAFOND',
  'ISODD': 'EST.IMPAIR',
  'ISOWEEKNUM': 'NO.SEMAINE.ISO',
  'ISREF': 'ESTREF',
  'ISTEXT': 'ESTTEXTE',
  'KURT': 'KURTOSIS',
  'LARGE': 'GRANDE.VALEUR',
  'LCM': 'PPCM',
  'LEFT': 'GAUCHE',
  'LEFTB': 'GAUCHEB',
  'LEN': 'NBCAR',
  'LENB': 'LENB',
  'LINEST': 'DROITEREG',
  'LN': 'LN',
  'LOG': 'LOG',
  'LOG10': 'LOG10',
  'LOGEST': 'LOGREG',
  'LOGNORM.DIST': 'LOI.LOGNORMALE.N',
  'LOGNORMDIST': 'LOI.LOGNORMALE',
  'LOOKUP': 'RECHERCHE',
  'LOWER': 'MINUSCULE',
  'MATCH': 'EQUIV',
  'MAX': 'MAX',
  'MAXA': 'MAXA',
  'MAXIFS': 'MAX.SI.ENS',
  'MDETERM': 'DETERMAT',
  'MDURATION': 'DUREE.MODIFIEE',
  'MEDIAN': 'MEDIANE',
  'MID': 'STXT',
  'MIDB': 'STXTB',
  'MIN': 'MIN',
  'MINA': 'MINA',
  'MINIFS': 'MIN.SI.ENS',
  'MINUTE': 'MINUTE',
  'MINVERSE': 'INVERSEMAT',
  'MIRR': 'TRIM',
  'MMULT': 'PRODUITMAT',
  'MOD': 'MOD',
  'MODE': 'MODE',
  'MODE.SNGL': 'MODE.SIMPLE',
  'MONTH': 'MOIS',
  'MROUND': 'ARRONDI.AU.MULTIPLE',
  'MULTINOMIAL': 'MULTINOMIALE',
  'MUNIT': 'MATRICE.UNITAIRE',
  'N': 'N',
  'NA': 'NA',
  'NETWORKDAYS': 'NB.JOURS.OUVRES',
  'NETWORKDAYS.INTL': 'NB.JOURS.OUVRES.INTL',
  'NOMINAL': 'TAUX.NOMINAL',
  'NORM.DIST': 'LOI.NORMALE.N',
  'NORM.INV': 'LOI.NORMALE.INVERSE.N',
  'NORM.S.DIST': 'LOI.NORMALE.STANDARD.N',
  'NORM.S.INV': 'LOI.NORMALE.STANDARD.INVERSE.N',
  'NORMDIST': 'LOI.NORMALE',
  'NORMINV': 'LOI.NORMALE.INVERSE',
  'NORMSDIST': 'LOI.NORMALE.STANDARD',
  'NORMSINV': 'LOI.NORMALE.STANDARD.INVERSE',
  'NOT': 'NON',
  'NOW': 'MAINTENANT',
  'NPER': 'NPM',
  'NPV': 'VAN',
  'NUMBERVALUE': 'VALEURNOMBRE',
  'OCT2BIN': 'OCTBIN',
  'OCT2DEC': 'OCTDEC',
  'OCT2HEX': 'OCTHEX',
  'ODD': 'IMPAIR',
  'OFFSET': 'DECALER',
  'OR': 'OU',
  'PEARSON': 'PEARSON',
  'PERCENTILE': 'CENTILE',
  'PERCENTILE.EXC': 'CENTILE.EXCLURE',
  'PERCENTILE.INC': 'CENTILE.INCLURE',
  'PERCENTRANK': 'RANG.POURCENTAGE',
  'PERCENTRANK.INC': 'RANG.POURCENTAGE.INCLURE',
  'PERMUT': 'PERMUTATION',
  'PI': 'PI',
  'PMT': 'VPM',
  'POISSON': 'LOI.POISSON',
  'POISSON.DIST': 'LOI.POISSON.N',
  'POWER': 'PUISSANCE',
  'PPMT': 'PRINCPER',
  'PRICE': 'PRIX.TITRE',
  'PRICEDISC': 'PRIX.TITRE.ESCOMPTE',
  'PRODUCT': 'PRODUIT',
  'PROPER': 'NOMPROPRE',
  'PV': 'VA',
  'QUARTILE': 'QUARTILE',
  'QUARTILE.EXC': 'QUARTILE.EXCLURE',
  'QUARTILE.INC': 'QUARTILE.INCLURE',
  'QUOTIENT': 'QUOTIENT',
  'RADIANS': 'RADIANS',
  'RAND': 'ALEA',
  'RANDARRAY': 'TABLEAU.ALEA',
  'RANDBETWEEN': 'ALEA.ENTRE.BORNES',
  'RANK': 'RANG',
  'RANK.AVG': 'MOYENNE.RANG',
  'RANK.EQ': 'EQUATION.RANG',
  'RATE': 'TAUX',
  'RECEIVED': 'VALEUR.ENCAISSEMENT',
  'REPLACE': 'REMPLACER',
  'REPT': 'REPT',
  'RIGHT': 'DROITE',
  'RIGHTB': 'DROITEB',
  'ROMAN': 'ROMAIN',
  'ROUND': 'ARRONDI',
  'ROUNDDOWN': 'ARRONDI.INF',
  'ROUNDUP': 'ARRONDI.SUP',
  'ROW': 'LIGNE',
  'ROWS': 'LIGNES',
  'RRI': 'TAUX.INT.EQUIV',
  'RSQ': 'COEFFICIENT.DETERMINATION',
  'SEARCH': 'CHERCHE',
  'SECOND': 'SECONDE',
  'SEQUENCE': 'SEQUENCE',
  'SERIESSUM': 'SOMME.SERIES',
  'SHEET': 'FEUILLE',
  'SHEETS': 'FEUILLES',
  'SIGN': 'SIGNE',
  'SIN': 'SIN',
  'SINH': 'SINH',
  'SKEW': 'COEFFICIENT.ASYMETRIE',
  'SLN': 'AMORLIN',
  'SLOPE': 'PENTE',
  'SMALL': 'PETITE.VALEUR',
  'SORT': 'TRIER',
  'SORTBY': 'TRIERPAR',
  'SQRT': 'RACINE',
  'SQRTPI': 'RACINE.PI',
  'STANDARDIZE': 'CENTREE.REDUITE',
  'STDEV': 'ECARTYPE',
  'STDEV.P': 'ECARTYPE.PEARSON',
  'STDEV.S': 'ECARTYPE.STANDARD',
  'STDEVA': 'STDEVA',
  'STDEVP': 'ECARTYPEP',
  'STDEVPA': 'STDEVPA',
  'SUBSTITUTE': 'SUBSTITUE',
  'SUBTOTAL': 'SOUS.TOTAL',
  'SUM': 'SOMME',
  'SUMIF': 'SOMME.SI',
  'SUMIFS': 'SOMME.SI.ENS',
  'SUMPRODUCT': 'SOMMEPROD',
  'SUMSQ': 'SOMME.CARRES',
  'SUMX2MY2': 'SOMME.X2MY2',
  'SUMX2PY2': 'SOMME.X2PY2',
  'SUMXMY2': 'SOMME.XMY2',
  'SWITCH': 'SI.MULTIPLE',
  'SYD': 'SYD',
  'T': 'T',
  'T.DIST': 'LOI.STUDENT.N',
  'T.DIST.2T': 'LOI.STUDENT.BILATERALE',
  'T.DIST.RT': 'LOI.STUDENT.DROITE',
  'T.INV': 'LOI.STUDENT.INVERSE.N',
  'T.INV.2T': 'LOI.STUDENT.INVERSE.BILATERALE',
  'T.TEST': 'T.TEST',
  'TAN': 'TAN',
  'TANH': 'TANH',
  'TBILLEQ': 'TAUX.ESCOMPTE.R',
  'TBILLPRICE': 'PRIX.BON.TRESOR',
  'TBILLYIELD': 'RENDEMENT.BON.TRESOR',
  'TDIST': 'LOI.STUDENT',
  'TEXT': 'TEXTE',
  'TEXTAFTER': 'TEXTE.APRES',
  'TEXTBEFORE': 'TEXTE.AVANT',
  'TEXTJOIN': 'JOINDRE.TEXTE',
  'TIME': 'TEMPS',
  'TIMEVALUE': 'TEMPSVAL',
  'TINV': 'LOI.STUDENT.INVERSE',
  'TODAY': 'AUJOURDHUI',
  'TRANSPOSE': 'TRANSPOSE',
  'TREND': 'TENDANCE',
  'TRIM': 'SUPPRESPACE',
  'TRIMMEAN': 'MOYENNE.REDUITE',
  'TRUE': 'VRAI',
  'TRUNC': 'TRONQUE',
  'TTEST': 'TEST.STUDENT',
  'TYPE': 'TYPE',
  'UNICHAR': 'UNICAR',
  'UNICODE': 'UNICODE',
  'UNIQUE': 'UNIQUE',
  'UPPER': 'MAJUSCULE',
  'VALUE': 'CNUM',
  'VALUETOTEXT': 'VALEURENTEXTE',
  'VAR': 'VAR',
  'VAR.P': 'VAR.P.N',
  'VAR.S': 'VAR.S',
  'VARA': 'VARA',
  'VARP': 'VAR.P',
  'VARPA': 'VARPA',
  'VDB': 'VDB',
  'VLOOKUP': 'RECHERCHEV',
  'WEEKDAY': 'JOURSEM',
  'WEEKNUM': 'NO.SEMAINE',
  'WORKDAY': 'SERIE.JOUR.OUVRE',
  'WORKDAY.INTL': 'SERIE.JOUR.OUVRE.INTL',
  'XIRR': 'TRI.PAIEMENTS',
  'XLOOKUP': 'RECHERCHEX',
  'XMATCH': 'EQUIVX',
  'XNPV': 'VAN.PAIEMENTS',
  'XOR': 'OUX',
  'YEAR': 'ANNEE',
  'YEARFRAC': 'FRACTION.ANNEE',
  'YIELD': 'RENDEMENT.TITRE',
  'YIELDDISC': 'RENDEMENT.SIMPLE',
};

/// The functions newer than the file format, which files call with the
/// prefix "_xlfn.", and those among them that take "_xlfn._xlws." as well.
const _newer = {
  'AGGREGATE',
  'ARABIC',
  'ARRAYTOTEXT',
  'BETA.DIST',
  'BINOM.DIST',
  'BITAND',
  'BITLSHIFT',
  'BITOR',
  'BITRSHIFT',
  'BITXOR',
  'CEILING.MATH',
  'CEILING.PRECISE',
  'CHISQ.DIST',
  'CHISQ.DIST.RT',
  'CHISQ.TEST',
  'COMBINA',
  'CONCAT',
  'CONFIDENCE.NORM',
  'CONFIDENCE.T',
  'COVARIANCE.P',
  'COVARIANCE.S',
  'DAYS',
  'EXPON.DIST',
  'F.DIST',
  'F.DIST.RT',
  'FLOOR.MATH',
  'FLOOR.PRECISE',
  'FORECAST.LINEAR',
  'FORMULATEXT',
  'GAMMA.DIST',
  'IFNA',
  'IFS',
  'ISO.CEILING',
  'ISOWEEKNUM',
  'LOGNORM.DIST',
  'MAXIFS',
  'MINIFS',
  'MODE.SNGL',
  'MUNIT',
  'NORM.DIST',
  'NORM.INV',
  'NORM.S.DIST',
  'NORM.S.INV',
  'NUMBERVALUE',
  'PERCENTILE.EXC',
  'PERCENTILE.INC',
  'PERCENTRANK.INC',
  'POISSON.DIST',
  'QUARTILE.EXC',
  'QUARTILE.INC',
  'RANDARRAY',
  'RANK.AVG',
  'RANK.EQ',
  'RRI',
  'SEQUENCE',
  'SHEET',
  'SHEETS',
  'SORTBY',
  'STDEV.P',
  'STDEV.S',
  'SWITCH',
  'T.DIST',
  'T.DIST.2T',
  'T.DIST.RT',
  'T.INV',
  'T.INV.2T',
  'T.TEST',
  'TEXTAFTER',
  'TEXTBEFORE',
  'TEXTJOIN',
  'UNICHAR',
  'UNICODE',
  'UNIQUE',
  'VALUETOTEXT',
  'VAR.P',
  'VAR.S',
  'XLOOKUP',
  'XMATCH',
  'XOR',
};

const _windowed = {'FILTER', 'SORT'};

const _errors = <String, String>{
  '#NULL!': '#NUL!',
  '#VALUE!': '#VALEUR!',
  '#NAME?': '#NOM?',
  '#NUM!': '#NOMBRE!',
  '#SPILL!': '#PROPAGATION!',
  '#CALC!': '#CALC!',
};

final _english = {for (final e in _functions.entries) e.value: e.key};
final _englishErrors = {for (final e in _errors.entries) e.value: e.key};

/// An error value as the French version shows it.
String errorText(String code) => _errors[code] ?? code;

/// The French names of the functions, by name, for suggestions.
Iterable<String> get frenchFunctions => _functions.values;

/// A formula as the French version of Excel shows it, from the way files
/// write it: French names, ";" between arguments, "," before decimals.
String formulaToFrench(String f) => _translate(f, toFrench: true);

/// A formula as files write it, from the way the French version shows it.
/// Names written in English are taken as well.
String formulaFromFrench(String f) => _translate(f, toFrench: false);

bool _letter(int c) => c >= 0x41 && c <= 0x5A || c >= 0x61 && c <= 0x7A || c == 0x5F || c == 0x5C || c > 0x7F;

final _spilled = RegExp(r'''\(((?:(?:'(?:[^']|'')+'|[^'"()!,;\s]+)!)?[^'"()!,;\s]+)\)''');

bool _digit(int c) => c >= 0x30 && c <= 0x39;

String _translate(String f, {required bool toFrench}) {
  final b = StringBuffer();
  var braces = 0;
  var operand = 0;
  var i = 0;
  while (i < f.length) {
    final c = f.codeUnitAt(i);
    // text and quoted sheet names are kept as they are
    if (c == 0x22 || c == 0x27) {
      operand = b.length;
      var j = i + 1;
      while (j < f.length) {
        if (f.codeUnitAt(j) == c) {
          if (j + 1 < f.length && f.codeUnitAt(j + 1) == c) {
            j += 2;
            continue;
          }
          break;
        }
        j++;
      }
      b.write(f.substring(i, (j + 1).clamp(0, f.length)));
      i = j + 1;
      continue;
    }
    if (c == 0x5B) {
      var depth = 0, j = i;
      for (; j < f.length; j++) {
        if (f[j] == '[') depth++;
        if (f[j] == ']' && --depth == 0) break;
      }
      b.write(f.substring(i, (j + 1).clamp(0, f.length)));
      i = j + 1;
      continue;
    }
    if (c == 0x23) {
      final rest = f.substring(i).toUpperCase();
      final table = toFrench ? _errors : _englishErrors;
      final hit = table.keys.where(rest.startsWith).firstOrNull;
      if (hit != null) {
        b.write(table[hit]);
        i += hit.length;
        continue;
      }
      b.write('#');
      i++;
      continue;
    }
    if (_digit(c) || c == 0x2E && i + 1 < f.length && _digit(f.codeUnitAt(i + 1)) && toFrench) {
      // a number, or a reference such as 1:3 or A1 read further on
      var j = i;
      while (j < f.length && (_digit(f.codeUnitAt(j)) || f[j] == (toFrench ? '.' : ','))) {
        j++;
      }
      if (j < f.length && (f[j] == 'E' || f[j] == 'e') && j + 1 < f.length && (f[j + 1] == '+' || f[j + 1] == '-' || _digit(f.codeUnitAt(j + 1)))) {
        j += 2;
        while (j < f.length && _digit(f.codeUnitAt(j))) {
          j++;
        }
      }
      if (j < f.length && _letter(f.codeUnitAt(j))) {
        // part of a name
        final k = _nameEnd(f, i);
        b.write(f.substring(i, k));
        i = k;
        continue;
      }
      final number = f.substring(i, j);
      b.write(toFrench ? number.replaceAll('.', ',') : number.replaceAll(',', '.'));
      i = j;
      continue;
    }
    if (_letter(c) || c == 0x24) {
      final k = _nameEnd(f, i);
      final word = f.substring(i, k);
      final call = k < f.length && f[k] == '(';
      final upper = word.toUpperCase();
      final spilled = toFrench && upper == '_XLFN.ANCHORARRAY' ? _spilled.matchAsPrefix(f, k) : null;
      if (spilled != null) {
        b
          ..write(spilled[1])
          ..write('#');
        i = k + spilled[0]!.length;
        continue;
      }
      if (i == 0 || f[i - 1] != '!') operand = b.length;
      if (!toFrench && !call && k < f.length && f[k] == '#' && !word.endsWith('!')) {
        final before = b.toString();
        b
          ..clear()
          ..write(before.substring(0, operand))
          ..write('_xlfn.ANCHORARRAY(')
          ..write(before.substring(operand))
          ..write('$word)');
        i = k + 1;
        continue;
      }
      if (call) {
        b.write(toFrench ? _toFrench(upper) : _fromFrench(upper, word));
      } else if (upper == 'TRUE' || upper == 'FALSE' || upper == 'VRAI' || upper == 'FAUX') {
        final isTrue = upper == 'TRUE' || upper == 'VRAI';
        b.write(toFrench ? (isTrue ? 'VRAI' : 'FAUX') : (isTrue ? 'TRUE' : 'FALSE'));
      } else {
        b.write(word);
      }
      i = k;
      continue;
    }
    switch (f[i]) {
      case '{':
        braces++;
        b.write('{');
      case '}':
        braces--;
        b.write('}');
      case ',' when toFrench:
        b.write(braces > 0 ? '.' : ';');
      case ';' when toFrench:
        b.write(';');
      case ';' when !toFrench:
        b.write(braces > 0 ? ';' : ',');
      case '.' when !toFrench && braces > 0:
        b.write(',');
      default:
        b.write(f[i]);
    }
    i++;
  }
  return b.toString();
}

/// Where the name that starts at [i] ends: letters, digits, "_", ".",
/// "\", "$", "!" and ":" of references.
int _nameEnd(String f, int i) {
  var k = i;
  while (k < f.length) {
    final c = f.codeUnitAt(k);
    if (_letter(c) || _digit(c) || c == 0x2E || c == 0x24 || c == 0x21 || c == 0x3F) {
      k++;
      continue;
    }
    break;
  }
  return k;
}

String _toFrench(String name) {
  var bare = name;
  for (final p in ['_XLFN._XLWS.', '_XLFN.', '_XLWS.']) {
    if (bare.startsWith(p)) bare = bare.substring(p.length);
  }
  return _functions[bare] ?? name;
}

String _fromFrench(String upper, String word) {
  final english = _english[upper] ?? (_functions.containsKey(upper) ? upper : null);
  if (english == null) return upper.startsWith('_XLFN.') ? word : upper;
  if (_windowed.contains(english)) return '_xlfn._xlws.$english';
  return _newer.contains(english) ? '_xlfn.$english' : english;
}
