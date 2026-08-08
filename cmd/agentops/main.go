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
	"log"
	"os"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/provider"
)

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

	feishuTasks := make(chan feishu.Task, 64)
	gateway := feishu.NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore)
	runner := agentops.NewRunner(llmProvider, workDir, logDir, messenger, approvalStore)
	deduper := NewDeduper()

	ctx := context.Background()
	agentTasks := make(chan agentops.Task, 64)
	go runner.Start(ctx, agentTasks)
	go func() {
		defer close(agentTasks)
		for task := range feishuTasks {
			if !deduper.Mark(task.MessageID) {
				continue
			}

			agentTask := agentops.Parse(task.Text)
			agentTask.TaskID = task.TaskID
			agentTask.ChatID = task.ChatID
			agentTask.SenderID = task.SenderID
			agentTask.MessageID = task.MessageID
			select {
			case agentTasks <- agentTask:
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Println("[AgentOps] listening on :7777")
	if err := gateway.Listen(":7777"); err != nil {
		log.Fatal(err)
	}
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

// Deduper prevents duplicate processing of Feishu messages by tracking seen message IDs.
type Deduper struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	now  func() time.Time
}

const defaultDedupeTTL = 24 * time.Hour

// NewDeduper creates a new Deduper with an empty seen set.
func NewDeduper() *Deduper {
	return NewDeduperWithTTL(defaultDedupeTTL)
}

// NewDeduperWithTTL creates a Deduper that reclaims message IDs after ttl.
func NewDeduperWithTTL(ttl time.Duration) *Deduper {
	return &Deduper{seen: make(map[string]time.Time), ttl: ttl, now: time.Now}
}

// Mark records a message ID and reports whether it was seen for the first time.
// Returns true if the ID is new (should be processed), false if already seen.
func (d *Deduper) Mark(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.nowOrDefault()
	d.cleanupLocked(now)
	if _, ok := d.seen[id]; ok {
		return false
	}
	d.seen[id] = now
	return true
}

// Cleanup removes expired message IDs from the dedupe set.
func (d *Deduper) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleanupLocked(d.nowOrDefault())
}

// Len returns the number of currently tracked message IDs.
func (d *Deduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.seen)
}

func (d *Deduper) cleanupLocked(now time.Time) {
	ttl := d.ttl
	if ttl <= 0 {
		ttl = defaultDedupeTTL
	}
	for id, seenAt := range d.seen {
		if now.Sub(seenAt) > ttl {
			delete(d.seen, id)
		}
	}
}

func (d *Deduper) nowOrDefault() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}
