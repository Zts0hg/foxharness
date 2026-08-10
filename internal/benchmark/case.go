package benchmark

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Case defines a single benchmark scenario: the prompt to send the agent,
// the fixture directory to copy as the workspace, and the validations that
// determine whether the run succeeded.
type Case struct {
	ID             string       `yaml:"id"`
	Name           string       `yaml:"name"`
	Fixture        string       `yaml:"fixture"`
	Prompt         string       `yaml:"prompt"`
	MaxTurns       int          `yaml:"max_turns"`
	TimeoutSeconds int          `yaml:"timeout_seconds"`
	Validations    []Validation `yaml:"validations"`
}

// Validation specifies a single post-run check. The Type field selects the
// validation strategy: "command" runs a shell command and expects exit zero;
// "file_contains" asserts that a file in the workspace includes a substring.
type Validation struct {
	Type     string `yaml:"type"`
	Command  string `yaml:"command,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Contains string `yaml:"contains,omitempty"`
}

type caseDocument struct {
	ID             string               `yaml:"id"`
	Name           string               `yaml:"name"`
	Fixture        string               `yaml:"fixture"`
	Prompt         string               `yaml:"prompt"`
	MaxTurns       int                  `yaml:"max_turns"`
	TimeoutSeconds *int                 `yaml:"timeout_seconds"`
	Validations    []validationDocument `yaml:"validations"`
}

type validationDocument struct {
	Type     string  `yaml:"type"`
	Command  *string `yaml:"command"`
	Path     *string `yaml:"path"`
	Contains *string `yaml:"contains"`
}

type caseShape struct {
	MaxTurnsNull       bool
	TimeoutSecondsNull bool
	Validations        []validationShape
}

type validationShape struct {
	Command  bool
	Path     bool
	Contains bool
}

// LoadCase reads and parses a YAML benchmark case file. It validates that
// required fields (id, fixture, prompt) and at least one validation are
// present, defaulting MaxTurns to 12 when zero.
func LoadCase(path string) (*Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 benchmark case 失败: %w", err)
	}

	var document caseDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("解析 benchmark case 失败: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("只允许一个 YAML document")
		}
		return nil, fmt.Errorf("解析 benchmark case 失败: %w", err)
	}
	shape, err := inspectCaseShape(data)
	if err != nil {
		return nil, fmt.Errorf("解析 benchmark case 失败: %w", err)
	}

	id := strings.TrimSpace(document.ID)
	fixture := strings.TrimSpace(document.Fixture)
	if id == "" {
		return nil, fmt.Errorf("benchmark case id 不能为空")
	}
	if fixture == "" {
		return nil, fmt.Errorf("benchmark case fixture 不能为空")
	}
	if strings.TrimSpace(document.Prompt) == "" {
		return nil, fmt.Errorf("benchmark case prompt 不能为空")
	}
	if len(document.Validations) == 0 {
		return nil, fmt.Errorf("benchmark case 至少需要一条 validation")
	}
	if shape.MaxTurnsNull {
		return nil, fmt.Errorf("benchmark case max_turns 不能为 null")
	}
	if document.MaxTurns < 0 {
		return nil, fmt.Errorf("benchmark case max_turns 不能为负数")
	}
	maxTurns := document.MaxTurns
	if maxTurns == 0 {
		maxTurns = 12
	}
	timeoutSeconds := 600
	if document.TimeoutSeconds != nil || shape.TimeoutSecondsNull {
		if shape.TimeoutSecondsNull {
			return nil, fmt.Errorf("benchmark case timeout_seconds 不能为 null")
		}
		timeoutSeconds = *document.TimeoutSeconds
		if timeoutSeconds <= 0 || int64(timeoutSeconds) > int64(math.MaxInt64)/int64(time.Second) {
			return nil, fmt.Errorf("benchmark case timeout_seconds 必须是可表示的正整数时长")
		}
	}

	validations := make([]Validation, 0, len(document.Validations))
	for index, raw := range document.Validations {
		validation, err := validateDocument(index, raw, shape.Validations[index])
		if err != nil {
			return nil, err
		}
		validations = append(validations, validation)
	}

	if !filepath.IsAbs(fixture) {
		casePath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("解析 benchmark case fixture 路径失败: %w", err)
		}
		fixture = filepath.Join(filepath.Dir(casePath), fixture)
	}
	fixture = filepath.Clean(fixture)

	return &Case{
		ID:             id,
		Name:           document.Name,
		Fixture:        fixture,
		Prompt:         document.Prompt,
		MaxTurns:       maxTurns,
		TimeoutSeconds: timeoutSeconds,
		Validations:    validations,
	}, nil
}

func validateDocument(index int, raw validationDocument, shape validationShape) (Validation, error) {
	validationType := strings.TrimSpace(raw.Type)
	prefix := fmt.Sprintf("benchmark case validation[%d]", index+1)
	switch validationType {
	case "command":
		if raw.Command == nil || strings.TrimSpace(*raw.Command) == "" {
			return Validation{}, fmt.Errorf("%s command.command 不能为空", prefix)
		}
		if raw.Path != nil || shape.Path {
			return Validation{}, fmt.Errorf("%s command 不允许 path 字段", prefix)
		}
		if raw.Contains != nil || shape.Contains {
			return Validation{}, fmt.Errorf("%s command 不允许 contains 字段", prefix)
		}
		return Validation{Type: validationType, Command: *raw.Command}, nil
	case "file_contains":
		if raw.Path == nil || strings.TrimSpace(*raw.Path) == "" {
			return Validation{}, fmt.Errorf("%s file_contains.path 不能为空", prefix)
		}
		if raw.Contains == nil || strings.TrimSpace(*raw.Contains) == "" {
			return Validation{}, fmt.Errorf("%s file_contains.contains 不能为空", prefix)
		}
		if raw.Command != nil || shape.Command {
			return Validation{}, fmt.Errorf("%s file_contains 不允许 command 字段", prefix)
		}
		return Validation{Type: validationType, Path: strings.TrimSpace(*raw.Path), Contains: *raw.Contains}, nil
	default:
		return Validation{}, fmt.Errorf("%s type %q 未知", prefix, validationType)
	}
}

func inspectCaseShape(data []byte) (caseShape, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return caseShape{}, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return caseShape{}, fmt.Errorf("benchmark case 必须是 YAML mapping")
	}
	root := document.Content[0]
	shape := caseShape{}
	if node, ok := mappingValue(root, "max_turns"); ok {
		shape.MaxTurnsNull = node.Tag == "!!null"
	}
	if node, ok := mappingValue(root, "timeout_seconds"); ok {
		shape.TimeoutSecondsNull = node.Tag == "!!null"
	}
	validations, ok := mappingValue(root, "validations")
	if !ok || validations.Kind != yaml.SequenceNode {
		return shape, nil
	}
	shape.Validations = make([]validationShape, 0, len(validations.Content))
	for _, validation := range validations.Content {
		shape.Validations = append(shape.Validations, validationShape{
			Command:  mappingHasKey(validation, "command"),
			Path:     mappingHasKey(validation, "path"),
			Contains: mappingHasKey(validation, "contains"),
		})
	}
	return shape, nil
}

func mappingValue(mapping *yaml.Node, name string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == name {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

func mappingHasKey(mapping *yaml.Node, name string) bool {
	_, ok := mappingValue(mapping, name)
	return ok
}
