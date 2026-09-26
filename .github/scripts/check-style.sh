#!/usr/bin/env bash
# Checks the AGENTS.md rules that gofmt and go vet don't cover. Run from the repository root.
set -u

failed=0

report() {
	echo "$1"
	echo "$2" | sed 's/^/  /'
	echo
	failed=1
}

non_test_go_files() {
	find internal cmd -name '*.go' ! -name '*_test.go'
}

prose_files() {
	{
		echo AGENTS.md README.md docs/third-party-facts.md
		find .agents -name '*.md'
		# .claude/agents is a symlink to .agents/agents in this repo, already covered above.
		[ -L .claude/agents ] || find .claude/agents -name '*.md'
		# docs/plans/ is left out: plans are historical records.
		find . -name '*.spec.md' -not -path './docs/plans/*' | sed 's|^\./||'
	} | tr ' ' '\n' | sort -u
}

check_dashes() {
	local findings
	findings=$(
		{
			prose_files | xargs grep -nE '(^| )(—|–)( |$)'
			non_test_go_files | xargs grep -nE '//(.* )?(—|–)( |$)'
		} 2>/dev/null
	)
	if [ -n "$findings" ]; then
		report "Dash used as punctuation, see AGENTS.md § Output Style:" "$findings"
	fi
}

check_map_names() {
	local findings
	findings=$(
		non_test_go_files | xargs grep -nE '^[[:space:]]*(var[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*[[:space:]]*:?=[[:space:]]*make\(map\[' |
			grep -vE ':[[:space:]]*(var[[:space:]]+)?[a-z][A-Za-z0-9]*By[A-Z][A-Za-z0-9]*[[:space:]]*:?='
	)
	if [ -n "$findings" ]; then
		report "Map not named <value>By<Key>, see AGENTS.md § Code Style:" "$findings"
	fi
}

check_package_specs() {
	local dir expected_spec missing=""
	for dir in $(non_test_go_files | xargs -n1 dirname | sort -u); do
		expected_spec="$(basename "$dir").spec.md"
		# The binary's spec covers its run orchestration, see AGENTS.md § Package Specs.
		[ "$dir" = cmd/pr-slack-reminder ] && expected_spec=run.spec.md
		[ -f "$dir/$expected_spec" ] || missing="$missing$dir/$expected_spec"$'\n'
	done
	if [ -n "$missing" ]; then
		report "Missing package spec, see AGENTS.md § Package Specs:" "${missing%$'\n'}"
	fi
}

check_references() {
	local findings
	if ! findings=$({ prose_files; find internal cmd -name '*.go'; } | xargs go run ./.github/scripts/checkreferences 2>&1); then
		report "Reference check failed to run:" "$findings"
		return
	fi
	if [ -n "$findings" ]; then
		report "Broken reference, see AGENTS.md § Development Commands:" "$findings"
	fi
}

check_dashes
check_references
check_map_names
check_package_specs

if [ "$failed" -ne 0 ]; then
	exit 1
fi
