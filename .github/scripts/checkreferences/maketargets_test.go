package main

import (
	"slices"
	"testing"
)

func TestMakefileTargets(t *testing.T) {
	makefile := "TEST=go test ./...\nCOMMIT := $(shell git rev-parse HEAD)\n.PHONY: test\n\n" +
		"test:\n\t$(TEST)\n\nlint: check-fmt check-vet\n\ncheck-fmt check-vet:\n\techo a: b\n"

	got := makefileTargets(makefile)

	want := []string{"test", "lint", "check-fmt", "check-vet"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMissingMakeTargets(t *testing.T) {
	isTargetByName := map[string]bool{"test": true, "check-fmt": true, "release-tag": true}
	markdown := "Run `make test`, `make check-fmt check-vett` and `make release-tag SEMVER=minor -j2`\n" +
		"Then `make`, `go test ./...` and `make testt`\n" +
		"```\nmake gone\n```"

	got := missingMakeTargets(markdown, markdownProse, isTargetByName)

	want := []nameMention{{line: 1, name: "check-vett", kind: "make target"}, {line: 2, name: "testt", kind: "make target"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
