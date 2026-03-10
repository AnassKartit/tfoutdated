package breaking

// ChangeKind classifies the type of breaking change.
type ChangeKind int

const (
	AttributeRemoved ChangeKind = iota
	AttributeRenamed
	ResourceRemoved
	ResourceRenamed
	ResourceSplit
	RequiredAdded
	TypeChanged
	BehaviorChanged
	DefaultChanged
	ProviderConfigChanged
	VariableRemoved
	VariableRenamed
	OutputRemoved
	OutputRenamed
	ProviderMigrated
	VariableValidationAdded
)

func (k ChangeKind) String() string {
	switch k {
	case AttributeRemoved:
		return "attribute removed"
	case AttributeRenamed:
		return "attribute renamed"
	case ResourceRemoved:
		return "resource removed"
	case ResourceRenamed:
		return "resource renamed"
	case ResourceSplit:
		return "resource split"
	case RequiredAdded:
		return "required attribute added"
	case TypeChanged:
		return "type changed"
	case BehaviorChanged:
		return "behavior changed"
	case DefaultChanged:
		return "default changed"
	case ProviderConfigChanged:
		return "provider config changed"
	case VariableRemoved:
		return "variable removed"
	case VariableRenamed:
		return "variable renamed"
	case OutputRemoved:
		return "output removed"
	case OutputRenamed:
		return "output renamed"
	case ProviderMigrated:
		return "provider migrated"
	case VariableValidationAdded:
		return "variable validation added"
	default:
		return "unknown"
	}
}

// Severity indicates the impact level of a breaking change.
type Severity int

const (
	SeverityInfo     Severity = iota // informational
	SeverityWarning                  // may require attention
	SeverityBreaking                 // will break existing configs
	SeverityCritical                 // data loss or security risk
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityBreaking:
		return "BREAKING"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// ValueHintConfidence indicates how confident we are about a value rewrite hint.
type ValueHintConfidence int

const (
	ValueHintNone    ValueHintConfidence = iota // no hint
	ValueHintSuggest                            // suggest to the user
	ValueHintAuto                               // auto-apply
)

// ValueHint describes a value rewrite suggestion for a renamed attribute.
type ValueHint struct {
	Confidence ValueHintConfidence
	OldSuffix  string // e.g., ".name"
	NewSuffix  string // e.g., ".id"
	Reason     string
}

// Transform describes a deterministic code transformation rule for a breaking change.
type Transform struct {
	RenameResource string            // new resource type name, e.g. "azurerm_linux_web_app"
	RenameAttrs    map[string]string // old attr name → new attr name
	RemoveAttrs    []string          // attributes to remove
	AddAttrs       map[string]string // new attr name → default value (quoted if string)
	ValueHints     map[string]ValueHint // old attr name → value rewrite hint
}

// BreakingChange describes a known breaking change in a provider or module.
type BreakingChange struct {
	Provider       string     // e.g., "azurerm", "azuread", "aws", "google"
	Version        string     // version that introduced the change
	ResourceType   string     // e.g., "azurerm_virtual_network"
	Attribute      string     // e.g., "resource_group_name"
	Kind           ChangeKind
	Severity       Severity
	Description    string
	MigrationGuide string
	OldValue       string // for renames: old attribute/resource name
	NewValue       string // for renames: new attribute/resource name
	IsModule       bool
	AutoFixable    bool
	BeforeSnippet  string // example Terraform code BEFORE the change
	AfterSnippet   string // example Terraform code AFTER the change
	EffortLevel    string // "small", "medium", "large" — how much work to migrate
	Transform       *Transform // deterministic transformation rule (nil if not applicable)
	DynamicDetected bool       // true if detected by schema diffing rather than knowledge base
}

// EffortEmoji returns a human-friendly indicator of migration effort.
func (bc *BreakingChange) EffortEmoji() string {
	switch bc.EffortLevel {
	case "small":
		return "Low effort (1-line change)"
	case "medium":
		return "Medium effort (a few attributes to update)"
	case "large":
		return "High effort (resource rewrite required)"
	default:
		return ""
	}
}
