#!/bin/sh
# Writes what the corpus tests rewrite and edit, then validates it with the
# Open XML SDK, as CI does. Without dotnet in the PATH, nix provides it.
#
#   tools/validate/run.sh [work directory [family...]]
#
# The families are opc, pptx, xlsx, legacy and docx, all by default.
set -eu
cd "$(dirname "$0")/../.."
work=${1:-$(mktemp -d)}
[ $# -gt 0 ] && shift
families=${*:-opc pptx xlsx legacy docx}
[ -d corpus/files ] || go run ./tools/corpus

if ! command -v dotnet >/dev/null; then
	exec nix shell nixpkgs#dotnet-sdk_8 -c "$0" "$work" $families
fi

status=0
rewrite() { # directory, package, test
	rm -rf "$work/$1"
	LOFFICE_OUT=$work/$1 go test -count=1 "./$2" -run "$3"
}
edit() { # directory, package, test, environment variable
	rm -rf "$work/$1"
	env "$4=$work/$1" go test -count=1 "./$2" -run "$3"
}
validate() { # originals, directory, edited
	echo "== $2"
	LOFFICE_EDITED=${3:-} dotnet run --project tools/validate -c Release -- "$1" "$work/$2" || status=1
}

for family in $families; do
	case $family in
	opc)
		rewrite opc opc Corpus
		validate corpus/files opc
		;;
	pptx)
		rewrite pptx pptx Corpus
		validate corpus/files pptx
		edit pptx-edited pptx Edits LOFFICE_EDITED
		validate corpus/files pptx-edited 1
		;;
	xlsx)
		rewrite xlsx xlsx CorpusRewrite
		validate corpus/files xlsx
		edit xlsx-edited xlsx CorpusEdits LOFFICE_EDITED
		validate corpus/files xlsx-edited 1
		edit xlsx-moved xlsx CorpusMoves LOFFICE_MOVED
		validate corpus/files xlsx-moved 1
		;;
	legacy)
		rewrite xls legacy CorpusWorkbooks
		validate "$work/xls/blank" xls/converted 1
		;;
	docx)
		rewrite docx docx CorpusRewrite
		validate corpus/files docx
		edit docx-edited docx CorpusEdit LOFFICE_EDITED
		validate corpus/files docx-edited 1
		;;
	*)
		echo "unknown family $family" >&2
		exit 2
		;;
	esac
done
exit $status
