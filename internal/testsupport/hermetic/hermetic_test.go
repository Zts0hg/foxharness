package hermetic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
)

func TestScriptedModelControlsStreamingFailuresSnapshotsAndBarrier(t *testing.T) {
	barrier := NewBarrier()
	model := NewScriptedModel([]ModelExchange{{
		Step: runtimecontract.ModelStep{
			Request: runtimecontract.ModelRequest{Model: "fixed-model", Provider: "fixed-provider"},
			Deltas:  []string{"hel", "lo"},
			Response: runtimecontract.ModelResponse{
				Content: "hello",
				Usage:   runtimecontract.Usage{InputTokens: 2, OutputTokens: 1, TotalTokens: 3},
			},
		},
		Barrier: barrier,
	}, {
		Step: runtimecontract.ModelStep{Error: "fallback failed"},
	}})

	type result struct {
		response runtimecontract.ModelResponse
		err      error
	}
	resultCh := make(chan result, 1)
	deltas := make([]string, 0, 2)
	request := runtimecontract.ModelRequest{Model: "fixed-model", Provider: "fixed-provider"}
	go func() {
		response, err := model.Invoke(context.Background(), request, func(delta string) {
			deltas = append(deltas, delta)
		})
		resultCh <- result{response: response, err: err}
	}()
	<-barrier.Started()
	barrier.Release()
	first := <-resultCh
	if first.err != nil || first.response.Content != "hello" {
		t.Fatalf("first Invoke() = %#v, %v", first.response, first.err)
	}
	if !reflect.DeepEqual(deltas, []string{"hel", "lo"}) {
		t.Fatalf("deltas = %#v", deltas)
	}
	if _, err := model.Invoke(context.Background(), runtimecontract.ModelRequest{}, nil); err == nil || err.Error() != "fallback failed" {
		t.Fatalf("second Invoke() error = %v", err)
	}
	requests := model.Requests()
	request.Model = "mutated"
	if len(requests) != 2 || requests[0].Model != "fixed-model" {
		t.Fatalf("Requests() = %#v", requests)
	}
}

func TestScriptedToolAndInteractionControlCorrelationAliasesAndCancellation(t *testing.T) {
	tools := NewScriptedTools([]ToolExchange{{
		Behavior: runtimecontract.ToolBehavior{
			Call:   runtimecontract.ToolCall{ID: "call-1", Name: "canonical", Arguments: `{}`},
			Result: runtimecontract.ToolResult{Output: strings.Repeat("x", 4096), ModelContent: "preview"},
		},
		Aliases:      []string{"alias"},
		ParallelSafe: true,
	}})
	result, err := tools.Execute(context.Background(), runtimecontract.ToolCall{ID: "call-1", Name: "alias", Arguments: `{}`})
	if err != nil || result.ModelContent != "preview" || len(result.Output) != 4096 {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}
	if !tools.ParallelSafe("alias") || tools.Calls()[0].Name != "alias" {
		t.Fatalf("tool snapshot = %#v", tools.Calls())
	}

	blocked := NewBarrier()
	cancelTools := NewScriptedTools([]ToolExchange{{
		Behavior: runtimecontract.ToolBehavior{Call: runtimecontract.ToolCall{ID: "cancel", Name: "wait"}},
		Barrier:  blocked,
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := cancelTools.Execute(ctx, runtimecontract.ToolCall{ID: "cancel", Name: "wait"})
		done <- err
	}()
	<-blocked.Started()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Execute() error = %v", err)
	}

	interactions := NewScriptedInteractions([]runtimecontract.InteractionReply{{
		Kind: "permission", CorrelationID: "permission-1", Value: "allow_once",
	}})
	reply, err := interactions.Reply(context.Background(), "permission", "permission-1")
	if err != nil || reply.Value != "allow_once" {
		t.Fatalf("Reply() = %#v, %v", reply, err)
	}
}

func TestClockIDsRootsAndFileSystemAreExplicitAndDeterministic(t *testing.T) {
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := NewSequenceClock(start, time.Second)
	ids := NewIDSequence("run-001", "run-002")
	if got := []time.Time{clock.Now(), clock.Now()}; !reflect.DeepEqual(got, []time.Time{start, start.Add(time.Second)}) {
		t.Fatalf("clock = %#v", got)
	}
	if first, _ := ids.Next(); first != "run-001" {
		t.Fatalf("first ID = %q", first)
	}

	roots, err := NewRoots(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoots() error = %v", err)
	}
	for _, path := range []string{roots.Home, roots.Config, roots.Sessions, roots.Workspace} {
		if !strings.HasPrefix(path, roots.Base+string(filepath.Separator)) {
			t.Fatalf("root %q escapes %q", path, roots.Base)
		}
	}
	env := roots.Env()
	if env["HOME"] != roots.Home || env["FOXHARNESS_CONFIG_DIR"] != roots.Config {
		t.Fatalf("Env() = %#v", env)
	}

	fs := NewFileSystem()
	if err := fs.WriteFile("state/data.txt", []byte("fixed"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := fs.ReadFile("state/data.txt")
	if err != nil || string(data) != "fixed" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	fs.Fail("rename", errors.New("rename denied"))
	if err := fs.Rename("state/data.txt", "state/new.txt"); err == nil || err.Error() != "rename denied" {
		t.Fatalf("Rename() error = %v", err)
	}
}

func TestProcessTransportMessengerCommandAndGitHubAreScripted(t *testing.T) {
	processes := NewProcessRunner([]ProcessExchange{{
		Request:  ProcessRequest{Name: "worker", Args: []string{"--once"}},
		Response: ProcessResponse{PID: 100, ChildPIDs: []int{101, 102}, Stdout: "done", ExitCode: 0},
	}})
	response, err := processes.Run(context.Background(), ProcessRequest{Name: "worker", Args: []string{"--once"}})
	if err != nil || response.Stdout != "done" || !reflect.DeepEqual(processes.Descendants(100), []int{101, 102}) {
		t.Fatalf("process response = %#v, descendants = %#v, err = %v", response, processes.Descendants(100), err)
	}

	transport := NewTransport([]HTTPExchange{{
		Method: http.MethodPost,
		URL:    "https://fake.invalid/hook",
		Status: http.StatusAccepted,
		Body:   "accepted",
	}})
	client := &http.Client{Transport: transport}
	httpResponse, err := client.Post("https://fake.invalid/hook", "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	body, _ := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	if string(body) != "accepted" || transport.Requests()[0].Body != "payload" {
		t.Fatalf("transport body = %q requests = %#v", body, transport.Requests())
	}

	messenger := NewMessenger([]MessageResult{{ID: "message-1"}})
	messageResult, err := messenger.Send(context.Background(), Message{Channel: "chat-1", Content: "hello"})
	if err != nil || messageResult.ID != "message-1" || messenger.Messages()[0].Content != "hello" {
		t.Fatalf("messenger = %#v, %v", messageResult, err)
	}

	commands := NewCommandRunner([]CommandExchange{{
		Request:  CommandRequest{Dir: "/repo", Name: "git", Args: []string{"status"}},
		Response: CommandResponse{Output: "clean"},
	}})
	commandResponse, err := commands.Run(context.Background(), CommandRequest{Dir: "/repo", Name: "git", Args: []string{"status"}})
	if err != nil || commandResponse.Output != "clean" {
		t.Fatalf("command = %#v, %v", commandResponse, err)
	}

	github := NewGitHub([]GitHubExchange{{
		Request:  GitHubRequest{Operation: "create_issue", Repository: "owner/repo", Payload: `{"title":"fixed"}`},
		Response: GitHubResponse{ID: "42", URL: "https://fake.invalid/issues/42"},
	}})
	githubResponse, err := github.Do(context.Background(), GitHubRequest{Operation: "create_issue", Repository: "owner/repo", Payload: `{"title":"fixed"}`})
	if err != nil || githubResponse.ID != "42" {
		t.Fatalf("GitHub Do() = %#v, %v", githubResponse, err)
	}
}

func TestLocalGitRepositoryUsesOnlyTestOwnedPaths(t *testing.T) {
	root := t.TempDir()
	repository, err := NewLocalGitRepository(context.Background(), root)
	if err != nil {
		t.Fatalf("NewLocalGitRepository() error = %v", err)
	}
	if !strings.HasPrefix(repository.Dir, root+string(filepath.Separator)) {
		t.Fatalf("repository %q escapes root %q", repository.Dir, root)
	}
	if err := os.WriteFile(filepath.Join(repository.Dir, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := repository.Git(context.Background(), "add", "README.md"); err != nil {
		t.Fatalf("git add error = %v", err)
	}
	if _, err := repository.Git(context.Background(), "commit", "-m", "fixture"); err != nil {
		t.Fatalf("git commit error = %v", err)
	}
	output, err := repository.Git(context.Background(), "log", "-1", "--format=%s")
	if err != nil || strings.TrimSpace(output) != "fixture" {
		t.Fatalf("git log = %q, %v", output, err)
	}
}
