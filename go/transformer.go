// Package transformer is the Go port of @typetorch/transformer, the roblox-ts transformer behind TypeTorch, for
// goblox-ts (roblox-ts on typescript-go). It provides user macros (generated `t` guards, stable type ids), DI class
// metadata and the `$print`/`$warn`/`$git`/... debug macros.
//
// Each Go file ports one TypeScript file of ../src and names it in its header; functions keep their TypeScript
// names, so fixes can be ported by hand. See README.md for what is ported.
//
// This file ports src/transformer.ts.
package transformer

import (
	"context"
	"sync"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// Name is the plugin's package name, as in a tsconfig plugin entry (`{ "transform": "@typetorch/transformer" }`).
const Name = "@typetorch/transformer"

type plugin struct {
	config Config

	mu      sync.Mutex
	program *ts.Program
	state   *TransformState
}

// New returns the transformer as a goblox plugin. A plugin keeps one TransformState per program (TypeScript makes
// one per compile).
func New(config Config) ts.Plugin {
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

// ConfigFromProgram reads the options of the `{ "transform": "@typetorch/transformer" }` entry in the program's
// tsconfig `compilerOptions.plugins` (false when there is none). Only the tsconfig itself is read, not the configs it
// extends: pkg/goblox doesn't expose the merged plugin entries yet (REQUESTS.md).
func ConfigFromProgram(program *ts.Program) (Config, bool) {
	// typescript-go's ordered JSON objects marshal with its own options (internal/json).
	data, err := json.Marshal(program.CommandLine().Raw, jsontext.AllowInvalidUTF8(true))
	if err != nil {
		return Config{}, false
	}
	var raw struct {
		CompilerOptions struct {
			Plugins []map[string]any `json:"plugins"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false
	}
	for _, entry := range raw.CompilerOptions.Plugins {
		if entry["transform"] == Name {
			return ConfigFromPluginEntry(entry), true
		}
	}
	return Config{}, false
}

// ConfigFromPluginEntry reads the options of a tsconfig plugin entry (`{ "transform": ..., ...options }`), already
// decoded from JSON.
func ConfigFromPluginEntry(entry map[string]any) Config {
	var config Config
	boolean := func(key string) *bool {
		if value, ok := entry[key].(bool); ok {
			return &value
		}
		return nil
	}
	text := func(key string) string {
		value, _ := entry[key].(string)
		return value
	}
	config.SemanticDiagnostics = boolean("semanticDiagnostics")
	config.NoSemanticDiagnostics = boolean("noSemanticDiagnostics")
	config.DebugMacros = boolean("debugMacros")
	config.RuntimeModule = text("runtimeModule")
	config.RuntimeReflectPath = text("runtimeReflectPath")
	config.GuardModule = text("guardModule")
	if optimizations, ok := entry["optimizations"].(map[string]any); ok {
		config.Optimizations = &Optimizations{}
		if limit, ok := optimizations["guardGenerationDedupLimit"].(float64); ok {
			value := int(limit)
			config.Optimizations.GuardGenerationDedupLimit = &value
		}
	}
	return config
}
