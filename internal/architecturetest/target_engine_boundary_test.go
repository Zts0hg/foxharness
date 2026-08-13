package architecturetest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestTargetEngineFilesDependOnlyOnStandardLibraryAndSchema(t *testing.T) {
	files := targetEngineFiles(t)
	if len(files) == 0 {
		t.Fatal("target engine production files = 0")
	}
	allowed := map[string]struct{}{
		"context": {}, "encoding/json": {}, "errors": {}, "fmt": {},
		modulePath + "/internal/schema": {},
	}
	for path, file := range files {
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if _, ok := allowed[importPath]; !ok {
				t.Errorf("target engine file %s imports package outside exact M08 allowset: %s", path, importPath)
			}
		}
	}
}

func TestTargetAgentEngineRetainsOnlyConfirmedCollaborators(t *testing.T) {
	files := targetEngineFiles(t)
	got := map[string]string{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || declaration.Name.Name != "AgentEngine" {
				return true
			}
			structure, ok := declaration.Type.(*ast.StructType)
			if !ok {
				t.Fatal("AgentEngine is not a struct")
			}
			for _, field := range structure.Fields.List {
				if len(field.Names) != 1 {
					t.Fatalf("AgentEngine field has %d names, want one", len(field.Names))
				}
				identifier, ok := field.Type.(*ast.Ident)
				if !ok {
					t.Fatalf("AgentEngine.%s type is %T, want collaborator interface", field.Names[0].Name, field.Type)
				}
				got[field.Names[0].Name] = identifier.Name
			}
			return false
		})
	}
	want := map[string]string{
		"model": "ModelInvoker", "tools": "ToolExecutor", "conversation": "Conversation",
		"policy": "TurnPolicy", "observer": "Observer",
	}
	if fmt.Sprint(sortedStringMap(got)) != fmt.Sprint(sortedStringMap(want)) {
		t.Fatalf("AgentEngine fields = %v, want exact immutable collaborators %v", sortedStringMap(got), sortedStringMap(want))
	}
}

func TestTargetEngineFilesDoNotUseLegacySamePackageContracts(t *testing.T) {
	forbidden := map[string]struct{}{
		"Config": {}, "DetailedReporter": {}, "LegacyEngine": {}, "Reporter": {},
		"RunResult": {}, "TurnLimitError": {},
	}
	var violations []string
	for path, file := range targetEngineFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, found := forbidden[identifier.Name]; found {
				violations = append(violations, filepath.Base(path)+":"+identifier.Name)
			}
			return true
		})
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("target engine uses legacy same-package contracts: %v", violations)
	}
}

func TestTargetEngineDoesNotWritePresentationOutput(t *testing.T) {
	forbiddenSelectors := map[string]struct{}{
		"fmt.Print": {}, "fmt.Printf": {}, "fmt.Println": {},
	}
	var violations []string
	for path, file := range targetEngineFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := expressionName(call.Fun)
			if name == "print" || name == "println" {
				violations = append(violations, filepath.Base(path)+":"+name)
			}
			if _, forbidden := forbiddenSelectors[name]; forbidden {
				violations = append(violations, filepath.Base(path)+":"+name)
			}
			return true
		})
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("target engine writes presentation output: %v", violations)
	}
}

func TestTargetConversationRequestsChangesWithoutOwningMutation(t *testing.T) {
	files := targetEngineFiles(t)
	var methods []string
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || declaration.Name.Name != "Conversation" {
				return true
			}
			contract, ok := declaration.Type.(*ast.InterfaceType)
			if !ok {
				t.Fatal("Conversation is not an interface")
			}
			for _, method := range contract.Methods.List {
				if len(method.Names) == 1 {
					methods = append(methods, method.Names[0].Name)
				}
			}
			return false
		})
	}
	sort.Strings(methods)
	if got, want := fmt.Sprint(methods), fmt.Sprint([]string{"Prepare", "RequestChanges"}); got != want {
		t.Fatalf("Conversation methods = %s, want %s", got, want)
	}
}

func TestTargetRunOutcomeContainsOnlyExecutionResultFields(t *testing.T) {
	got := targetStructFields(t, "RunOutcome")
	want := map[string]string{
		"Err": "error", "ErrorKind": "string", "FinalMessage": "string",
		"FinishReason": "string", "Partial": "bool", "TurnCount": "int", "Usage": "schema.Usage",
	}
	if fmt.Sprint(sortedStringMap(got)) != fmt.Sprint(sortedStringMap(want)) {
		t.Fatalf("RunOutcome fields = %v, want runtime-neutral execution fields %v", sortedStringMap(got), sortedStringMap(want))
	}
}

func targetEngineFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	root := moduleRoot(t)
	engineDir := filepath.Join(root, "internal", "engine")
	legacyFiles := map[string]struct{}{
		"config.go": {}, "context.go": {}, "errors.go": {},
		"loop.go": {}, "reporter.go": {}, "todo_gate.go": {},
	}
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, engineDir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse engine package: %v", err)
	}
	result := map[string]*ast.File{}
	for _, parsedPackage := range packages {
		for path, file := range parsedPackage.Files {
			_, legacy := legacyFiles[filepath.Base(path)]
			if !legacy || declaresTargetEngine(file) {
				result[path] = file
			}
		}
	}
	return result
}

func targetStructFields(t *testing.T, typeName string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, file := range targetEngineFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || declaration.Name.Name != typeName {
				return true
			}
			structure, ok := declaration.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s is not a struct", typeName)
			}
			for _, field := range structure.Fields.List {
				if len(field.Names) != 1 {
					t.Fatalf("%s field has %d names, want one", typeName, len(field.Names))
				}
				result[field.Names[0].Name] = expressionName(field.Type)
			}
			return false
		})
	}
	if len(result) == 0 {
		t.Fatalf("%s declaration not found in target engine files", typeName)
	}
	return result
}

func declaresTargetEngine(file *ast.File) bool {
	targetTypes := map[string]struct{}{
		"AgentEngine": {}, "Conversation": {}, "Fact": {}, "ModelInvoker": {},
		"Observer": {}, "RunContext": {}, "RunInput": {}, "RunOutcome": {},
		"ToolExecutor": {}, "TurnPolicy": {}, "TurnRunPolicy": {},
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if _, target := targetTypes[typeSpec.Name.Name]; target {
						return true
					}
				}
			}
		case *ast.FuncDecl:
			if declaration.Recv != nil && len(declaration.Recv.List) == 1 && receiverName(declaration.Recv.List[0].Type) == "AgentEngine" {
				return true
			}
		}
	}
	return false
}

func receiverName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverName(expression.X)
	default:
		return ""
	}
}

func expressionName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expressionName(expression.X) + "." + expression.Sel.Name
	case *ast.StarExpr:
		return "*" + expressionName(expression.X)
	case *ast.ArrayType:
		return "[]" + expressionName(expression.Elt)
	default:
		return fmt.Sprintf("%T", expression)
	}
}

func sortedStringMap(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}
