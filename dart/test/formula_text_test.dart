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
    ]) {
      expect(formulaFromFrench(typed), file, reason: typed);
    }
  });

  test('round trip', () {
    for (final f in ['SUMIFS(C:C,A:A,">5",B:B,"x")', '_xlfn.IFS(A1>1,"a",TRUE,"b")', 'VLOOKUP(A2,Data!A:D,4,FALSE)']) {
      expect(formulaFromFrench(formulaToFrench(f)), f);
    }
  });
}
