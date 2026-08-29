// Package main is the entry point for the AgentOps server.
//
// The AgentOps server provides Feishu/Lark integration for production
// incident analysis. It receives incident reports via Feishu webhooks
// and dispatches AI-powered analysis tasks.
//
// Required environment variables:
//
//	FEISHU_APP_ID           - Feishu application ID
//	FEISHU_APP_SECRET       - Feishu application secret
//	FEISHU_VERIFICATION_TOKEN - Feishu webhook verification token
//	FEISHU_ENCRYPT_KEY      - Feishu webhook encryption key
//	AGENTOPS_WORKDIR        - Working directory for agent execution
//	AGENTOPS_LOGDIR         - Directory for log storage
//
// The server listens on :7777 for incoming Feishu webhook events.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
)

const defaultShutdownTimeout = 30 * time.Second

type gatewayService interface {
	Listen(string) error
	StopAccepting(context.Context) error
	Shutdown(context.Context) error
}

type runnerService interface {
	Start(context.Context, <-chan agentops.Task)
	NotifyCancellation(agentops.Task)
}

func main() {
	appID := mustEnv("FEISHU_APP_ID")
	appSecret := mustEnv("FEISHU_APP_SECRET")
	verificationToken := mustEnv("FEISHU_VERIFICATION_TOKEN")
	encryptKey := mustEnv("FEISHU_ENCRYPT_KEY")

	workDir := mustEnv("AGENTOPS_WORKDIR")
	logDir := mustEnv("AGENTOPS_LOGDIR")

	homeDir, _ := os.UserHomeDir()
	llmProvider, err := newConfiguredLLMProvider(homeDir, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	messenger := feishu.NewMessenger(appID, appSecret)
	approvalStore := approval.NewStore()
	deliveryStore, err := newDeliveryStore(homeDir)
	if err != nil {
		log.Fatal(err)
	}

	feishuTasks := make(chan feishu.Task, 64)
	gateway := feishu.NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore).WithDeliveryStore(deliveryStore)
	sessionStore := session.NewFileStore(workDir)
	taskFactory := newAgentOpsTaskExecutionFactory(llmProvider, workDir, logDir, messenger, sessionStore, approvalStore)
	runner := agentops.NewRunner(taskFactory, messenger).
		WithDeliveryFailureObserver(agentops.NewLoggingDeliveryFailureObserver(log.Default()))

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := serve(signalCtx, gateway, runner, feishuTasks, ":7777", defaultShutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

func serve(
	signalCtx context.Context,
	gateway gatewayService,
	runner runnerService,
	feishuTasks chan feishu.Task,
	addr string,
	shutdownTimeout time.Duration,
) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	defer cancelRunner()

	agentTasks := make(chan agentops.Task, 64)
	runnerDone := make(chan struct{})
	go func() {
		runner.Start(runnerCtx, agentTasks)
		close(runnerDone)
	}()

	bridgeDone := make(chan struct{})
	go func() {
		bridgeAgentOpsTasks(runnerCtx, feishuTasks, agentTasks, runner.NotifyCancellation)
		close(bridgeDone)
	}()

	listenResult := make(chan error, 1)
	go func() {
		log.Printf("[AgentOps] listening on %s", addr)
		listenResult <- gateway.Listen(addr)
	}()

	select {
	case listenErr := <-listenResult:
		// A returned listener ends serve regardless of the error. Drain
		// gateway admission before closing the shared task channel, so an
		// in-flight delivery handler that already reserved its message can
		// never send on a closed channel.
		waitCtx, cancelWait := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelWait()
		admissionErr := gateway.StopAccepting(waitCtx)
		close(feishuTasks)
		// The runner must stop too; its queued tasks reach cancellation
		// terminals through the bridge instead of blocking shutdown forever.
		cancelRunner()
		return errors.Join(
			listenErr,
			admissionErr,
			waitForCompletion(waitCtx, "AgentOps bridge", bridgeDone),
			waitForCompletion(waitCtx, "AgentOps runner", runnerDone),
		)
	case <-signalCtx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdown()
		admissionErr := gateway.StopAccepting(shutdownCtx)
		listenerCtx := shutdownCtx
		var cancelListener context.CancelFunc
		if shutdownCtx.Err() != nil {
			listenerCtx, cancelListener = context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancelListener()
		}
		shutdownErr := gateway.Shutdown(listenerCtx)
		listenErr, listenerStopped := waitForListenResult(listenerCtx, listenResult)
		waitCtx := listenerCtx
		var cancelWait context.CancelFunc
		if !listenerStopped {
			waitCtx, cancelWait = context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancelWait()
		}
		close(feishuTasks)
		cancelRunner()
		return errors.Join(
			admissionErr,
			shutdownErr,
			listenErr,
			waitForCompletion(waitCtx, "AgentOps bridge", bridgeDone),
			waitForCompletion(waitCtx, "AgentOps runner", runnerDone),
		)
	}
}

// bridgeAgentOpsTasks forwards accepted Feishu tasks to the AgentOps runner.
// Forwarding keeps draining preference: while the runner consumes, every
// queued task is handed to it exactly as before cancellation. Only when the
// capacity-64 buffer is full does the bridge wait context-aware, so shutdown
// can never deadlock on a runner that stopped consuming: the blocked task and
// everything still queued receive their cancellation terminal through
// notifyCancelled instead of hanging serve until its shutdown budget expires.
func bridgeAgentOpsTasks(
	ctx context.Context,
	feishuTasks <-chan feishu.Task,
	agentTasks chan<- agentops.Task,
	notifyCancelled func(agentops.Task),
) {
	defer close(agentTasks)
	convert := func(task feishu.Task) agentops.Task {
		agentTask := agentops.Parse(task.Text)
		agentTask.TaskID = task.TaskID
		agentTask.ChatID = task.ChatID
		agentTask.SenderID = task.SenderID
		agentTask.MessageID = task.MessageID
		return agentTask
	}
	for task := range feishuTasks {
		agentTask := convert(task)
		select {
		case agentTasks <- agentTask:
			continue
		default:
		}
		select {
		case agentTasks <- agentTask:
		case <-ctx.Done():
			notifyCancelled(agentTask)
			for queued := range feishuTasks {
				notifyCancelled(convert(queued))
			}
			return
		}
	}
}

func waitForListenResult(ctx context.Context, result <-chan error) (error, bool) {
	select {
	case err := <-result:
		return err, true
	case <-ctx.Done():
		return fmt.Errorf("wait for AgentOps listener: %w", ctx.Err()), false
	}
}

func waitForCompletion(ctx context.Context, name string, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for %s: %w", name, ctx.Err())
	}
}

func newDeliveryStore(homeDir string) (feishu.DeliveryStore, error) {
	return feishu.NewFileDeliveryStore(homeDir, filepath.Join(".foxharness", "feishu", "deliveries.json"))
}

func newConfiguredLLMProvider(homeDir string, lookup llmconfig.EnvLookup) (provider.LLMProvider, error) {
	llmConfig, err := llmresolve.FromUserSettings(homeDir, llmconfig.CLIOverrides{}, lookup)
	if err != nil {
		return nil, err
	}
	return provider.NewProvider(llmConfig)
}

// mustEnv reads an environment variable and exits if it is not set.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing environment variable: %s", key)
	}
	return v
}
