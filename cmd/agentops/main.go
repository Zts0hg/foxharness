package main

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/provider"
)

func main() {
	appID := mustEnv("FEISHU_APP_ID")
	appSecret := mustEnv("FEISHU_APP_SECRET")
	verificationToken := mustEnv("FEISHU_VERIFICATION_TOKEN")
	encryptKey := mustEnv("FEISHU_ENCRYPT_KEY")

	workDir := mustEnv("AGENTOPS_WORKDIR")
	logDir := mustEnv("AGENTOPS_LOGDIR")

	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")
	messenger := feishu.NewMessenger(appID, appSecret)
	approvalStore := approval.NewStore()

	feishuTasks := make(chan feishu.Task, 64)
	gateway := feishu.NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore)
	runner := agentops.NewRunner(llmProvider, workDir, logDir, messenger, approvalStore)
	deduper := NewDeduper()

	ctx := context.Background()
	go func() {
		for task := range feishuTasks {
			if !deduper.Mark(task.MessageID) {
				continue
			}

			agentTask := agentops.Parse(task.Text)
			agentTask.TaskID = task.TaskID
			agentTask.ChatID = task.ChatID
			agentTask.SenderID = task.SenderID
			agentTask.MessageID = task.MessageID
			go runner.Run(ctx, agentTask)
		}
	}()

	log.Println("[AgentOps] listending on :7777")
	if err := gateway.Listen(":7777"); err != nil {
		log.Fatal(err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("缺少环境变量: %s", key)
	}
	return v
}

type Deduper struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewDeduper() *Deduper {
	return &Deduper{seen: make(map[string]bool)}
}

func (d *Deduper) Mark(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[id] {
		return false
	}
	d.seen[id] = true
	return true
}
