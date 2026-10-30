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
	shared  *programState
	state   *TransformState
}

// New returns the transformer as a goblox plugin. A plugin keeps one TransformState per program (TypeScript makes
// one per compile).
func New(config TransformerConfig) ts.Plugin {
	return &plugin{config: config}
}

func (p *plugin) Name() string { return Name }

// programStateFor is what the states of a program share, new for each program.
func (p *plugin) programStateFor(ctx *ts.TransformContext) *programState {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shared == nil || p.program != ctx.Program {
		if ctx.Verbose {
			newLogger(true).write("\n")
		}
		p.program = ctx.Program
		p.shared = &programState{}
		p.state = nil
	}
	return p.shared
}

// ForChecker is the plugin for the files of one checker, with its own TransformState: types and symbols belong to a
// checker. Semantic diagnostics ask every checker for the program's global diagnostics, so they run on one goroutine.
func (p *plugin) ForChecker() ts.Plugin {
	if p.config.semanticDiagnostics() {
		return nil
	}
	return &checkerPlugin{parent: p}
}

type checkerPlugin struct {
	parent *plugin
	state  *TransformState
}

func (p *checkerPlugin) Name() string { return Name }

func (p *checkerPlugin) TransformSourceFile(ctx *ts.TransformContext, file *ts.SourceFile) *ts.SourceFile {
	if p.state == nil || p.state.program != ctx.Program {
		p.state = newTransformState(ctx.Program, ctx, p.parent.config, p.parent.programStateFor(ctx))
	}
	p.state.setContext(ctx)
	return transformFile(p.state, file)
}

func (p *plugin) TransformSourceFile(ctx *ts.TransformContext, file *ts.SourceFile) *ts.SourceFile {
	shared := p.programStateFor(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == nil {
		p.state = newTransformState(ctx.Program, ctx, p.config, shared)
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
