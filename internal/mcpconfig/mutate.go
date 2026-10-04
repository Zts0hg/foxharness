package mcpconfig

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type document struct {
	fields  map[string]json.RawMessage
	servers map[string]json.RawMessage
}

func readDocument(file string) (document, error) {
	doc := document{fields: make(map[string]json.RawMessage), servers: make(map[string]json.RawMessage)}
	info, err := os.Stat(file)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, fmt.Errorf("cannot read configuration")
	}
	if !info.Mode().IsRegular() {
		return doc, fmt.Errorf("configuration is not a regular file")
	}
	f, err := os.OpenFile(file, configurationReadFlags, 0)
	if err != nil {
		return doc, fmt.Errorf("cannot read configuration")
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return doc, fmt.Errorf("configuration is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return doc, fmt.Errorf("configuration exceeds limit or cannot be read")
	}
	if err := json.Unmarshal(data, &doc.fields); err != nil || doc.fields == nil {
		return doc, fmt.Errorf("invalid JSON object")
	}
	if raw, ok := doc.fields["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &doc.servers); err != nil || doc.servers == nil {
			return doc, fmt.Errorf("mcpServers must be an object")
		}
	}
	return doc, nil
}

/* Add adds a new complete definition without overwriting an existing name. */
func Add(home, work string, scope Scope, name string, definition Definition) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("server name is required")
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return fmt.Errorf("invalid server definition")
	}
	var validation Definition
	if err := json.Unmarshal(encoded, &validation); err != nil {
		return fmt.Errorf("invalid server definition")
	}
	if err := resolve(&validation, func(ref string) string {
		if ref == "" {
			return ""
		}
		return "validation-placeholder"
	}); err != nil {
		return err
	}
	return mutate(home, work, scope, func(doc *document) error {
		if _, ok := doc.servers[name]; ok {
			return fmt.Errorf("server already exists in selected scope")
		}
		data, err := json.Marshal(definition)
		if err != nil {
			return err
		}
		doc.servers[name] = data
		return nil
	})
}

/* Remove deletes only the selected scope's definition. */
func Remove(home, work string, scope Scope, name string) error {
	return mutate(home, work, scope, func(doc *document) error {
		if _, ok := doc.servers[name]; !ok {
			return fmt.Errorf("server not found in selected scope")
		}
		delete(doc.servers, name)
		return nil
	})
}

/* SetEnabled edits only enablement and preserves unknown definition fields. */
func SetEnabled(home, work string, scope Scope, name string, enabled bool) error {
	return mutate(home, work, scope, func(doc *document) error {
		raw, ok := doc.servers[name]
		if !ok {
			return fmt.Errorf("server not found in selected scope")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return fmt.Errorf("invalid server definition")
		}
		fields["enabled"], _ = json.Marshal(enabled)
		doc.servers[name], _ = json.Marshal(fields)
		return nil
	})
}

func mutate(home, work string, scope Scope, change func(*document) error) error {
	dir := home
	switch scope {
	case UserScope:
	case ProjectScope:
		dir = work
	default:
		return fmt.Errorf("explicit user or project scope is required")
	}
	if dir == "" {
		return fmt.Errorf("scope directory is required")
	}
	dir = filepath.Join(dir, ".foxharness")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Mkdir("mcp.json.lock", 0700); err != nil {
		return fmt.Errorf("configuration locked; retry after other edit completes: %w", err)
	}
	defer root.Remove("mcp.json.lock")
	if info, err := root.Lstat("mcp.json"); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("configuration is not a regular file")
	}
	doc, err := readDocument(filepath.Join(dir, "mcp.json"))
	if err != nil {
		return err
	}
	if err := change(&doc); err != nil {
		return err
	}
	doc.fields["mcpServers"], err = json.Marshal(doc.servers)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc.fields, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := ".mcp-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	return root.Rename(tmp, "mcp.json")
}
