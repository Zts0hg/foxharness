package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	sidebarMinTotalWidth = 120
	sidebarWidth         = 34
	sidebarGap           = 1
	sidebarDocumentCount = 3
	sidebarHintHeight    = 1
	sidebarHintText      = "Use /sidebar off to hide"
)

var sidebarFiles = []struct {
	title    string
	filename string
	empty    string
}{
	{title: "Memory", filename: "MEMORY.md", empty: "No memory"},
	{title: "Plan", filename: "PLAN.md", empty: "No active plan"},
	{title: "Todo", filename: "TODO.md", empty: "No todos"},
}

type sidebarDocument struct {
	Title   string
	Path    string
	Content string
	Missing bool
	Error   string
}

func loadSidebarDocuments(workDir string) []sidebarDocument {
	workDir = strings.TrimSpace(workDir)
	docs := make([]sidebarDocument, 0, len(sidebarFiles))
	for _, file := range sidebarFiles {
		doc := sidebarDocument{
			Title: file.title,
			Path:  file.filename,
		}
		if workDir == "" {
			doc.Missing = true
			doc.Content = file.empty
			docs = append(docs, doc)
			continue
		}

		path := filepath.Join(workDir, file.filename)
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
