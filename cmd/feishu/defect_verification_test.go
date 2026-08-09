package main

import (
	"context"
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
