// Package main is the entry point for the Feishu webhook gateway server.
//
// The gateway receives Feishu/Lark webhook events and dispatches them
// to a runner that executes agent tasks and responds via Feishu messages.
//
// Required environment variables:
//
//	FEISHU_APP_ID           - Feishu application ID
//	FEISHU_APP_SECRET       - Feishu application secret
//	FEISHU_VERIFICATION_TOKEN - Feishu webhook verification token
//	FEISHU_ENCRYPT_KEY      - Feishu webhook encryption key
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
	Start(context.Context, <-chan feishu.Task)
}

func main() {
	appID := mustEnv("FEISHU_APP_ID")
	appSecret := mustEnv("FEISHU_APP_SECRET")
	verificationToken := mustEnv("FEISHU_VERIFICATION_TOKEN")
	encryptKey := mustEnv("FEISHU_ENCRYPT_KEY")

	workDir, _ := os.Getwd()

	homeDir, _ := os.UserHomeDir()
	llmProvider, err := newConfiguredLLMProvider(homeDir, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	sessionManager := session.NewFileStore(workDir)
	messenger := feishu.NewMessenger(appID, appSecret)
	approvalStore := approval.NewStore()

	tasks := make(chan feishu.Task, 32)

	taskFactory := newFeishuTaskExecutionFactory(llmProvider, workDir, messenger, sessionManager, approvalStore)
	runner := feishu.NewRunner(taskFactory, messenger).
		WithDeliveryFailureObserver(feishu.NewLoggingDeliveryFailureObserver(log.Default()))
	deliveryStore, err := newDeliveryStore(homeDir)
	if err != nil {
		log.Fatal(err)
	}
	gateway := feishu.NewGateway(verificationToken, encryptKey, tasks, approvalStore).WithDeliveryStore(deliveryStore)

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := serve(signalCtx, gateway, runner, tasks, ":7777", defaultShutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

func serve(
	signalCtx context.Context,
	gateway gatewayService,
	runner runnerService,
	tasks chan feishu.Task,
	addr string,
	shutdownTimeout time.Duration,
) error {
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	runnerCtx, cancelRunner := context.WithCancel(context.Background())
	defer cancelRunner()
	runnerDone := make(chan struct{})
	go func() {
		runner.Start(runnerCtx, tasks)
		close(runnerDone)
	}()

	listenResult := make(chan error, 1)
	go func() {
		log.Printf("[Feishu] listening on %s", addr)
		listenResult <- gateway.Listen(addr)
	}()

	select {
	case listenErr := <-listenResult:
		close(tasks)
		if listenErr != nil {
			cancelRunner()
		}
		waitCtx, cancelWait := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelWait()
		return errors.Join(listenErr, waitForCompletion(waitCtx, "runner", runnerDone))
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
		close(tasks)
		cancelRunner()
		runnerErr := waitForCompletion(waitCtx, "runner", runnerDone)
		return errors.Join(admissionErr, shutdownErr, listenErr, runnerErr)
	}
}

func waitForListenResult(ctx context.Context, result <-chan error) (error, bool) {
	select {
	case err := <-result:
		return err, true
	case <-ctx.Done():
		return fmt.Errorf("wait for Feishu listener: %w", ctx.Err()), false
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
	return feishu.NewFileDeliveryStore(filepath.Join(homeDir, ".foxharness", "feishu", "deliveries.json"))
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
