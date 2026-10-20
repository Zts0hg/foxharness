package mcpintegration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/Zts0hg/foxharness/internal/mcpintegration"
	"github.com/Zts0hg/foxharness/internal/media"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPlaywrightAcceptance(t *testing.T) {
	if os.Getenv("FOX_MCP_PLAYWRIGHT") != "1" {
		t.Skip("not run: set FOX_MCP_PLAYWRIGHT=1; requires Node >=18 and Chrome")
	}
	if _, err := exec.LookPath("npx"); err != nil {
		t.Fatal("requested Playwright acceptance requires npx")
	}
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<!doctype html><html><body><div style="background:orange;width:220px;height:100px">VISUAL BADGE</div><label>Name<input aria-label="Name" id="name"></label><button onclick="document.getElementById('result').textContent='Hello '+document.getElementById('name').value">Greet</button><p id="result"></p></body></html>`)
	}))
	defer page.Close()
	home, work, sessionDir := t.TempDir(), t.TempDir(), t.TempDir()
	if err := mcpconfig.Add(home, work, mcpconfig.ProjectScope, "browser", mcpconfig.Definition{Transport: "stdio", Command: "npx", Args: []string{"-y", "@playwright/mcp@0.0.83", "--headless", "--isolated", "--image-responses", "allow"}, StartupTimeoutSeconds: 90, CallTimeoutSeconds: 30}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	host, err := mcpintegration.New(ctx, home, work, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())
	if s := host.Statuses(); len(s) != 1 || s[0].State != "ready" {
		t.Fatalf("Playwright unavailable: %#v", s)
	}
	registry := tools.NewRegistry()
	if err := mcpintegration.Mount(registry, host); err != nil {
		t.Fatal(err)
	}
	store := media.NewStore(sessionDir)
	var allowed []string
	for _, c := range host.Capabilities() {
		allowed = append(allowed, c.Name)
	}
	executor := toolruntime.New(registryexec.Capabilities(registry, allowed, nil), toolresult.OSFileSystem{}, sessionDir).WithImageStore(store)
	call := func(name string, args any) engine.ToolExecutionResult {
		t.Helper()
		raw, _ := json.Marshal(args)
		snapshot, err := executor.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		result, err := executor.Execute(ctx, snapshot, []schema.ToolCall{{ID: name, Name: mcpintegration.ToolName("browser", name), Arguments: raw}})
		if err != nil {
			t.Fatal(err)
		}
		r := result.Results[0]
		if r.IsError {
			t.Fatalf("%s: %s", name, r.FullContent)
		}
		return r
	}
	call("browser_navigate", map[string]any{"url": page.URL})
	first := call("browser_snapshot", map[string]any{})
	findRef := func(output, role string) string {
		t.Helper()
		matches := regexp.MustCompile(role + `[^\n]*\[ref=([^\]]+)\]`).FindStringSubmatch(output)
		if len(matches) != 2 {
			t.Fatalf("missing %s reference: %s", role, output)
		}
		return matches[1]
	}
	call("browser_type", map[string]any{"target": findRef(first.FullContent, `textbox "Name"`), "text": "Ada"})
	call("browser_click", map[string]any{"target": findRef(first.FullContent, `button "Greet"`)})
	state := call("browser_snapshot", map[string]any{})
	if !strings.Contains(state.FullContent, "Hello Ada") {
		t.Fatal("browser state did not survive consecutive calls")
	}
	childModel := &browserChildModel{}
	child := childruntime.New(childruntime.Config{Provider: childModel, WorkDir: work, HomeDir: home, ParentProfile: childruntime.CLIExec, Services: host, MaxTurns: 3})
	result, err := child.Run(ctx, subagent.Request{ParentSessionID: "playwright-parent", ParentRunID: "run", DelegationID: "child", Task: "read the current page", Depth: 1})
	if err != nil || result == nil || !strings.Contains(childModel.observed, "Hello Ada") {
		t.Fatalf("child did not inherit page: %v %q", err, childModel.observed)
	}
	if r := call("browser_snapshot", map[string]any{}); !strings.Contains(r.FullContent, "Hello Ada") {
		t.Fatal("child completion reset parent page")
	}
	shot := call("browser_take_screenshot", map[string]any{"type": "png"})
	var part schema.ContentPart
	for _, p := range shot.ContentParts {
		if p.Type == "image" {
			part = p
			break
		}
	}
	if part.ArtifactKey == "" {
		t.Fatalf("screenshot missing: %s", shot.FullContent)
	}
	image, err := store.Resolve(part)
	if err != nil || len(image) == 0 {
		t.Fatalf("screenshot replay: %v", err)
	}
	for _, protocol := range []string{"openai", "claude"} {
		for _, mode := range []string{"enabled", "disabled"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				var body string
				endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, _ := io.ReadAll(r.Body)
					body = string(data)
					w.Header().Set("Content-Type", "application/json")
					if protocol == "openai" {
						io.WriteString(w, `{"id":"r","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"received"},"finish_reason":"stop"}]}`)
					} else {
						io.WriteString(w, `{"id":"r","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"received"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					}
				}))
				defer endpoint.Close()
				p, err := provider.NewProvider(llmconfig.ResolvedConfig{Protocol: protocol, BaseURL: endpoint.URL, Model: "fixture", Auth: "none", ImageInput: mode})
				if err != nil {
					t.Fatal(err)
				}
				answer := invokeScreenshot(t, ctx, p, store, part)
				hasImage := strings.Contains(body, base64.StdEncoding.EncodeToString(image))
				if hasImage != (mode == "enabled") {
					t.Fatalf("actual screenshot encoded=%v mode=%s", hasImage, mode)
				}
				if mode == "disabled" && !strings.Contains(answer, "image") {
					t.Fatalf("visual limitation missing: %s", answer)
				}
				if _, err := store.Resolve(part); err != nil {
					t.Fatal("fallback removed user artifact")
				}
			})
		}
	}
	t.Run("live-model", func(t *testing.T) {
		if os.Getenv("FOX_MCP_LIVE_MODEL") == "" {
			t.Skip("not run: FOX_MCP_LIVE_MODEL and explicit endpoint/auth credentials were not provided")
		}
		cfg := llmconfig.ResolvedConfig{Protocol: os.Getenv("FOX_MCP_LIVE_PROTOCOL"), BaseURL: os.Getenv("FOX_MCP_LIVE_BASE_URL"), Model: os.Getenv("FOX_MCP_LIVE_MODEL"), Auth: llmconfig.AuthAPIKey, APIKey: os.Getenv("FOX_MCP_LIVE_API_KEY"), ImageInput: "enabled"}
		p, err := provider.NewProvider(cfg)
		if err != nil {
			t.Fatal(err)
		}
		answer := invokeScreenshot(t, ctx, p, store, part)
		if !strings.Contains(strings.ToLower(answer), "orange") {
			t.Fatalf("model did not identify visual badge color: %s", answer)
		}
		t.Logf("model %s (%s) screenshot analysis: %s", cfg.Model, cfg.Protocol, answer)
	})
	t.Log("Playwright MCP 0.0.83: navigate, snapshot, type, click, child continuity, screenshot persistence and both provider image encodings verified")
}
func invokeScreenshot(t *testing.T, ctx context.Context, p provider.LLMProvider, store *media.Store, part schema.ContentPart) string {
	t.Helper()
	run, err := modelinvoke.New(p, modelinvoke.Config{ResolveImage: store.Resolve}).StartRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Invoke(ctx, engine.RunContext{Messages: []schema.Message{{Role: schema.RoleUser, Content: "What is the background color of the VISUAL BADGE? Answer in English using only the image."}, {Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "shot", Name: "capture", Arguments: json.RawMessage(`{}`)}}}, {Role: schema.RoleUser, ToolCallID: "shot", Content: "Screenshot", ContentParts: []schema.ContentPart{part}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Message.Content
}

type browserChildModel struct{ observed string }

func (p *browserChildModel) Generate(_ context.Context, m []schema.Message, defs []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	for i := len(m) - 1; i >= 0; i-- {
		if m[i].ToolCallID != "" {
			p.observed = m[i].Content
			return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "page inspected"}}, nil
		}
	}
	name := mcpintegration.ToolName("browser", "browser_snapshot")
	for _, d := range defs {
		if d.Name == name {
			return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "child-snapshot", Name: name, Arguments: json.RawMessage(`{}`)}}}}, nil
		}
	}
	return nil, fmt.Errorf("authorized child cannot access browser_snapshot")
}
