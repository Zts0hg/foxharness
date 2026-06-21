package automemory

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MemoryType classifies a memory and determines both its storage scope and the
// body structure it must carry.
type MemoryType string

const (
	// TypeUser captures durable facts about the user (role, expertise,
	// preferences). User memories live in the user-global scope.
	TypeUser MemoryType = "user"
	// TypeFeedback captures guidance the user has given on how the agent should
	// work. Feedback memories require a Why and a How to apply explanation.
	TypeFeedback MemoryType = "feedback"
	// TypeProject captures ongoing work, goals, or constraints not derivable from
	// the code or git history. Project memories require a Why and a How to apply
	// explanation.
	TypeProject MemoryType = "project"
	// TypeReference captures pointers to external resources (URLs, dashboards,
	// tickets).
	TypeReference MemoryType = "reference"
)

// MaxContentChars bounds a memory body (frontmatter excluded), per CON-005.
const MaxContentChars = 40000

// Valid reports whether t is one of the four supported memory types.
func (t MemoryType) Valid() bool {
	switch t {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		return true
	default:
		return false
	}
}

// requiresWhyHow reports whether a memory of this type must include a Why and a
// How to apply section in its body.
func (t MemoryType) requiresWhyHow() bool {
	return t == TypeFeedback || t == TypeProject
}

// Memory is a single parsed memory file: its YAML frontmatter fields plus the
// Markdown body.
type Memory struct {
	// Name is the short kebab-case slug identifying the memory; it also derives
	// the on-disk filename.
	Name string
	// Description is the one-line relevance signal shown in the injected index.
	Description string
	// Type classifies the memory and selects its scope.
	Type MemoryType
	// Body is the Markdown content following the frontmatter.
	Body string
}

// frontmatter mirrors the YAML header of a memory file.
type frontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Type        MemoryType `yaml:"type"`
}

// ParseMemory decodes a memory file's bytes into a Memory. The file MUST open
// with a YAML frontmatter block delimited by lines containing only "---". The
// returned Memory is not validated; call Validate for that.
func ParseMemory(data []byte) (Memory, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Memory{}, errors.New("memory file is missing the opening frontmatter delimiter")
	}

	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Memory{}, errors.New("memory file is missing the closing frontmatter delimiter")
	}

	header := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return Memory{}, fmt.Errorf("failed to parse memory frontmatter: %w", err)
	}

	return Memory{
		Name:        strings.TrimSpace(fm.Name),
		Description: strings.TrimSpace(fm.Description),
		Type:        MemoryType(strings.TrimSpace(string(fm.Type))),
		Body:        strings.TrimRight(body, "\n"),
	}, nil
}

// Validate enforces the memory invariants: required frontmatter fields, a valid
// type, the Why/How-to-apply structure for feedback and project memories, and the
// content size cap (CON-005).
func (m Memory) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("memory name is required")
	}
	if strings.TrimSpace(m.Description) == "" {
		return errors.New("memory description is required")
	}
	if !m.Type.Valid() {
		return fmt.Errorf("invalid memory type %q (want user|feedback|project|reference)", m.Type)
	}
	if len(m.Body) > MaxContentChars {
		return fmt.Errorf("memory body has %d characters, exceeding the %d limit", len(m.Body), MaxContentChars)
	}
	if m.Type.requiresWhyHow() {
		lower := strings.ToLower(m.Body)
		if !strings.Contains(lower, "why") {
			return fmt.Errorf("%s memory must include a Why explanation", m.Type)
		}
		if !strings.Contains(lower, "how to apply") {
			return fmt.Errorf("%s memory must include a How to apply explanation", m.Type)
		}
	}
	return nil
}

// Marshal renders the memory back to its on-disk Markdown-with-frontmatter form.
func (m Memory) Marshal() ([]byte, error) {
	header, err := yaml.Marshal(frontmatter{
		Name:        m.Name,
		Description: m.Description,
		Type:        m.Type,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render memory frontmatter: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(header)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(m.Body, "\n"))
	b.WriteString("\n")
	return []byte(b.String()), nil
}
