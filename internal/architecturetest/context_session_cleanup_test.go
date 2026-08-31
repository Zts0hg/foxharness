package architecturetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var legacySessionNames = map[string]bool{
	"Event": true, "Manager": true, "NewManager": true,
	"NewManagerWithHome": true, "NewTranscript": true, "Run": true,
	"Session": true, "Transcript": true,
}

func TestM26ContextFacadeIsDeletedRepositoryWide(t *testing.T) {
	root := moduleRoot(t)
	contextDir := filepath.Join(root, "internal", "context")
	if _, err := os.Stat(contextDir); !os.IsNotExist(err) {
		t.Errorf("temporary internal/context facade remains: %v", err)
	}
	assertNoPackageImport(t, root, modulePath+"/internal/context")
}

func TestM26SessionContainsNoCompatibilityVocabularyOrMemoryOwner(t *testing.T) {
	root := moduleRoot(t)
	sessionDir := filepath.Join(root, "internal", "session")
	paths, err := filepath.Glob(filepath.Join(sessionDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err == nil && importPath == modulePath+"/internal/memory" {
				t.Errorf("session remains a duplicate working-memory owner in %s", filepath.Base(path))
			}
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if legacySessionNames[value.Name.Name] || value.Name.Name == "MemoryPath" || value.Name.Name == "Finish" {
					t.Errorf("legacy session function or method remains in %s: %s", filepath.Base(path), value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && legacySessionNames[typeSpec.Name.Name] {
						t.Errorf("legacy session type remains in %s: %s", filepath.Base(path), typeSpec.Name.Name)
					}
				}
			}
		}
	}
}

func TestM26RepositoryHasNoLegacySessionConsumers(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := make(map[string]bool)
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != modulePath+"/internal/session" {
				continue
			}
			alias := "session"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				t.Errorf("legacy-sensitive session boundary is dot-imported by %s", path)
				continue
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !legacySessionNames[selector.Sel.Name] {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[identifier.Name] {
				t.Errorf("legacy session consumer remains in %s: %s.%s", path, identifier.Name, selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertNoPackageImport(t *testing.T, root, forbidden string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err == nil && importPath == forbidden {
				t.Errorf("forbidden package import remains in %s: %s", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
