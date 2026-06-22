package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	sidebarMinTotalWidth   = 110
	sidebarWidth           = 36
	sidebarMaxWidth        = 58
	sidebarGap             = 3
	sidebarDocumentCount   = 3
	sidebarHintHeight      = 1
	sidebarSeparatorHeight = 1
	sidebarHintText        = "/sidebar off to hide"
)

var sidebarFiles = []struct {
	title    string
	filename string
	empty    string
	session  bool
}{
	{title: "Memory", filename: "MEMORY.md", empty: "No memories yet"},
	{title: "Plan", filename: "PLAN.md", empty: "No active plan", session: true},
	{title: "Todo", filename: "TODO.md", empty: "No todos", session: true},
}

type sidebarDocument struct {
	Title   string
	Path    string
	Content string
	Missing bool
	Error   string
}

// loadSidebarDocuments builds the sidebar panels. The Memory panel reflects the
// cross-session persistent memory via memoryIndex (the merged two-tier index);
// it no longer reads the legacy <workDir>/MEMORY.md, which is orphaned now that
// persistent memory lives under ~/.foxharness. Plan and Todo remain
// session-scoped files.
func loadSidebarDocuments(workDir string, sessionDir string, memoryIndex string) []sidebarDocument {
	workDir = strings.TrimSpace(workDir)
	sessionDir = strings.TrimSpace(sessionDir)
	memoryIndex = strings.TrimSpace(memoryIndex)
	docs := make([]sidebarDocument, 0, len(sidebarFiles))
	for _, file := range sidebarFiles {
		doc := sidebarDocument{
			Title: file.title,
			Path:  file.filename,
		}
		if file.title == "Memory" {
			doc.Path = "memory index"
			if memoryIndex != "" {
				doc.Content = memoryIndex
			} else {
				doc.Missing = true
				doc.Content = file.empty
			}
			docs = append(docs, doc)
			continue
		}
		baseDir := workDir
		if file.session {
			baseDir = sessionDir
		}
		if baseDir == "" {
			doc.Missing = true
			doc.Content = file.empty
			docs = append(docs, doc)
			continue
		}

		path := filepath.Join(baseDir, file.filename)
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			doc.Content = strings.TrimSpace(string(data))
			if doc.Content == "" {
				doc.Content = file.empty
			}
		case errors.Is(err, os.ErrNotExist):
			doc.Missing = true
			doc.Content = file.empty
		default:
			doc.Error = err.Error()
			doc.Content = "Unable to read " + file.filename
		}
		docs = append(docs, doc)
	}
	return docs
}

func shouldRenderSidebar(width int) bool {
	return width >= sidebarMinTotalWidth
}
