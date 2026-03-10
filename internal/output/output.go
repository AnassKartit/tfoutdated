package output

import (
	"fmt"
	"io"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
)

// Renderer renders analysis results.
type Renderer interface {
	Render(w io.Writer, analysis *analyzer.Analysis) error
}

// Options configures output rendering.
type Options struct {
	NoColor bool
}

// New creates a renderer for the given format.
func New(format string, opts Options) (Renderer, error) {
	switch format {
	case "table", "":
		return &TableRenderer{NoColor: opts.NoColor}, nil
	case "json":
		return &JSONRenderer{}, nil
	case "markdown", "md":
		return &MarkdownRenderer{}, nil
	case "html":
		return &HTMLRenderer{}, nil
	case "github":
		return &GitHubRenderer{}, nil
	case "azdevops":
		return &AzDevOpsRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown output format: %s (supported: table, json, markdown, html, github, azdevops)", format)
	}
}
