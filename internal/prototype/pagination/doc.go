// Package pagination is a prototype that measures how closely Word's page
// breaks can be reproduced with free fonts drawn on the metrics of Office
// fonts (Carlito for Calibri, Caladea for Cambria, Liberation for Arial,
// Times New Roman and Courier New).
//
// The reference is what Word itself saved: the page count in
// docProps/app.xml, and the w:lastRenderedPageBreak markers it leaves where
// its last layout started a page. TestAgainstWord lays out every Word
// document of the corpus and scores, page by page, whether a page started
// where Word started it ends where Word ended it.
//
//	LOFFICE_FONTS=/usr/share/fonts/truetype go test ./internal/prototype/pagination -run AgainstWord -v
//
// It is written in Go to run on the corpus in CI; the editor lays out pages
// in Dart, where these rules will be ported.
package pagination
