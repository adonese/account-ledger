#!/usr/bin/env bash
# Reproduces the scaling table in DESIGN.md, section 1.
#
# Builds cmd/bench twice, each in a temporary directory:
#   before  the ledger at commit $BEFORE (default e51366a), when reassess
#           revisited every closed day of every account and every read
#           scanned the whole journal
#   after   the ledger in this checkout
# Every row replays the same seeded stream on each build. Wherever both builds
# finish a row their hashes must match: the journal and report are then
# byte-identical, or the journal alone on rows run with -report=false. A
# mismatch stops the script. The last step profiles one row on each build.
#
# Timings depend on the machine; line counts and hashes should not. A row that
# does not finish stops at DEADLINE (default 1m). A full run takes about three
# minutes.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
before=${BEFORE:-e51366a}
deadline=${DEADLINE:-1m}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir "$work/src-before"
git -C "$root" archive "$before" | tar -x -C "$work/src-before"
cp -R "$here" "$work/src-before/cmd/"
go build -C "$work/src-before" -o "$work/before" ./cmd/bench
go build -C "$root" -o "$work/after" ./cmd/bench

echo "$(go version) on $(sysctl -n machdep.cpu.brand_string 2>/dev/null || uname -m)"
echo "before = $before, after = this checkout, deadline $deadline"
echo

# row FLAGS BUILD:RUNS... runs each build with FLAGS and checks the hashes agree.
row() {
	local flags=$1 spec name runs line hash first=""
	shift
	for spec in "$@"; do
		name=${spec%%:*} runs=${spec##*:}
		# shellcheck disable=SC2086 # FLAGS is a list of flags
		line=$("$work/$name" $flags -runs "$runs" -deadline "$deadline") || true
		printf '  %-6s %s\n' "$name" "$line"
		[[ $line == *hash=* ]] || continue
		hash=${line##*hash=}
		if [[ -z $first ]]; then
			first=$hash
		elif [[ $hash != "$first" ]]; then
			echo "  MISMATCH: the builds disagree on this stream" >&2
			exit 1
		fi
	done
	echo
}

row "-brief" before:3 after:3
row "-events 10 -accounts 2" before:3 after:3
row "-events 1000 -accounts 2" before:3 after:3
row "-events 1000 -accounts 20" before:3 after:3
row "-events 1000 -accounts 200" before:3 after:3
row "-events 10000 -accounts 2" before:1 after:3
# before would take hours on the rows below: its cost grows with accounts x events^2.
row "-events 10000 -accounts 2000" after:3
row "-events 100000 -accounts 20000 -report=false" after:3
row "-events 100000 -accounts 2 -report=false" after:1

profile() { # profile BUILD FLAGS
	# shellcheck disable=SC2086 # FLAGS is a list of flags
	"$work/$1" $2 -report=false -cpuprofile "$work/$1.out" >/dev/null
	echo "CPU profile, $1, $2 (cum% of all samples):"
	go tool pprof -top -cum -nodecount=40 "$work/$1" "$work/$1.out" 2>/dev/null |
		grep -E 'ledger\.(Replay|\(\*Ledger\)\.(Apply|reassess|authorize)|View\.(Balance|Auths|Fee|Accrued))$' |
		sed 's/^/  /'
	echo
}
profile before "-events 1000 -accounts 200"
profile after "-events 10000 -accounts 2"
