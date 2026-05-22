package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkTrackEdit(b *testing.B) {
	cp := New(Config{SessionDir: b.TempDir()}).(*FileCheckpointer)
	filePath := writeBenchmarkFile(b, b.TempDir(), "main.go", []byte("one"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cp.TrackEdit(filePath, "1")
	}
}

func BenchmarkMakeSnapshot(b *testing.B) {
	cp := New(Config{SessionDir: b.TempDir()}).(*FileCheckpointer)
	dir := b.TempDir()
	for i := 0; i < 10; i++ {
		filePath := writeBenchmarkFile(b, dir, string(rune('a'+i))+".txt", []byte("one"))
		_ = cp.TrackEdit(filePath, "1")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cp.MakeSnapshot(string(rune('a' + (i % 26))))
	}
}

func BenchmarkChangeDetection(b *testing.B) {
	cp := New(Config{SessionDir: b.TempDir()}).(*FileCheckpointer)
	filePath := writeBenchmarkFile(b, b.TempDir(), "main.go", []byte("one"))
	_ = cp.TrackEdit(filePath, "1")
	backup := cp.State().Snapshots[0].TrackedFileBackups[filePath]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cp.mu.Lock()
		_, _ = cp.fileChangedLocked(filePath, backup)
		cp.mu.Unlock()
	}
}

func BenchmarkGetDiffStats(b *testing.B) {
	cp := New(Config{SessionDir: b.TempDir()}).(*FileCheckpointer)
	filePath := writeBenchmarkFile(b, b.TempDir(), "main.go", []byte("one\n"))
	_ = cp.TrackEdit(filePath, "1")
	_ = os.WriteFile(filePath, []byte("one\ntwo\n"), 0644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cp.GetDiffStats("1")
	}
}

func writeBenchmarkFile(b *testing.B, dir, name string, data []byte) string {
	b.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		b.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		b.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
