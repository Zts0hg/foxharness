package keeprun

import (
	"strings"
	"testing"
)

func TestPipelinePhases(t *testing.T) {
	phases := PipelinePhases()

	wantCommands := []string{
		"codexspec:specify",
		"codexspec:clarify",
		"codexspec:generate-spec",
		"codexspec:review-spec",
		"codexspec:spec-to-plan",
		"codexspec:review-plan",
		"codexspec:plan-to-tasks",
		"codexspec:review-tasks",
		"codexspec:implement-tasks",
		"codexspec:review-code",
		"codexspec:commit-staged",
		"codexspec:pr",
	}

	if len(phases) != 12 {
		t.Fatalf("len(PipelinePhases()) = %d, want 12", len(phases))
	}

	reviewPhases := map[int]bool{4: true, 6: true, 8: true, 10: true} // 1-indexed

	for i, p := range phases {
		oneIdx := i + 1
		if p.Command != wantCommands[i] {
			t.Errorf("phase %d Command = %q, want %q", oneIdx, p.Command, wantCommands[i])
		}
		if !strings.HasPrefix(p.Command, "codexspec:") {
			t.Errorf("phase %d Command = %q, want codexspec: prefix", oneIdx, p.Command)
		}
		if strings.TrimSpace(p.Name) == "" {
			t.Errorf("phase %d (%s) has empty Name", oneIdx, p.Command)
		}
		if p.Review != reviewPhases[oneIdx] {
			t.Errorf("phase %d (%s) Review = %v, want %v", oneIdx, p.Command, p.Review, reviewPhases[oneIdx])
		}
		wantRemote := oneIdx == 12
		if p.Remote != wantRemote {
			t.Errorf("phase %d (%s) Remote = %v, want %v", oneIdx, p.Command, p.Remote, wantRemote)
		}
	}
}
