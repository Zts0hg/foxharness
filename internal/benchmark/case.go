package benchmark

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Case struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Fixture     string       `yaml:"fixture"`
	Prompt      string       `yaml:"prompt"`
	MaxTurns    int          `yaml:"max_turns"`
	Validations []Validation `yaml:"validations"`
}

type Validation struct {
	Type     string `yaml:"type"`
	Command  string `yaml:"command,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Contains string `yaml:"contains,omitempty"`
}

func LoadCase(path string) (*Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 benchmark case 失败: %w", err)
	}

	var c Case
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("解析 benchmark case 失败: %w", err)
	}

	if c.ID == "" || c.Fixture == "" || c.Prompt == "" {
		return nil, fmt.Errorf("benchmark case 缺少id、fixture或prompt")
	}

	if len(c.Validations) == 0 {
		return nil, fmt.Errorf("benchmark case 至少需要一条 validation")
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = 12
	}

	return &c, nil
}
