package agentops

import (
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
)

type taskProviderMetadataSource interface {
	ProviderProtocol() string
	ModelName() string
}

type taskProviderSnapshot struct {
	provider taskScopedProvider
}

type taskScopedProvider struct {
	provider.LLMProvider
	protocol string
	model    string
}

func snapshotTaskProvider(llmProvider provider.LLMProvider) taskProviderSnapshot {
	snapshot := taskProviderSnapshot{provider: taskScopedProvider{LLMProvider: llmProvider}}
	metadata, ok := llmProvider.(taskProviderMetadataSource)
	if !ok {
		return snapshot
	}
	snapshot.provider.protocol = metadata.ProviderProtocol()
	snapshot.provider.model = metadata.ModelName()
	return snapshot
}

func (p taskScopedProvider) ProviderProtocol() string {
	return p.protocol
}

func (p taskScopedProvider) ModelName() string {
	return p.model
}

func (s taskProviderSnapshot) apply(engineConfig *engine.Config, compactionConfig *compaction.CompactionConfig) {
	engineConfig.ProviderProtocol = s.provider.protocol
	engineConfig.Model = s.provider.model
	compactionConfig.Model = s.provider.model
}
