package breaking

// KnowledgeBase provides curated breaking change data for known providers.
// This takes precedence over parsed changelogs (higher fidelity).
type KnowledgeBase struct {
	changes map[string][]BreakingChange // provider → changes
}

// NewKnowledgeBase creates a knowledge base with all built-in provider data.
func NewKnowledgeBase() *KnowledgeBase {
	kb := &KnowledgeBase{
		changes: make(map[string][]BreakingChange),
	}

	// Register all known provider breaking changes
	kb.registerAzureRM()
	kb.registerAzureAD()
	kb.registerAzAPI()

	return kb
}

// GetChanges returns known breaking changes for a provider between two versions.
func (kb *KnowledgeBase) GetChanges(provider, fromVersion, toVersion string) []BreakingChange {
	allChanges, ok := kb.changes[provider]
	if !ok {
		return nil
	}

	var relevant []BreakingChange
	for _, bc := range allChanges {
		// Include changes introduced after fromVersion and up to toVersion
		if bc.Version > fromVersion && bc.Version <= toVersion {
			relevant = append(relevant, bc)
		}
	}
	return relevant
}

// SupportedProviders returns the list of providers with breaking change knowledge.
func (kb *KnowledgeBase) SupportedProviders() []string {
	var providers []string
	for p := range kb.changes {
		providers = append(providers, p)
	}
	return providers
}

func (kb *KnowledgeBase) register(provider string, changes []BreakingChange) {
	kb.changes[provider] = append(kb.changes[provider], changes...)
}
