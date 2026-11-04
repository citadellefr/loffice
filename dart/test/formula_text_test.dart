import 'package:flutter_test/flutter_test.dart';
import 'package:loffice/src/excel/formula_text.dart';

void main() {
  test('formulas shown in French', () {
    for (final (file, shown) in [
      ('SUM(A1:B2,1.5)', 'SOMME(A1:B2;1,5)'),
      ('IF(A1>0,TRUE,FALSE)', 'SI(A1>0;VRAI;FAUX)'),
      ('_xlfn.XLOOKUP(A1,B:B,C:C)', 'RECHERCHEX(A1;B:B;C:C)'),
      ('TEXT(A1,"0.00, x")', 'TEXTE(A1;"0.00, x")'),
      ("'Q1, 24'!A1*2", "'Q1, 24'!A1*2"),
      ('SUM({1,2;3,4})', 'SOMME({1.2;3.4})'),
      ('IFERROR(A1,#VALUE!)', 'SIERREUR(A1;#VALEUR!)'),
      (r'Feuil1!$A$1+1.5E+3', r'Feuil1!$A$1+1,5E+3'),
      ('SUM(1:3)+.5', 'SOMME(1:3)+,5'),
      ('Taux*2+Table1[Montant]', 'Taux*2+Table1[Montant]'),
      ('NOSUCH(1,2)', 'NOSUCH(1;2)'),
      ('_xlfn._xlws.SORT(_xlfn.UNIQUE(A1:A9))', 'TRIER(UNIQUE(A1:A9))'),
      ('_xlfn._xlws.FILTER(A1:A3,B1:B3>0)', 'FILTRE(A1:A3;B1:B3>0)'),
      ('_xlfn.RANDARRAY(2,3)', 'TABLEAU.ALEA(2;3)'),
      ('_xlfn.SEQUENCE(5)', 'SEQUENCE(5)'),
      ('_xlfn.SORTBY(A1:A3,B1:B3,-1)', 'TRIERPAR(A1:A3;B1:B3;-1)'),
      ('IFERROR(A1,#SPILL!)', 'SIERREUR(A1;#PROPAGATION!)'),
      ('SUM(_xlfn.ANCHORARRAY(A1))', 'SOMME(A1#)'),
      (r'_xlfn.ANCHORARRAY(Feuil2!$B$2)', r'Feuil2!$B$2#'),
      ("_xlfn.ANCHORARRAY('Feuil 2'!B2)", "'Feuil 2'!B2#"),
      ('_xlfn.ANCHORARRAY(Taux)+1', 'Taux#+1'),
      ('_xlfn.CONCAT("A1#",Table1[#All])', 'CONCAT("A1#";Table1[#All])'),
      ('_xlfn.NORM.DIST(A1,0,1,TRUE)', 'LOI.NORMALE.N(A1;0;1;VRAI)'),
      ('NORMDIST(A1,0,1,TRUE)', 'LOI.NORMALE(A1;0;1;VRAI)'),
      ('_xlfn.T.TEST(A:A,B:B,2,1)', 'T.TEST(A:A;B:B;2;1)'),
      ('TTEST(A:A,B:B,2,1)', 'TEST.STUDENT(A:A;B:B;2;1)'),
      ('LINEST(A:A,B:B,TRUE,TRUE)', 'DROITEREG(A:A;B:B;VRAI;VRAI)'),
      ('_xlfn.MUNIT(3)', 'MATRICE.UNITAIRE(3)'),
      ('_xlfn.RRI(A1,A2,A3)', 'TAUX.INT.EQUIV(A1;A2;A3)'),
      ('NETWORKDAYS.INTL(A1,B1,11)', 'NB.JOURS.OUVRES.INTL(A1;B1;11)'),
      ('XIRR(A1:A5,B1:B5)', 'TRI.PAIEMENTS(A1:A5;B1:B5)'),
      ('MIRR(A1:A5,0.1,0.12)', 'TRIM(A1:A5;0,1;0,12)'),
      ('TRIM(A1)', 'SUPPRESPACE(A1)'),
    ]) {
      expect(formulaToFrench(file), shown, reason: file);
    }
  });

  test('formulas typed in French', () {
    for (final (typed, file) in [
      ('SOMME(A1;1,5)', 'SUM(A1,1.5)'),
      ('recherchex(1;A:A;B:B)', '_xlfn.XLOOKUP(1,A:A,B:B)'),
      ('SUM(1;2)', 'SUM(1,2)'),
      ('SOMME({1.2;3.4})', 'SUM({1,2;3,4})'),
      ('SI.NON.DISP(A1;0)', '_xlfn.IFNA(A1,0)'),
      ('SI(A1=#VALEUR!;VRAI;"a;b")', 'IF(A1=#VALUE!,TRUE,"a;b")'),
      (r'Feuil1!$A$1*2,5', r'Feuil1!$A$1*2.5'),
      ('TRIER(UNIQUE(A1:A9))', '_xlfn._xlws.SORT(_xlfn.UNIQUE(A1:A9))'),
      ('FILTRE(A1:A3;B1:B3>0)', '_xlfn._xlws.FILTER(A1:A3,B1:B3>0)'),
      ('tableau.alea(2;3)', '_xlfn.RANDARRAY(2,3)'),
      ('SEQUENCE(5)', '_xlfn.SEQUENCE(5)'),
      ('TRIERPAR(A1:A3;B1:B3;-1)', '_xlfn.SORTBY(A1:A3,B1:B3,-1)'),
      ('SI(A1=#PROPAGATION!;1;2)', 'IF(A1=#SPILL!,1,2)'),
      ('SOMME(A1#)', 'SUM(_xlfn.ANCHORARRAY(A1))'),
      (r'SOMME(A1:Feuil2!$B$2#)', r'SUM(A1:_xlfn.ANCHORARRAY(Feuil2!$B$2))'),
      ("SOMME('Feuil 2'!B2#)", "SUM(_xlfn.ANCHORARRAY('Feuil 2'!B2))"),
      ('SOMME(Taux#;Table1[#All])+#N/A', 'SUM(_xlfn.ANCHORARRAY(Taux),Table1[#All])+#N/A'),
      ('CONCAT("A1#";B1)', '_xlfn.CONCAT("A1#",B1)'),
      ('Feuil1!#REF!+1', 'Feuil1!#REF!+1'),
      ('loi.normale.n(1;0;1;VRAI)', '_xlfn.NORM.DIST(1,0,1,TRUE)'),
      ('PRODUITMAT(A1:B2;D1:E2)', 'MMULT(A1:B2,D1:E2)'),
      ('KURTOSIS(A1:A9)', 'KURT(A1:A9)'),
      ('prix.titre(A1;A2;A3;A4;100;2)', 'PRICE(A1,A2,A3,A4,100,2)'),
      ('taux.int.equiv(96;A1;A2)', '_xlfn.RRI(96,A1,A2)'),
      ('TRIM(A1:A5;0,1;0,12)', 'MIRR(A1:A5,0.1,0.12)'),
    ]) {
      expect(formulaFromFrench(typed), file, reason: typed);
    }
  });

  test('round trip', () {
    for (final f in ['SUMIFS(C:C,A:A,">5",B:B,"x")', '_xlfn.IFS(A1>1,"a",TRUE,"b")', 'VLOOKUP(A2,Data!A:D,4,FALSE)', '_xlfn.T.INV.2T(A1,5)', '_xlfn.CHISQ.TEST(A1:B2,D1:E2)']) {
      expect(formulaFromFrench(formulaToFrench(f)), f);
    }
  });
}
