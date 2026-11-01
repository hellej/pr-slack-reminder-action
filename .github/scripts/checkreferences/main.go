// Prints every broken reference in the files given as arguments. See AGENTS.md § References for the forms it checks.
// Run from the repository root by check-style.sh.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// A PR description may name a path the PR deletes.
	isPRDescription := flag.Bool("pr-description", false, "skip the repository path check")
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
	var findings []string
	for _, check := range []func() ([]string, error){
		func() ([]string, error) { return brokenLinks(paths, r) },
		func() ([]string, error) { return brokenSectionPointers(paths, r) },
		func() ([]string, error) {
			if *isPRDescription {
				return nil, nil
			}
			return brokenRepoPaths(paths, r)
		},
		func() ([]string, error) { return brokenSkillAndAgentNames(paths, r) },
		func() ([]string, error) { return brokenFrontmatterNames(paths) },
		func() ([]string, error) { return brokenMakeTargets(paths, isMakeTargetByName) },
	} {
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
