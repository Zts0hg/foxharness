package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/feishu"
)

func TestDVFEI006ProductionEntryCoordinatesShutdown(t *testing.T) {
	recorder := &shutdownRecorder{}
	gateway := &recordingGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
		shutdown:  make(chan struct{}),
	}
	runner := &recordingRunner{recorder: recorder}
	tasks := make(chan feishu.Task)
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, tasks, ":0", time.Second)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; err != nil {
		t.Fatalf("serve() error = %v", err)
	}
	if got, want := recorder.snapshot(), []string{"http-shutdown", "tasks-closed", "runner-cancelled"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown order = %#v, want %#v", got, want)
	}

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	source := string(data)
	for _, requiredCoordination := range []string{
		"signal.NotifyContext",
		"serve(signalCtx, gateway, runner, tasks",
	} {
		if !strings.Contains(source, requiredCoordination) {
			t.Fatalf("production entry does not contain %q", requiredCoordination)
		}
	}
	if strings.Contains(source, "context.Background()\n\tgo runner.Start") {
		t.Fatal("production entry still starts an uncoordinated background Runner")
	}
}

func TestDVFEI006ShutdownTimeoutStillDrainsAcceptedTasks(t *testing.T) {
	recorder := &shutdownRecorder{}
	gateway := &stuckRecordingGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
	}
	runner := &drainingRecordingRunner{recorder: recorder}
	tasks := make(chan feishu.Task, 1)
	tasks <- feishu.Task{TaskID: "accepted"}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, tasks, ":0", 20*time.Millisecond)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; !strings.Contains(fmt.Sprint(err), "wait for Feishu listener") {
		t.Fatalf("serve() error = %v, want listener shutdown timeout", err)
	}
	got := recorder.snapshot()
	if len(got) != 4 {
		t.Fatalf("shutdown-timeout order = %#v, want four coordinated events", got)
	}
	seen := make(map[string]bool, len(got))
	for _, event := range got {
		seen[event] = true
	}
	for _, required := range []string{"http-shutdown", "task-accepted", "tasks-closed", "runner-cancelled"} {
		if !seen[required] {
			t.Fatalf("shutdown-timeout order = %#v, missing %s", got, required)
		}
	}
	if got[2] != "tasks-closed" || got[3] != "runner-cancelled" {
		t.Fatalf("shutdown-timeout terminal order = %#v, want task input closed before runner cancellation", got)
	}
}

func TestDVFEI006StopAcceptingErrorStillDrainsAcceptedTasks(t *testing.T) {
	recorder := &shutdownRecorder{}
	gateway := &recordingGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
		shutdown:  make(chan struct{}),
		stopErr:   errors.New("stop accepting Feishu deliveries"),
	}
	runner := &drainingRecordingRunner{recorder: recorder}
	tasks := make(chan feishu.Task, 1)
	tasks <- feishu.Task{TaskID: "accepted"}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, tasks, ":0", time.Second)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; !strings.Contains(fmt.Sprint(err), "stop accepting Feishu deliveries") {
		t.Fatalf("serve() error = %v, want StopAccepting failure", err)
	}
	got := recorder.snapshot()
	for _, required := range []string{"http-shutdown", "task-accepted", "tasks-closed", "runner-cancelled"} {
		if !containsShutdownEvent(got, required) {
			t.Fatalf("StopAccepting failure order = %#v, missing %s", got, required)
		}
	}
	if len(got) < 2 {
		t.Fatalf("StopAccepting failure order = %#v, want terminal close and cancellation events", got)
	}
	if got[len(got)-2] != "tasks-closed" || got[len(got)-1] != "runner-cancelled" {
		t.Fatalf("StopAccepting failure terminal order = %#v, want input closed before runner cancellation", got)
	}
}

func TestDVFEI010ProductionEntryComposesDeliveryFailureObserver(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	source := string(data)
	for _, required := range []string{
		"WithDeliveryFailureObserver",
		"NewLoggingDeliveryFailureObserver(log.Default())",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production entry does not compose %q", required)
		}
	}
}

type shutdownRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *shutdownRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *shutdownRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type recordingGateway struct {
	recorder  *shutdownRecorder
	listening chan struct{}
	shutdown  chan struct{}
	stopErr   error
}

func (g *recordingGateway) Listen(string) error {
	close(g.listening)
	<-g.shutdown
	return nil
}

func (g *recordingGateway) Shutdown(context.Context) error {
	g.recorder.add("http-shutdown")
	close(g.shutdown)
	return nil
}

func (g *recordingGateway) StopAccepting(context.Context) error { return g.stopErr }

type recordingRunner struct {
	recorder *shutdownRecorder
}

func (r *recordingRunner) Start(ctx context.Context, tasks <-chan feishu.Task) {
	if _, ok := <-tasks; ok {
		return
	}
	r.recorder.add("tasks-closed")
	<-ctx.Done()
	r.recorder.add("runner-cancelled")
}

type stuckRecordingGateway struct {
	recorder  *shutdownRecorder
	listening chan struct{}
}

func (g *stuckRecordingGateway) Listen(string) error {
	close(g.listening)
	select {}
}

func (g *stuckRecordingGateway) Shutdown(context.Context) error {
	g.recorder.add("http-shutdown")
	return nil
}

func (*stuckRecordingGateway) StopAccepting(context.Context) error { return nil }

type drainingRecordingRunner struct {
	recorder *shutdownRecorder
}

func (r *drainingRecordingRunner) Start(ctx context.Context, tasks <-chan feishu.Task) {
	for task := range tasks {
		if task.TaskID == "accepted" {
			r.recorder.add("task-accepted")
		}
	}
	r.recorder.add("tasks-closed")
	<-ctx.Done()
	r.recorder.add("runner-cancelled")
}

func containsShutdownEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
