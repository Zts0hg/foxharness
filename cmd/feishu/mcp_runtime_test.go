package main

import (
	"context"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
	"testing"
	"time"
)

func TestFeishuMCPRequiresRemoteApproval(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	f := mcpfixture.Start(t, work)
	model := &mcpfixture.Model{}
	factory := newFeishuTaskExecutionFactory(model, work, discardFeishuMessenger{}, session.NewFileStoreWithHome(work, home), approval.NewStore())
	prepared, err := factory.PrepareTask(context.Background(), feishu.TaskExecutionRequest{Task: feishu.Task{TaskID: "mcp", ChatID: "chat", SenderID: "user", MessageID: "message"}, Prompt: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Drain(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = prepared.Application.Run(ctx, app.RunCommand{Prompt: "capture"}, nil)
	if f.Calls.Load() != 0 {
		t.Fatal("cancelled remote task dispatched MCP")
	}
}

func TestRemoteMCPApprovalAndDenial(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprint(allow), func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			f := mcpfixture.Start(t, work)
			model := &mcpfixture.Model{}
			approvals := approval.NewStore()
			messenger := &mcpfixture.Messenger{Store: approvals, Approved: allow}
			factory := newFeishuTaskExecutionFactory(model, work, messenger, session.NewFileStoreWithHome(work, home), approvals)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			prepared, err := factory.PrepareTask(ctx, feishu.TaskExecutionRequest{Task: feishu.Task{TaskID: "mcp", ChatID: "chat", SenderID: "user", MessageID: "message"}, Prompt: "capture"})
			if err != nil {
				t.Fatal(err)
			}
			a := prepared.Application
			defer prepared.Drain(context.Background())
			if _, err := a.Run(ctx, app.RunCommand{Prompt: "capture"}, nil); err != nil {
				t.Fatal(err)
			}
			images, denied := model.Counts()
			if messenger.Requests.Load() != 1 {
				t.Fatalf("approval missing: %d", messenger.Requests.Load())
			}
			if allow {
				if images != 1 || f.Calls.Load() != 1 {
					t.Fatalf("images=%d calls=%d", images, f.Calls.Load())
				}
			} else if denied != 1 || f.Calls.Load() != 0 {
				t.Fatalf("denied=%d calls=%d", denied, f.Calls.Load())
			}
		})
	}
}
