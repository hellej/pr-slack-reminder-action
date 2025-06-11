package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ActionYAML struct {
	Inputs map[string]interface{} `yaml:"inputs"`
}

func main() {
	workspaceDir := os.Getenv("GITHUB_WORKSPACE")
	if workspaceDir == "" {
		var err error
		workspaceDir, err = os.Getwd()
		if err != nil {
			log.Fatalf("Error getting current directory: %v", err)
		}
	}

	actionFile := filepath.Join(workspaceDir, "action.yml")
	configFile := filepath.Join(workspaceDir, "internal/config/config.go")

	actionInputs, err := getActionInputs(actionFile)
	if err != nil {
		log.Fatalf("Error getting inputs from %s: %v", actionFile, err)
	}

	configInputConstants, err := getConfigInputConstants(configFile)
	if err != nil {
		log.Fatalf("Error getting input constants from %s: %v", configFile, err)
	}

	var errorsFound bool

	var missingInConfig []string
	for inputName := range actionInputs {
		if _, exists := configInputConstants[inputName]; !exists {
			missingInConfig = append(missingInConfig, inputName)
		}
	}
	if len(missingInConfig) > 0 {
		sort.Strings(missingInConfig)
		fmt.Println("Error: The following inputs are defined in action.yml but their corresponding string constants were not found or correctly defined in internal/config/config.go:")
		for _, name := range missingInConfig {
			fmt.Printf("  - %s\n", name)
		}
		errorsFound = true
	}

	var missingInActionYML []string
	for constValue := range configInputConstants {
		if _, exists := actionInputs[constValue]; !exists {
			missingInActionYML = append(missingInActionYML, constValue)
		}
	}
	if len(missingInActionYML) > 0 {
		sort.Strings(missingInActionYML)
		fmt.Println("Error: The following input string constants from internal/config/config.go are not defined as inputs in action.yml:")
		for _, val := range missingInActionYML {
			fmt.Printf("  - %s\n", val)
		}
		errorsFound = true
	}

	if errorsFound {
		os.Exit(1)
	}

	fmt.Println("Input consistency check passed: action.yml and internal/config/config.go are aligned.")
}

func getActionInputs(filePath string) (map[string]bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var ay ActionYAML
	err = yaml.Unmarshal(data, &ay)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	inputNames := make(map[string]bool)
	if ay.Inputs != nil {
		for name := range ay.Inputs {
			inputNames[name] = true
		}
	}
	return inputNames, nil
}

func getConfigInputConstants(filePath string) (map[string]bool, error) {
	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, filePath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	constants := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		decl, ok := n.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			return true // Continue searching
		}

		for _, spec := range decl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, nameIdent := range valueSpec.Names {
				// We are looking for constants like: InputSomething = "something-input"
				// The constant name must start with "Input"
				if strings.HasPrefix(nameIdent.Name, "Input") {
					if len(valueSpec.Values) > i {
						val := valueSpec.Values[i]
						if basicLit, ok := val.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
							// Remove quotes from the string literal value
							value := strings.Trim(basicLit.Value, "\"")
							constants[value] = true
						}
					}
				}
			}
		}
		return true
	})
	return constants, nil
}
