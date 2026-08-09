package feishu

import (
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
)

type taskProviderMetadata struct {
	protocol string
	model    string
}

type taskProviderMetadataSource interface {
	ProviderProtocol() string
	ModelName() string
}

func snapshotTaskProviderMetadata(llmProvider provider.LLMProvider) taskProviderMetadata {
	metadata, ok := llmProvider.(taskProviderMetadataSource)
	if !ok {
		return taskProviderMetadata{}
	}
	return taskProviderMetadata{
		protocol: metadata.ProviderProtocol(),
		model:    metadata.ModelName(),
	}
}

func (m taskProviderMetadata) apply(engineConfig *engine.Config, compactionConfig *compaction.CompactionConfig) {
	engineConfig.ProviderProtocol = m.protocol
	engineConfig.Model = m.model
	compactionConfig.Model = m.model
}
