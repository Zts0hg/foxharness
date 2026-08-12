package architecturetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIACHD006ChildInvocationAdaptersRemainHeadlessAndSinglePath(t *testing.T) {
	tests := []struct {
		path     string
		receiver string
		method   string
	}{
		{path: "internal/subagent/tool.go", receiver: "Tool", method: "Execute"},
		{path: "internal/app/runner.go", receiver: "subagentForkRunner", method: "Run"},
	}
	for _, test := range tests {
		t.Run(test.receiver+"."+test.method, func(t *testing.T) {
			file := parseGoFile(t, filepath.Join(moduleRoot(t), filepath.FromSlash(test.path)))
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{"/internal/tui", "/internal/feishu", "/internal/agentops", "/internal/benchmark"} {
					if strings.HasSuffix(path, forbidden) {
						t.Fatalf("adapter imports presentation/control package %q", path)
					}
				}
			}
			method := findMethod(t, file, test.receiver, test.method)
			runCalls := 0
			ast.Inspect(method.Body, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.CallExpr:
					selector, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if selector.Sel.Name == "Run" {
						runCalls++
					}
					for _, forbidden := range []string{"NewAgentEngine", "StartRun", "Create", "Open", "Print", "Printf", "Println"} {
						if selector.Sel.Name == forbidden {
							t.Errorf("adapter calls forbidden owner/output API %s", forbidden)
						}
					}
				case *ast.SelectorExpr:
					if identifier, ok := node.X.(*ast.Ident); ok && identifier.Name == "os" && node.Sel.Name == "Stdout" {
						t.Error("adapter accesses stdout")
					}
				}
				return true
			})
			if runCalls != 1 {
				t.Fatalf("adapter Manager.Run calls = %d, want exactly one", runCalls)
			}
		})
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func findMethod(t *testing.T, file *ast.File, receiver, method string) *ast.FuncDecl {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != method || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		receiverType := function.Recv.List[0].Type
		if pointer, ok := receiverType.(*ast.StarExpr); ok {
			receiverType = pointer.X
		}
		if identifier, ok := receiverType.(*ast.Ident); ok && identifier.Name == receiver {
			return function
		}
	}
	t.Fatalf("method %s.%s not found", receiver, method)
	return nil
}
