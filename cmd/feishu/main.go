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
//	ZHIPU_API_KEY           - Zhipu AI API key
//
// The server listens on :7777 for incoming Feishu webhook events.
package main

import (
	"context"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
)

func main() {
	appID := mustEnv("FEISHU_APP_ID")
	appSecret := mustEnv("FEISHU_APP_SECRET")
	verificationToken := mustEnv("FEISHU_VERIFICATION_TOKEN")
	encryptKey := mustEnv("FEISHU_ENCRYPT_KEY")

	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()

	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")
	sessionManager := session.NewManager(workDir)
	messenger := feishu.NewMessenger(appID, appSecret)
	approvalStore := approval.NewStore()

	tasks := make(chan feishu.Task, 32)

	runner := feishu.NewRunner(llmProvider, workDir, messenger, sessionManager, approvalStore)
	gateway := feishu.NewGateway(verificationToken, encryptKey, tasks, approvalStore)

	ctx := context.Background()
	go runner.Start(ctx, tasks)

	log.Printf("[Feishu] listening on :7777")
	if err := gateway.Listen(":7777"); err != nil {
		log.Fatal(err)
	}

}

// mustEnv reads an environment variable and exits if it is not set.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing environment variable: %s", key)
	}

	return v
}
