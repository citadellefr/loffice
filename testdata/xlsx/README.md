The trees `xlsx` reads from test workbooks of
[Apache POI](https://poi.apache.org) (Apache License 2.0), which the Dart
package lays out on pages in its tests. Written by
`go test ./xlsx -run Fixtures -update` once the corpus is fetched.
`blank.xlsx` is an empty workbook the tests open.

`strict.xlsx` and `transitional.xlsx` hold the same content, written by hand
in Strict and in Transitional Open XML.
