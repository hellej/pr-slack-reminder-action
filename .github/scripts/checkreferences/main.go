// Prints every broken reference in the files given as arguments. See AGENTS.md § References for the forms it checks.
// check-style.sh runs it from the repository root. make check-pr-description runs it with -pr-description.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	isPRDescription := flag.Bool("pr-description", false, "skip the repository path check, and fail a blob URL on a branch other than main or the current one")
	flag.Parse()
	paths := flag.Args()
	r, err := loadRepo(".")
	if err != nil {
		exitWithError(err)
	}
	isMakeTargetByName, err := loadMakeTargets(".")
	if err != nil {
		exitWithError(err)
	}
	found, note, err := findings(paths, r, isMakeTargetByName, *isPRDescription)
	if err != nil {
		exitWithError(err)
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	for _, finding := range found {
		fmt.Println(finding)
	}
}

// The note goes to stderr: check-style.sh and make check-pr-description fail on any stdout.
func findings(paths []string, r repo, isMakeTargetByName map[string]bool, isPRDescription bool) (found []string, note string, err error) {
	checks := []func() ([]string, error){
		func() ([]string, error) { return brokenLinks(paths, r) },
		func() ([]string, error) { return brokenSectionPointers(paths, r) },
		func() ([]string, error) { return brokenSkillAndAgentNames(paths, r) },
		func() ([]string, error) { return brokenFrontmatterNames(paths) },
		func() ([]string, error) { return brokenMakeTargets(paths, isMakeTargetByName) },
	}
	if isPRDescription {
		checks = append(checks, func() ([]string, error) { return blobURLsOnOtherBranches(paths, r) })
	} else {
		checks = append(checks, func() ([]string, error) { return brokenRepoPaths(paths, r) })
	}
	for _, check := range checks {
		checkFindings, err := check()
		if err != nil {
			return nil, "", err
		}
		found = append(found, checkFindings...)
	}
	if isPRDescription && r.blobURLRoot == "" {
		note = "note: blob URLs not checked: origin is missing or not a recognised GitHub URL"
	}
	return found, note, nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
