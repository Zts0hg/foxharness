package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	workDir string
}

func NewStore(workDir string) *Store {
	return &Store{workDir: workDir}
}

func (s *Store) PlanPath() string {
	return filepath.Join(s.workDir, "PLAN.md")
}

func (s *Store) TodoPath() string {
	return filepath.Join(s.workDir, "TODO.md")
}

func (s *Store) MemoryPath() string {
	return filepath.Join(s.workDir, "MEMORY.md")
}

func (s *Store) EnsureFiles() error {
	files := map[string]string{
		s.PlanPath():   planTemplate(),
		s.TodoPath():   todoTemplate(),
		s.MemoryPath(): memoryTemplate(),
	}

	for path, content := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("初始化记忆文件失败 %s: %w", path, err)
			} else if err != nil {
				return fmt.Errorf("检查记忆文件失败 %s: %w", path, err)
			}
		}
	}

	return nil
}

func planTemplate() string {
	return "# PLAN\n\n## Goal\n\n未记录。\n\n## Strategy\n\n未记录。\n\n## Verification\n\n未记录。\n"
}

func todoTemplate() string {
	return "# TODO\n\n- [] 未记录。\n"
}

func memoryTemplate() string {
	return "# MEMORY\n\n- 未记录。\n"
}

type Bundle struct {
	Plan   string
	Todo   string
	Memory string
}

func (s *Store) Load() (*Bundle, error) {
	plan, err := readOptional(s.PlanPath())
	if err != nil {
		return nil, err
	}
	todo, err := readOptional(s.TodoPath())
	if err != nil {
		return nil, err
	}
	mem, err := readOptional(s.MemoryPath())
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Plan:   plan,
		Todo:   todo,
		Memory: mem,
	}, nil
}

func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取文件失败 %s: %w", path, err)
	}
	return string(data), nil
}
