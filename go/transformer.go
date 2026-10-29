// Port of src/transformer.ts.

package transformer

import (
	"context"
	"sync"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

type plugin struct {
	config TransformerConfig

	mu      sync.Mutex
	program *ts.Program
	state   *TransformState
}

// New returns the transformer as a goblox plugin. A plugin keeps one TransformState per program (TypeScript makes
// one per compile).
func New(config TransformerConfig) ts.Plugin {
	return &plugin{config: config}
}

func (p *plugin) Name() string { return Name }

func (p *plugin) TransformSourceFile(ctx *ts.TransformContext, file *ts.SourceFile) *ts.SourceFile {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == nil || p.program != ctx.Program {
		if ctx.Verbose {
			newLogger(true).write("\n")
		}
		p.program = ctx.Program
		p.state = newTransformState(ctx.Program, ctx, p.config)
	}
	state := p.state
	state.setContext(ctx)

	// roblox-ts type-checks the transformed program and reports its diagnostics (with the original spans when the
	// original file has errors), so checking every file here again doubles the type-check time of the whole build.
	// Only do it when asked.
	if state.semanticDiagnostics {
		if originalFile := state.parseTreeNode(file.AsNode()); originalFile != nil {
			hasError := false
			var preEmitDiagnostics []*ts.Diagnostic
			for _, diagnostic := range ts.GetPreEmitDiagnostics(context.Background(), ctx.Program, originalFile.AsSourceFile()) {
				preEmitDiagnostics = append(preEmitDiagnostics, diagnostic)
				hasError = hasError || diagnostic.Category() == ts.CategoryError
			}
			if hasError {
				for _, diagnostic := range preEmitDiagnostics {
					if diagnostic.File() != nil {
						ctx.AddDiagnostic(diagnostic)
					}
				}
				return file
			}
		} else {
			relativeName := relativePath(state.srcDir, file.FileName())
			state.logger.warn("Failed to validate '" + relativeName + "' due to lack of parse tree node.")
		}
	}

	return transformFile(state, file)
}
