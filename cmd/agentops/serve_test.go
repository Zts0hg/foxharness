package main

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/feishu"
)

type probeGateway struct{}

func (probeGateway) Listen(string) error                 { return nil }
func (probeGateway) StopAccepting(context.Context) error { return nil }
func (probeGateway) Shutdown(context.Context) error      { return nil }

// cancellingRunner models a runner that stops consuming when its context is
// cancelled, which the runnerService contract permits: Start returns and the
// queued tasks the runner never consumed are the runner's to decline.
type cancellingRunner struct {
	mu       sync.Mutex
	notified []string
}

func (r *cancellingRunner) Start(ctx context.Context, _ <-chan agentops.Task) {
	<-ctx.Done()
}

func (r *cancellingRunner) NotifyCancellation(task agentops.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notified = append(r.notified, task.TaskID)
}

func (r *cancellingRunner) notifiedTaskIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.notified...)
}

// Probe: when the AgentOps bridge blocks on a full agentTasks channel after
// the runner stopped consuming, graceful shutdown must still complete within
// its budget instead of hanging until the deadline and reporting failure.
// Every task the bridge cannot deliver reaches exactly one cancellation
// terminal, and no task is notified twice.
func TestServeShutdownDoesNotHangWhenRunnerStopsConsuming(t *testing.T) {
	const totalTasks = 100
	feishuTasks := make(chan feishu.Task, totalTasks)
	taskIDs := make(map[string]bool, totalTasks)
	for i := 0; i < totalTasks; i++ {
		id := fmtTaskID(i)
		taskIDs[id] = true
		feishuTasks <- feishu.Task{TaskID: id, ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "task"}
	}
	runner := &cancellingRunner{}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	defer cancelSignal()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, probeGateway{}, runner, feishuTasks, ":0", 100*time.Millisecond)
	}()
	time.Sleep(20 * time.Millisecond)
	cancelSignal()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("serve() shutdown error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return; shutdown hung on the AgentOps bridge")
	}
	notified := runner.notifiedTaskIDs()
	for _, id := range notified {
		if !taskIDs[id] {
			t.Fatalf("notified unknown task: %s", id)
		}
	}
	seen := make(map[string]bool, len(notified))
	for _, id := range notified {
		if seen[id] {
			t.Fatalf("task %s received more than one cancellation terminal", id)
		}
		seen[id] = true
	}
}

func fmtTaskID(i int) string {
	return "task-" + strconv.Itoa(i)
}

// admissionGateway records when StopAccepting starts and blocks it until the
// test releases it, so the test can prove the shared task channel is not
// closed while gateway admission is still undrained.
type admissionGateway struct {
	probeGateway
	stopAcceptingStarted chan struct{}
	releaseStopAccepting chan struct{}
}

func (g *admissionGateway) StopAccepting(ctx context.Context) error {
	if g.stopAcceptingStarted != nil {
		close(g.stopAcceptingStarted)
	}
	select {
	case <-g.releaseStopAccepting:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// A returned listener ends serve, and serve must drain gateway admission
// before closing the shared Feishu task channel: an in-flight delivery
// handler that is already past reserveDelivery would otherwise send on a
// closed channel, losing the event and leaking its durable reservation.
func TestServeDrainsGatewayAdmissionBeforeClosingTaskChannel(t *testing.T) {
	gateway := &admissionGateway{
		stopAcceptingStarted: make(chan struct{}),
		releaseStopAccepting: make(chan struct{}),
	}
	feishuTasks := make(chan feishu.Task, 1)
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	defer cancelSignal()
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, &cancellingRunner{}, feishuTasks, ":0", time.Second)
	}()
	select {
	case <-gateway.stopAcceptingStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not drain gateway admission before closing the task channel")
	}
	feishuTasks <- feishu.Task{TaskID: "in-flight", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "task"}
	close(gateway.releaseStopAccepting)
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("serve() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after admission drain")
	}
}
