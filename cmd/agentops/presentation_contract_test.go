package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/feishu"
)

func TestUIAOP001RequiredEnvironmentOrderingAndExit(t *testing.T) {
	keys := []string{
		"FEISHU_APP_ID",
		"FEISHU_APP_SECRET",
		"FEISHU_VERIFICATION_TOKEN",
		"FEISHU_ENCRYPT_KEY",
		"AGENTOPS_WORKDIR",
		"AGENTOPS_LOGDIR",
	}
	for missingIndex, wantKey := range keys {
		t.Run(wantKey, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestUIAOP001ProcessHelper$")
			env := append(os.Environ(), "FOX_UI_AGENTOPS_ENV_HELPER=1")
			for i, key := range keys {
				value := ""
				if i < missingIndex {
					value = "configured"
				}
				env = append(env, key+"="+value)
			}
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("process error = %v, want exit 1", err)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "missing environment variable: "+wantKey) {
				t.Fatalf("stdout/stderr = %q/%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUIAOP001ProcessHelper(t *testing.T) {
	if os.Getenv("FOX_UI_AGENTOPS_ENV_HELPER") != "1" {
		return
	}
	main()
}

func TestUIAOP001CompositionOrderConstantsAndListenFailure(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	ordered := []string{
		"newConfiguredLLMProvider(homeDir, os.Getenv)",
		"feishu.NewMessenger(appID, appSecret)",
		"approval.NewStore()",
		"newDeliveryStore(homeDir)",
		"make(chan feishu.Task, 64)",
		"feishu.NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore).WithDeliveryStore(deliveryStore)",
		"newAgentOpsTaskExecutionFactory(llmProvider, workDir, logDir, messenger, sessionStore, approvalStore)",
		"agentops.NewRunner(taskFactory, messenger)",
		`serve(signalCtx, gateway, runner, feishuTasks, ":7777", defaultShutdownTimeout)`,
	}
	position := -1
	for _, required := range ordered {
		index := strings.Index(text, required)
		if index < 0 || index <= position {
			t.Fatalf("composition root missing or misorders %q", required)
		}
		position = index
	}
	for _, required := range []string{
		"make(chan agentops.Task, 64)",
		`feishu.NewFileDeliveryStore(homeDir, filepath.Join(".foxharness", "feishu", "deliveries.json"))`,
		"WithDeliveryFailureObserver(agentops.NewLoggingDeliveryFailureObserver(log.Default()))",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("composition root missing %q", required)
		}
	}
	for _, removed := range []string{"NewDeduper", "deduper.Mark", "type Deduper"} {
		if strings.Contains(text, removed) {
			t.Fatalf("composition root restored superseded process-local acceptance authority %q", removed)
		}
	}

	wantErr := errors.New("listen failed")
	gateway := &uiAgentOpsImmediateGateway{listenErr: wantErr}
	runner := &uiAgentOpsDrainingRunner{}
	tasks := make(chan feishu.Task)
	if err := serve(context.Background(), gateway, runner, tasks, ":0", time.Second); !errors.Is(err, wantErr) {
		t.Fatalf("serve() error = %v, want listener failure", err)
	}
	if !runner.returned {
		t.Fatal("runner did not drain closed bridge after listener failure")
	}
}

func TestUIAOP002BridgeMapsEveryTypedFieldAndCloses(t *testing.T) {
	input := make(chan feishu.Task, 2)
	input <- feishu.Task{TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1", Text: "/new inspect"}
	input <- feishu.Task{TaskID: "task-2", ChatID: "chat-2", SenderID: "sender-2", MessageID: "message-2", Text: "new session"}
	close(input)
	output := make(chan agentops.Task)
	go bridgeAgentOpsTasks(input, output)

	var got []agentops.Task
	for task := range output {
		got = append(got, task)
	}
	want := []agentops.Task{
		{TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1", Text: "/new inspect", Query: "/new inspect"},
		{TaskID: "task-2", ChatID: "chat-2", SenderID: "sender-2", MessageID: "message-2", Text: "new session", Query: "new session"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bridged tasks = %#v, want %#v", got, want)
	}
}

func TestUIAOP003BridgePreservesBackpressureAndDeliveryOrder(t *testing.T) {
	input := make(chan feishu.Task)
	output := make(chan agentops.Task)
	go bridgeAgentOpsTasks(input, output)

	firstAccepted := make(chan struct{})
	go func() {
		input <- feishu.Task{TaskID: "first", MessageID: "message-1", Text: "first"}
		close(firstAccepted)
	}()
	<-firstAccepted

	secondAccepted := make(chan struct{})
	go func() {
		input <- feishu.Task{TaskID: "second", MessageID: "message-2", Text: "second"}
		close(secondAccepted)
		close(input)
	}()

	first := <-output
	<-secondAccepted
	second := <-output
	if first.TaskID != "first" || first.MessageID != "message-1" || second.TaskID != "second" || second.MessageID != "message-2" {
		t.Fatalf("bridge order = %#v, %#v", first, second)
	}
	if _, open := <-output; open {
		t.Fatal("bridge output remained open after input closure")
	}
}

type uiAgentOpsImmediateGateway struct{ listenErr error }

func (g *uiAgentOpsImmediateGateway) Listen(string) error          { return g.listenErr }
func (*uiAgentOpsImmediateGateway) Shutdown(context.Context) error { return nil }
func (*uiAgentOpsImmediateGateway) StopAccepting(context.Context) error {
	return nil
}

type uiAgentOpsDrainingRunner struct{ returned bool }

func (r *uiAgentOpsDrainingRunner) Start(_ context.Context, tasks <-chan agentops.Task) {
	for range tasks {
	}
	r.returned = true
}
