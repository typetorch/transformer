package goblox

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/bundled"
	"github.com/microsoft/typescript-go/internal/collections"
	"github.com/microsoft/typescript-go/internal/compiler"
	"github.com/microsoft/typescript-go/internal/core"
	"github.com/microsoft/typescript-go/internal/execute/tsc"
	"github.com/microsoft/typescript-go/internal/printer"
	"github.com/microsoft/typescript-go/internal/tsoptions"
	"github.com/microsoft/typescript-go/internal/tspath"
	"github.com/microsoft/typescript-go/internal/vfs"
	"github.com/microsoft/typescript-go/internal/vfs/osvfs"
)

// GetPreEmitDiagnostics is TypeScript's ts.getPreEmitDiagnostics(program, file).
func GetPreEmitDiagnostics(program *compiler.Program, file *ast.SourceFile) []*ast.Diagnostic {
	ctx := context.Background()
	var diags []*ast.Diagnostic
	diags = append(diags, program.GetConfigFileParsingDiagnostics()...)
	diags = append(diags, program.GetSyntacticDiagnostics(ctx, file)...)
	diags = append(diags, program.GetGlobalDiagnostics(ctx)...)
	diags = append(diags, program.GetSemanticDiagnostics(ctx, file)...)
	if program.Options().GetEmitDeclarations() {
		diags = append(diags, program.GetDeclarationDiagnostics(ctx, file)...)
	}
	return compiler.SortAndDeduplicateDiagnostics(diags)
}

type host struct {
	fs  vfs.FS
	cwd string
}

func (h *host) FS() vfs.FS                  { return h.fs }
func (h *host) GetCurrentDirectory() string { return h.cwd }

// Project is a loaded tsconfig.json and its program: a minimal goblox driver for plugin tests.
type Project struct {
	Directory string
	Config    *tsoptions.ParsedCommandLine
	Program   *compiler.Program
}

// LoadProject parses `<directory>/tsconfig.json` and creates its program.
func LoadProject(directory string) (*Project, error) {
	directory = tspath.NormalizePath(directory)
	h := &host{fs: bundled.WrapFS(osvfs.FS()), cwd: directory}
	configFileName := tspath.CombinePaths(directory, "tsconfig.json")
	extendedConfigCache := &tsc.ExtendedConfigCache{}
	config, errs := tsoptions.GetParsedCommandLineOfConfigFile(configFileName, &core.CompilerOptions{}, nil, h, extendedConfigCache)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %s", configFileName, errs[0].String())
	}
	// TypeScript 7 defaults `types` to []; roblox-ts projects rely on TypeScript 5 including every package in
	// `typeRoots` (@rbxts/types, @rbxts/compiler-types).
	if options := config.CompilerOptions(); options.Types == nil {
		cliOptions := &core.CompilerOptions{Types: automaticTypeDirectiveNames(h.fs, options.TypeRoots)}
		config, errs = tsoptions.GetParsedCommandLineOfConfigFile(configFileName, cliOptions, nil, h, extendedConfigCache)
		if len(errs) > 0 {
			return nil, fmt.Errorf("%s: %s", configFileName, errs[0].String())
		}
	}
	compilerHost := compiler.NewCachedFSCompilerHost(directory, h.fs, bundled.LibPath(), extendedConfigCache, nil, nil)
	program := compiler.NewProgram(compiler.ProgramOptions{Config: config, Host: compilerHost, SingleThreaded: core.TSTrue})
	return &Project{Directory: directory, Config: config, Program: program}, nil
}

// automaticTypeDirectiveNames is TypeScript 5's getAutomaticTypeDirectiveNames: every package directory in the type
// roots, except those whose package.json says `"typings": null`.
func automaticTypeDirectiveNames(fs vfs.FS, typeRoots []string) []string {
	var names []string
	for _, root := range typeRoots {
		for _, directory := range fs.GetAccessibleEntries(root).Directories {
			if strings.HasPrefix(directory, ".") {
				continue
			}
			if text, ok := fs.ReadFile(tspath.CombinePaths(root, directory, "package.json")); ok && strings.Contains(text, `"typings": null`) {
				continue
			}
			names = append(names, directory)
		}
	}
	return names
}

// PluginEntries are the `compilerOptions.plugins` entries of the tsconfig (typescript-go parses them but keeps
// them only in the raw config; entries from an `extends`ed config are not seen).
func (p *Project) PluginEntries() []map[string]any {
	raw, _ := plainJSON(p.Config.Raw).(map[string]any)
	compilerOptions, _ := raw["compilerOptions"].(map[string]any)
	plugins, _ := compilerOptions["plugins"].([]any)
	var entries []map[string]any
	for _, plugin := range plugins {
		if entry, ok := plugin.(map[string]any); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// plainJSON converts typescript-go's ordered JSON objects to map[string]any.
func plainJSON(value any) any {
	switch v := value.(type) {
	case *collections.OrderedMap[string, any]:
		result := map[string]any{}
		for key, element := range v.Entries() {
			result[key] = plainJSON(element)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, element := range v {
			result[i] = plainJSON(element)
		}
		return result
	}
	return value
}

// TransformResult is one source file after a plugin ran over it.
type TransformResult struct {
	File        *ast.SourceFile
	Transformed *ast.SourceFile
	EmitContext *printer.EmitContext
}

// Transform runs the plugin over every non-declaration source file under rootDir (as roblox-ts does), in program
// order, and returns the files with the diagnostics the plugin reported.
func (p *Project) Transform(plugin Plugin, projectType string) ([]TransformResult, []*ast.Diagnostic) {
	ctx := context.Background()
	rootDir := p.Program.Options().RootDir
	var results []TransformResult
	var diagnostics []*ast.Diagnostic
	for _, file := range p.Program.GetSourceFiles() {
		if file.IsDeclarationFile || (rootDir != "" && !strings.HasPrefix(file.FileName(), tspath.EnsureTrailingDirectorySeparator(rootDir))) {
			continue
		}
		checker, done := p.Program.GetTypeCheckerForFile(ctx, file)
		emitContext := printer.NewEmitContext()
		transformContext := &TransformContext{
			Program:          p.Program,
			Checker:          checker,
			Options:          p.Program.Options(),
			EmitContext:      emitContext,
			Factory:          emitContext.Factory,
			CurrentDirectory: p.Directory,
			ProjectType:      projectType,
			AddDiagnostic:    func(d *ast.Diagnostic) { diagnostics = append(diagnostics, d) },
		}
		transformed := plugin.TransformSourceFile(transformContext, file)
		done()
		results = append(results, TransformResult{File: file, Transformed: transformed, EmitContext: emitContext})
	}
	return results, diagnostics
}

// Print prints a transformed file the way roblox-ts hands it to its own compile (TypeScript's printer, LF).
func (r TransformResult) Print() string {
	p := printer.NewPrinter(printer.PrinterOptions{NewLine: core.NewLineKindLF}, printer.PrintHandlers{}, r.EmitContext)
	return p.EmitSourceFile(r.Transformed)
}
