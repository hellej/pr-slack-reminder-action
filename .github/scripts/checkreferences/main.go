// Prints every broken reference in the files given as arguments. See AGENTS.md § References for the forms it checks.
// Run from the repository root by check-style.sh.
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
	checks := []func() ([]string, error){
		func() ([]string, error) { return brokenLinks(paths, r) },
		func() ([]string, error) { return brokenSectionPointers(paths, r) },
		func() ([]string, error) { return brokenSkillAndAgentNames(paths, r) },
		func() ([]string, error) { return brokenFrontmatterNames(paths) },
		func() ([]string, error) { return brokenMakeTargets(paths, isMakeTargetByName) },
	}
	if *isPRDescription {
		checks = append(checks, func() ([]string, error) { return blobURLsOnOtherBranches(paths, r) })
	} else {
		checks = append(checks, func() ([]string, error) { return brokenRepoPaths(paths, r) })
	}
	var findings []string
	for _, check := range checks {
		checkFindings, err := check()
		if err != nil {
			exitWithError(err)
		}
		findings = append(findings, checkFindings...)
	}
	for _, finding := range findings {
		fmt.Println(finding)
	}
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
