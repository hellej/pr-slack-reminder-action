---
mode: agent
---

# Remove Redundant Comments

Remove redundant comments from Go code by applying these principles:

## Comments to Remove
- **Function purpose comments** when the function name is self-explanatory
  - ❌ `// Save writes data to file` + `func Save(...)`
  - ✅ Just `func Save(...)` (rename the function to have more explicit name if needed)
- **Step-by-step comments** that restate what the code clearly shows (also in tests)
  - ❌ `// Marshal to JSON` + `json.Marshal(...)`
  - ✅ Use descriptive variable name: `jsonData, err := json.Marshal(...)` (extract well named helper functions if needed)
- **Variable declaration comments** when the name is descriptive
  - ❌ `// Create temp directory` + `tempDir := ...`
  - ✅ the descriptive variable name is enough

## Comments to Keep
- **Complex business logic** that isn't obvious from code alone
- **Special/incomplete code** that requires special attention soon
- **Non-obvious technical decisions** with important context
  - ✅ `// Preserves os.IsNotExist() behavior` (explains why we return err directly)
- **Gotchas or important side effects**
- **Algorithm explanations** for complex logic

## Refactoring Strategy
1. **Replace comment + generic name** with **descriptive name**:
   - `data` → `jsonData` (when it's specifically JSON)
   - `result` → `parsedConfig` (when it's a parsed configuration)
   - `current` → `workingDirectory` (when it's a working directory)

2. **Remove function comments** for simple CRUD operations:
   - `Save`, `Load`, `Create`, `Delete`, `Update` functions with obvious behavior

3. **Keep comments that explain "why"**, remove those that explain "what"

4. **Extract code that exists for one purpose as a well named function**, rather
than commenting the code block

## Example Transformation
```go
// Before
// Save writes a State to a JSON file at the specified path
func Save(path string, state State) error {
    // Marshal state to JSON
    data, err := json.MarshalIndent(state, "", "  ")
    // ... rest of function
}

// After  
func Save(path string, state State) error {
    jsonData, err := json.MarshalIndent(state, "", "  ")
    // ... rest of function
}
```

## Usage
Apply this refactoring to:
- New code before committing
- Existing code during reviews

The goal: Code that reads like well-written prose, not a step-by-step manual.
