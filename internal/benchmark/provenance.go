package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type fixtureIdentityEntry struct {
	Path          string `json:"path"`
	Type          string `json:"type"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
	LinkTarget    string `json:"link_target,omitempty"`
}

type caseIdentityDocument struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	Prompt         string                       `json:"prompt"`
	MaxTurns       int                          `json:"max_turns"`
	TimeoutSeconds int                          `json:"timeout_seconds"`
	Validations    []validationIdentityDocument `json:"validations"`
	FixtureID      string                       `json:"fixture_id"`
}

type validationIdentityDocument struct {
	Type     string `json:"type"`
	Command  string `json:"command,omitempty"`
	Path     string `json:"path,omitempty"`
	Contains string `json:"contains,omitempty"`
}

func fixtureTreeID(ctx context.Context, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("fixture root must be a directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer root.Close()

	entries := make([]fixtureIdentityEntry, 0)
	if err := collectFixtureIdentity(ctx, root, ".", &entries); err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return sha256JSON(entries)
}

func caseDefinitionID(c *Case, fixtureID string) (string, error) {
	validations := make([]validationIdentityDocument, 0, len(c.Validations))
	for _, validation := range c.Validations {
		validations = append(validations, validationIdentityDocument{
			Type: validation.Type, Command: validation.Command, Path: validation.Path, Contains: validation.Contains,
		})
	}
	document := caseIdentityDocument{
		ID:             c.ID,
		Name:           c.Name,
		Prompt:         c.Prompt,
		MaxTurns:       c.MaxTurns,
		TimeoutSeconds: c.TimeoutSeconds,
		Validations:    validations,
		FixtureID:      fixtureID,
	}
	return sha256JSON(document)
}

func collectFixtureIdentity(ctx context.Context, root *os.Root, relative string, entries *[]fixtureIdentityEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := root.Lstat(relative)
	if err != nil {
		return err
	}
	identity := fixtureIdentityEntry{Path: filepath.ToSlash(relative)}
	if info.Mode()&os.ModeSymlink != 0 {
		identity.Type = "symlink"
		identity.LinkTarget, err = root.Readlink(relative)
		if err != nil {
			return err
		}
		*entries = append(*entries, identity)
		return nil
	}
	if info.IsDir() {
		identity.Type = "directory"
		*entries = append(*entries, identity)
		directory, err := root.Open(relative)
		if err != nil {
			return err
		}
		openedInfo, statErr := directory.Stat()
		if statErr != nil || !openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
			_ = directory.Close()
			if statErr != nil {
				return statErr
			}
			return fmt.Errorf("fixture directory changed while hashing: %s", relative)
		}
		children, readErr := directory.ReadDir(-1)
		closeErr := directory.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if err := collectFixtureIdentity(ctx, root, filepath.Join(relative, child.Name()), entries); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		identity.Type = "unsupported:" + info.Mode().Type().String()
		*entries = append(*entries, identity)
		return nil
	}
	file, err := root.Open(relative)
	if err != nil {
		return err
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("fixture entry changed while hashing: %s", relative)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, &contextReader{ctx: ctx, reader: file})
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	identity.Type = "file"
	identity.ContentSHA256 = hex.EncodeToString(hash.Sum(nil))
	*entries = append(*entries, identity)
	return nil
}

func sha256JSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
