package hermetic

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionHelpersContainNoAmbientOrExternalBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() did not return this test file")
	}
	directory := filepath.Dir(currentFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	banned := []string{
		"time.Sleep(",
		"time.Now(",
		"rand.",
		"net.Dial(",
		"net.Listen(",
		"http.DefaultClient",
		"http.Get(",
		"http.Post(",
		"os.UserHomeDir(",
		`os.Getenv("HOME")`,
		`os.LookupEnv("HOME")`,
		"GITHUB_TOKEN",
		"FEISHU_APP_SECRET",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}
		for _, token := range banned {
			if strings.Contains(string(data), token) {
				t.Errorf("%s contains forbidden hermetic-test dependency %q", entry.Name(), token)
			}
		}
	}
}
