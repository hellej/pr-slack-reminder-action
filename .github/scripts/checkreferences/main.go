// Prints every broken reference in the files given as arguments. See AGENTS.md § References for the forms it checks.
// Run from the repository root by check-style.sh.
package main

import (
	"fmt"
	"os"
)

func main() {
	paths := os.Args[1:]
	r, err := loadRepo(".")
	if err != nil {
		exitWithError(err)
	}
	var findings []string
	for _, check := range []func() ([]string, error){
		func() ([]string, error) { return brokenLinks(paths) },
		func() ([]string, error) { return brokenSectionPointers(paths, r) },
		func() ([]string, error) { return brokenRepoPaths(paths, r) },
		func() ([]string, error) { return brokenSkillAndAgentNames(paths, r) },
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
