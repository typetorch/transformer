// Package transformer is the Go port of @typetorch/transformer, the roblox-ts transformer behind TypeTorch, for
// goblox-ts (roblox-ts on typescript-go). It provides user macros (generated `t` guards, stable type ids), DI class
// metadata and the `$print`/`$warn`/`$git`/... debug macros.
//
// New returns the transformer as a goblox.Plugin; TransformerConfig holds the tsconfig plugin entry's options.
//
// Each Go file ports one TypeScript file of ../src, named after its path (src/util/functions/gitInfo.ts is
// util_functions_gitInfo.go); functions keep their TypeScript names, so fixes can be ported by hand. See README.md
// for what is ported.
//
// This file stands for src/index.ts, the package entry point. The TypeScript one installs a require hook so the
// transformer uses roblox-ts' copy of TypeScript; goblox has one TypeScript, built in, so here it holds what a host
// needs to load the plugin from a tsconfig entry instead.
package transformer

import (
	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// Name is the plugin's package name, as in a tsconfig plugin entry (`{ "transform": "@typetorch/transformer" }`).
const Name = "@typetorch/transformer"

// ConfigFromProgram reads the options of the `{ "transform": "@typetorch/transformer" }` entry in the program's
// tsconfig `compilerOptions.plugins` (false when there is none). Only the tsconfig itself is read, not the configs it
// extends: pkg/goblox doesn't expose the merged plugin entries yet (REQUESTS.md).
func ConfigFromProgram(program *ts.Program) (TransformerConfig, bool) {
	// typescript-go's ordered JSON objects marshal with its own options (internal/json).
	data, err := json.Marshal(program.CommandLine().Raw, jsontext.AllowInvalidUTF8(true))
	if err != nil {
		return TransformerConfig{}, false
	}
	var raw struct {
		CompilerOptions struct {
			Plugins []map[string]any `json:"plugins"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return TransformerConfig{}, false
	}
	for _, entry := range raw.CompilerOptions.Plugins {
		if entry["transform"] == Name {
			return ConfigFromPluginEntry(entry), true
		}
	}
	return TransformerConfig{}, false
}

// ConfigFromPluginEntry reads the options of a tsconfig plugin entry (`{ "transform": ..., ...options }`), already
// decoded from JSON.
func ConfigFromPluginEntry(entry map[string]any) TransformerConfig {
	var config TransformerConfig
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
