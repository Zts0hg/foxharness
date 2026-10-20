package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/mcpintegration"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
	"github.com/Zts0hg/foxharness/internal/tui"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCLIMCPCompositionCallsAndHydratesScreenshot(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	f := mcpfixture.Start(t, work)
	model := &mcpfixture.Model{}
	a, err := newCLIApplicationWithProvider(context.Background(), foxConfig{WorkDir: work, Model: "fixture", MaxTurns: 3, ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}}, model)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Run(context.Background(), app.RunCommand{Prompt: "capture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	images, _ := model.Counts()
	if images != 1 || f.Calls.Load() != 1 {
		t.Fatalf("images=%d calls=%d", images, f.Calls.Load())
	}
	if f.Active() != 0 {
		t.Fatal("CLI drain left an MCP session open")
	}
}
func TestAutodevMCPUsesItemWorkspace(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	f := mcpfixture.Start(t, work)
	model := &mcpfixture.Model{}
	factory := &autodevRuntimeCoreFactory{llmConfig: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}, maxTurns: 3, newProvider: func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }}
	core, err := factory.New(context.Background(), work, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close(context.Background())
	outcome := core.Run(context.Background(), autodev.CoreAttempt{AttemptID: "mcp", Prompt: "capture"}, nil)
	if outcome.Status != autodev.CoreOutcomeSucceeded {
		t.Fatalf("%#v %v", outcome, outcome.Cause)
	}
	images, _ := model.Counts()
	if images != 1 || f.Calls.Load() != 1 {
		t.Fatalf("images=%d calls=%d", images, f.Calls.Load())
	}
	if err := core.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.Active() != 0 {
		t.Fatal("Autodev close left an MCP session open")
	}
}

func TestTUIMCPDenialPreservesStateAcrossTurns(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	f := mcpfixture.Start(t, work)
	model := &mcpfixture.Model{}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), foxConfig{WorkDir: work, Model: "fixture", MaxTurns: 3, ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}}, tui.Interactions{Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{}}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())
	for range 2 {
		if _, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "capture"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	_, denied := model.Counts()
	if denied != 2 || f.Calls.Load() != 0 {
		t.Fatalf("permission bypass: denied=%d calls=%d", denied, f.Calls.Load())
	}
}

func TestAutodevProviderForwardsImageCapability(t *testing.T) {
	s := &autodevProviderState{config: llmconfig.ResolvedConfig{ImageInput: "disabled"}}
	if s.ImageInput() != "disabled" {
		t.Fatal("capability lost")
	}
}

type allowMCPPort struct{}

func (allowMCPPort) RequestPermission(_ context.Context, r app.PermissionRequest) (app.PermissionResponse, error) {
	return app.PermissionResponse{CorrelationID: r.Correlation.ID, Decision: app.PermissionAllowOnce}, nil
}
func TestTUIMCPReconnectAndNewSessionPreserveOwnership(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	f := mcpfixture.Start(t, work)
	model := &mcpfixture.Model{}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), foxConfig{WorkDir: work, Model: "fixture", MaxTurns: 3, ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}}, tui.Interactions{Permissions: allowMCPPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{}}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())
	for range 2 {
		if _, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "capture"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if f.Active() != 1 || f.Calls.Load() != 2 {
		t.Fatal("turns did not reuse owned MCP session")
	}
	if _, err := startup.MCP.ExecuteMCP(context.Background(), app.MCPCommand{Action: "reconnect", Name: "browser"}, nil); err != nil {
		t.Fatal(err)
	}
	status, err := startup.MCP.ExecuteMCP(context.Background(), app.MCPCommand{Action: "status"}, nil)
	if err != nil || status.Servers[0].Generation != 2 {
		t.Fatalf("reconnect: %#v %v", status, err)
	}
	if _, err := startup.Application.NewSession(context.Background(), app.NewSessionCommand{}); err != nil {
		t.Fatal(err)
	}
	if f.Active() != 1 {
		t.Fatal("session replacement leaked old owner")
	}
	status, _ = startup.MCP.ExecuteMCP(context.Background(), app.MCPCommand{Action: "status"}, nil)
	if status.Servers[0].Generation != 1 {
		t.Fatal("new session borrowed old host")
	}
}

func TestCLIActualProvidersReceiveMCPImagesAndTextFallback(t *testing.T) {
	for _, protocol := range []string{"openai", "claude"} {
		for _, mode := range []string{"enabled", "disabled"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				home, work := t.TempDir(), t.TempDir()
				t.Setenv("HOME", home)
				f := mcpfixture.Start(t, work)
				var requests atomic.Int32
				var imageSeen atomic.Bool
				endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, _ := io.ReadAll(r.Body)
					if strings.Contains(string(data), base64.StdEncoding.EncodeToString(f.Image)) {
						imageSeen.Store(true)
					}
					first := requests.Add(1) == 1
					w.Header().Set("Content-Type", "application/json")
					name := mcpintegration.ToolName("browser", "screenshot")
					if protocol == "openai" {
						if first {
							fmt.Fprintf(w, `{"id":"r","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"shot","type":"function","function":{"name":%q,"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, name)
						} else {
							io.WriteString(w, `{"id":"r","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"received"},"finish_reason":"stop"}]}`)
						}
					} else {
						if first {
							fmt.Fprintf(w, `{"id":"r","type":"message","role":"assistant","model":"fixture","content":[{"type":"tool_use","id":"shot","name":%q,"input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`, name)
						} else {
							io.WriteString(w, `{"id":"r","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"received"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
						}
					}
				}))
				defer endpoint.Close()
				cfg := llmconfig.ResolvedConfig{Protocol: protocol, BaseURL: endpoint.URL, Model: "fixture", Auth: "none", ImageInput: mode}
				p, err := provider.NewProvider(cfg)
				if err != nil {
					t.Fatal(err)
				}
				a, err := newCLIApplicationWithProvider(context.Background(), foxConfig{WorkDir: work, Model: "fixture", MaxTurns: 3, ResolvedLLM: cfg}, p)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Drain(context.Background())
				outcome, err := a.Run(context.Background(), app.RunCommand{Prompt: "capture"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if imageSeen.Load() != (mode == "enabled") || f.Calls.Load() != 1 {
					t.Fatalf("image=%v calls=%d", imageSeen.Load(), f.Calls.Load())
				}
				if mode == "disabled" && !strings.Contains(outcome.FinalMessage, "image") {
					t.Fatal("user did not receive visual limitation")
				}
				if strings.Contains(outcome.FinalMessage, base64.StdEncoding.EncodeToString(f.Image)) {
					t.Fatal("base64 leaked to presentation")
				}
			})
		}
	}
}
