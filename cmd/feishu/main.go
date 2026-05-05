package main

import (
	"context"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/tools"
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

	LLMProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))

	eng := engine.NewAgentEngine(LLMProvider, registry, workDir, true)

	tasks := make(chan feishu.Task, 32)
	messenger := feishu.NewMessenger(appID, appSecret)
	runner := feishu.NewRunner(eng, messenger)
	gateway := feishu.NewGateway(verificationToken, encryptKey, tasks)

	ctx := context.Background()
	go runner.Start(ctx, tasks)

	log.Printf("[Feishu] listening on :7777")
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
