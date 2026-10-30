// Port of src/classes/transformState.ts.

package transformer

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// DefaultRuntimeModule is the package generated code imports the runtime (`Reflect`, and `t` as a fallback) from.
const DefaultRuntimeModule = "@typetorch/framework"

// DefaultRuntimeReflectPath is, inside the runtime package itself, the rootDir-relative file (no extension) that
// exports `Reflect`.
const DefaultRuntimeReflectPath = "reflection/reflect"

// packageRegex: roblox-ts treats a project whose package name matches this as a package (see createProjectData).
var packageRegex = regexp.MustCompile(`^@[a-z0-9-]*/`)

// TransformerConfig holds the options from the plugin entry in tsconfig.json:
// `{ "transform": "@typetorch/transformer", ...options }`. Field names and JSON keys are the TypeScript ones.
type TransformerConfig struct {
	// SemanticDiagnostics runs TypeScript's semantic check on every file before transforming it and skips files with
	// errors.
	//
	// Off by default: roblox-ts already type-checks the transformed program and reports its diagnostics, so this
	// check only protects the transformer from running on ill-typed code, at the cost of type-checking the whole
	// project a second time.
	SemanticDiagnostics *bool `json:"semanticDiagnostics,omitempty"`

	// Deprecated: semantic diagnostics are off by default now; use `semanticDiagnostics: true` to turn them on.
	NoSemanticDiagnostics *bool `json:"noSemanticDiagnostics,omitempty"`

	// DebugMacros is whether the debug macros (`$print`, `$warn`, `$error`, `$assert`, `$dbg` from
	// `@typetorch/transformer/macros`) compile their output and source positions in. On by default;
	// TYPETORCH_DEBUG_MACROS=0 in the environment turns them off whatever this says. Off, `$print`/`$warn`/`$dbg`
	// compile to nothing and `$assert`/`$error` to the plain call.
	DebugMacros *bool `json:"debugMacros,omitempty"`

	// RuntimeModule is the module generated code imports `Reflect` from (and `t`, when `@rbxts/t` can't be imported
	// directly). Defaults to `@typetorch/framework`.
	RuntimeModule string `json:"runtimeModule,omitempty"`

	// RuntimeReflectPath is only used while compiling the runtime package itself (package name == RuntimeModule):
	// the file that exports `Reflect`, relative to `rootDir`, without extension. Generated code imports it
	// relatively instead of importing the package from inside itself. Defaults to `reflection/reflect`.
	RuntimeReflectPath string `json:"runtimeReflectPath,omitempty"`

	// GuardModule is the module generated guards import `t` from. By default `@rbxts/t` is used when the file
	// resolves the same copy the runtime package uses, otherwise `t` is imported from RuntimeModule (which re-exports
	// it).
	GuardModule string `json:"guardModule,omitempty"`

	// Optimizations are some experimental optimizations.
	Optimizations *Optimizations `json:"optimizations,omitempty"`

	// Hooks replace the outside world (environment, clock, git, random UUIDs); for tests. Not part of the tsconfig
	// entry.
	Hooks Hooks `json:"-"`
}

// Optimizations is `TransformerConfig.optimizations`.
type Optimizations struct {
	// GuardGenerationDedupLimit: types used at least this many times inside one generated guard are hoisted into a
	// local and reused.
	GuardGenerationDedupLimit *int `json:"guardGenerationDedupLimit,omitempty"`
}

// semanticDiagnostics is whether each file is semantically checked before it is transformed. The deprecated
// `noSemanticDiagnostics: false` used to mean "check"; keep honoring it when set explicitly.
func (c TransformerConfig) semanticDiagnostics() bool {
	if c.SemanticDiagnostics != nil {
		return *c.SemanticDiagnostics
	}
	return c.NoSemanticDiagnostics != nil && !*c.NoSemanticDiagnostics
}

// Hooks replace what the TypeScript transformer reads from the process. Nil fields use the real thing.
type Hooks struct {
	Getenv func(key string) string
	Now    func() time.Time
	// Git runs git in cwd and returns its trimmed output, ok false when git failed.
	Git  func(cwd string, args ...string) (output string, ok bool)
	UUID func() string
}

func (h Hooks) getenv(key string) string {
	if h.Getenv != nil {
		return h.Getenv(key)
	}
	return os.Getenv(key)
}

func (h Hooks) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

type importItem struct {
	name       string
	identifier *ts.Node
}

type importInfo struct {
	path    string
	entries []importItem
}

// programState is what the TransformStates of a program share, one for each checker running at the same time.
type programState struct {
	macroCandidatesOnce sync.Once
	macroCandidates     map[string]bool

	gitInfoOnce sync.Once
	gitInfo     GitInfo

	debugMacrosLogOnce sync.Once
}

// TransformState is the per-compile state of the transformer, one for each checker.
type TransformState struct {
	parsedCommandLine commandLine
	currentDirectory  string
	options           *ts.CompilerOptions
	srcDir            string
	outDir            string
	rootDirs          []string
	typeChecker       *ts.Checker

	// semanticDiagnostics is whether each file is semantically checked before it is transformed (see TransformerConfig).
	semanticDiagnostics bool

	pathTranslator *pathTranslator

	// rootDirectory is the directory holding the project's package.json.
	rootDirectory string
	packageName   string

	// isPackage is whether the project compiles as a roblox-ts package. Ids of a package's own declarations are
	// prefixed with the package name, the same id consumers compute from its published declaration files.
	isPackage bool

	runtimeModule    string
	isRuntimePackage bool

	// debugMacros is whether the debug macros emit (see TransformerConfig.DebugMacros).
	debugMacros bool

	// macroCandidates is what collectMacroCandidates returned for the program.
	macroCandidates map[string]bool

	shared *programState

	isUserMacroCache map[*ts.Symbol]bool
	// macroCache maps a symbol to the built-in macro it declares ("": none).
	macroCache map[*ts.Symbol]macroName
	// macroFileCache maps a file name to whether the file declares built-in macros.
	macroFileCache    map[string]bool
	packageJsonFields *jsonObject
	guardLibraryPath  string
	resolver          *ts.ModuleResolver

	program     *ts.Program
	context     *ts.TransformContext
	config      TransformerConfig
	f           *factory
	visitor     *ts.NodeVisitor
	diagnostics *diagnostics
	logger      *logger

	fileImports map[string][]*importInfo
	prereqStack [][]*ts.Node
}

func newTransformState(program *ts.Program, context *ts.TransformContext, config TransformerConfig, shared *programState) *TransformState {
	tsconfigPath := path.Join(context.CurrentDirectory, "tsconfig.json")
	state := &TransformState{
		parsedCommandLine: commandLine{tsconfigPath: tsconfigPath, project: context.CurrentDirectory, projectType: context.ProjectType},
		currentDirectory:  context.CurrentDirectory,
		options:           program.Options(),
		program:           program,
		config:            config,
		shared:            shared,
		isUserMacroCache:  map[*ts.Symbol]bool{},
		macroCache:        map[*ts.Symbol]macroName{},
		macroFileCache:    map[string]bool{},
		fileImports:       map[string][]*importInfo{},
		diagnostics:       &diagnostics{},
		logger:            newLogger(context.Verbose),
	}
	state.srcDir = state.options.RootDir
	if state.srcDir == "" {
		state.srcDir = state.currentDirectory
	}
	state.outDir = state.options.OutDir
	if state.outDir == "" {
		state.outDir = state.currentDirectory
	}
	state.rootDirs = state.options.RootDirs
	if len(state.rootDirs) == 0 {
		state.rootDirs = []string{state.srcDir}
	}
	state.setContext(context)
	shared.macroCandidatesOnce.Do(func() { shared.macroCandidates = collectMacroCandidates(program) })
	state.macroCandidates = shared.macroCandidates

	pkg := getPackageJson(state.currentDirectory)
	assert(pkg.result.Name != "", "package.json in "+pkg.directory+" has no name")

	state.rootDirectory = pkg.directory
	state.packageName = pkg.result.Name
	state.pathTranslator = createPathTranslator(program)

	if projectType := state.parsedCommandLine.projectType; projectType != "" {
		state.isPackage = projectType == "package"
	} else {
		state.isPackage = packageRegex.MatchString(state.packageName)
	}

	state.runtimeModule = config.RuntimeModule
	if state.runtimeModule == "" {
		state.runtimeModule = DefaultRuntimeModule
	}
	state.isRuntimePackage = state.packageName == state.runtimeModule
	state.semanticDiagnostics = config.semanticDiagnostics()
	state.debugMacros = (config.DebugMacros == nil || *config.DebugMacros) && config.Hooks.getenv("TYPETORCH_DEBUG_MACROS") != "0"
	if state.logger.verbose && !state.debugMacros {
		shared.debugMacrosLogOnce.Do(func() {
			state.logger.writeLine("debug macros off: $print/$warn/$dbg compile to nothing, $assert/$error without the source position")
		})
	}
	return state
}

// setContext points the state at the checker, emit context and factory goblox hands out for the next file.
func (s *TransformState) setContext(context *ts.TransformContext) {
	s.context = context
	s.typeChecker = context.Checker
	if s.f == nil || s.f.ec != context.EmitContext {
		s.f = newFactory(context.EmitContext)
		s.visitor = context.EmitContext.NewNodeVisitor(func(node *ts.Node) *ts.Node { return transformNode(s, node) })
	}
}

func (s *TransformState) moduleResolver() *ts.ModuleResolver {
	if s.resolver == nil {
		s.resolver = ts.NewModuleResolver(s.program)
	}
	return s.resolver
}

// getGitInfo is the project's git state, read once per compile (so watch mode sees new commits).
func (s *TransformState) getGitInfo() GitInfo {
	s.shared.gitInfoOnce.Do(func() {
		git := gitRunner(runGit)
		if s.config.Hooks.Git != nil {
			git = s.config.Hooks.Git
		}
		s.shared.gitInfo = readGitInfo(s.rootDirectory, git, s.config.Hooks.now())
	})
	return s.shared.gitInfo
}

// getPackageJson is the project's package.json, every field (for `$package`).
func (s *TransformState) getPackageJson() *jsonObject {
	if s.packageJsonFields == nil {
		s.packageJsonFields = &jsonObject{values: map[string]any{}}
		if text, err := os.ReadFile(getPackageJson(s.currentDirectory).path); err == nil {
			if fields, err := decodeOrderedJSON(text); err == nil {
				s.packageJsonFields = fields
			}
		}
	}
	return s.packageJsonFields
}

// idPrefix is the prefix for ids of this project's own declarations (`<prefix>:<path>@<name>`), if any.
func (s *TransformState) idPrefix() (string, bool) {
	return s.packageName, s.isPackage
}

func (s *TransformState) isUserMacro(symbol *ts.Symbol) bool {
	if cached, ok := s.isUserMacroCache[symbol]; ok {
		return cached
	}

	for _, declaration := range symbol.Declarations {
		metadata := newNodeMetadata(s, declaration)
		if metadata.isRequested("macro") {
			s.isUserMacroCache[symbol] = true
			return true
		}
	}

	s.isUserMacroCache[symbol] = false
	return false
}

func (s *TransformState) addFileImport(file *ts.SourceFile, importPath string, name string) *ts.Node {
	importInfos := s.fileImports[file.FileName()]

	var info *importInfo
	for _, existing := range importInfos {
		if existing.path == importPath {
			info = existing
			break
		}
	}
	if info == nil {
		info = &importInfo{path: importPath}
		s.fileImports[file.FileName()] = append(importInfos, info)
	}

	var identifier *ts.Node
	for _, entry := range info.entries {
		if entry.name == name {
			identifier = entry.identifier
			break
		}
	}

	if identifier == nil {
	start:
		for _, statement := range file.Statements.Nodes {
			if statement.Kind != ts.KindImportDeclaration {
				break
			}
			declaration := statement.AsImportDeclaration()
			if declaration.ModuleSpecifier.Kind != ts.KindStringLiteral {
				continue
			}
			clause := declaration.ImportClause
			if clause == nil || clause.Kind != ts.KindImportClause {
				continue
			}
			bindings := clause.AsImportClause().NamedBindings
			if bindings == nil || bindings.Kind != ts.KindNamedImports {
				continue
			}
			if clause.IsTypeOnly() {
				continue
			}
			if declaration.ModuleSpecifier.Text() != importPath {
				continue
			}

			for _, element := range bindings.AsNamedImports().Elements.Nodes {
				specifier := element.AsImportSpecifier()
				if specifier.IsTypeOnly {
					continue
				}

				if specifier.PropertyName != nil {
					if specifier.PropertyName.Text() == name {
						identifier = element.Name()
						break start
					}
				} else if element.Name().Text() == name {
					identifier = element.Name()
					break start
				}
			}
		}
	}

	if identifier == nil {
		identifier = s.f.identifier(name, true)
		info.entries = append(info.entries, importItem{name: name, identifier: identifier})
	}

	return identifier
}

// getReflect imports `Reflect` from the runtime package. While compiling the runtime package itself, the import
// points at the file that defines it instead (a package can't import itself by name).
func (s *TransformState) getReflect(file *ts.SourceFile) *ts.Node {
	if !s.isRuntimePackage {
		return s.addFileImport(file, s.runtimeModule, "Reflect")
	}

	reflectPath := s.config.RuntimeReflectPath
	if reflectPath == "" {
		reflectPath = DefaultRuntimeReflectPath
	}
	reflectFile := path.Join(s.srcDir, reflectPath)
	importPath := strings.ReplaceAll(relativePath(path.Dir(file.FileName()), reflectFile), "\\", "/")
	if !strings.HasPrefix(importPath, ".") {
		importPath = "./" + importPath
	}

	return s.addFileImport(file, importPath, "Reflect")
}

// getGuardLibrary imports `t` (the guard library) into the file.
func (s *TransformState) getGuardLibrary(file *ts.SourceFile) *ts.Node {
	if s.config.GuardModule != "" {
		return s.addFileImport(file, s.config.GuardModule, "t")
	}

	if s.guardLibraryPath != "" {
		return s.addFileImport(file, s.guardLibraryPath, "t")
	}

	fileGuardPath := tryResolveTS(s, "@rbxts/t", path.Dir(file.FileName()))
	if s.isRuntimePackage {
		s.guardLibraryPath = "@rbxts/t"
		return s.addFileImport(file, s.guardLibraryPath, "t")
	}

	runtimePath := tryResolveTS(s, s.runtimeModule, file.FileName())
	if runtimePath == "" {
		if fileGuardPath == "" {
			s.diagnostics.warning(
				file.EndOfFileToken,
				"Neither @rbxts/t nor "+s.runtimeModule+" was found, guard generation may not work.",
			)
		}
		return s.addFileImport(file, "@rbxts/t", "t")
	}

	runtimeGuardPath := tryResolveTS(s, "@rbxts/t", runtimePath)
	if fileGuardPath != "" && fileGuardPath == runtimeGuardPath {
		// The runtime package and this project use the same @rbxts/t.
		s.guardLibraryPath = "@rbxts/t"
	} else {
		// Use the runtime's own copy, re-exported from its entry point.
		s.guardLibraryPath = s.runtimeModule
	}

	return s.addFileImport(file, s.guardLibraryPath, "t")
}

// parseTreeNode is TypeScript's `ts.getParseTreeNode`.
func (s *TransformState) parseTreeNode(node *ts.Node) *ts.Node {
	return s.context.EmitContext.ParseNode(node)
}

func (s *TransformState) getSourceFile(node *ts.Node) *ts.SourceFile {
	parseNode := s.parseTreeNode(node)
	if parseNode == nil {
		panic("Could not find parse tree node")
	}
	return ts.GetSourceFileOfNode(parseNode)
}

func (s *TransformState) getSymbol(node *ts.Node) *ts.Symbol {
	return s.getSymbolEx(node, true)
}

func (s *TransformState) getSymbolEx(node *ts.Node, followAlias bool) *ts.Symbol {
	if isNamedDeclaration(node) {
		return s.getSymbol(node.Name())
	}

	symbol := s.typeChecker.GetSymbolAtLocation(node)

	if symbol != nil && followAlias {
		return s.typeChecker.SkipAlias(symbol)
	}
	return symbol
}

func (s *TransformState) addDiagnostic(diag *ts.Diagnostic) {
	s.context.AddDiagnostic(diag)
}

func (s *TransformState) capture(cb func() *ts.Node) (*ts.Node, []*ts.Node) {
	s.prereqStack = append(s.prereqStack, []*ts.Node{})
	result := cb()
	prereqs := s.prereqStack[len(s.prereqStack)-1]
	s.prereqStack = s.prereqStack[:len(s.prereqStack)-1]
	return result, prereqs
}

func (s *TransformState) prereq(statement *ts.Node) {
	if len(s.prereqStack) > 0 {
		s.prereqStack[len(s.prereqStack)-1] = append(s.prereqStack[len(s.prereqStack)-1], statement)
	}
}

func (s *TransformState) prereqList(statements []*ts.Node) {
	if len(s.prereqStack) > 0 {
		s.prereqStack[len(s.prereqStack)-1] = append(s.prereqStack[len(s.prereqStack)-1], statements...)
	}
}

func (s *TransformState) isCapturing(threshold int) bool {
	return len(s.prereqStack) > threshold
}

// transform visits the children of node with the transformer.
func (s *TransformState) transform(node *ts.Node) *ts.Node {
	return s.visitor.VisitEachChild(node)
}

// transformNode visits node itself with the transformer.
func (s *TransformState) transformNode(node *ts.Node) *ts.Node {
	return s.visitor.VisitNode(node)
}

// jsonObject is a decoded JSON object that keeps JavaScript's property order (Object.entries: integer-like keys
// first, ascending, then the rest as written), which `$package` compiles in.
type jsonObject struct {
	keys   []string
	values map[string]any
}

func (o *jsonObject) get(key string) (any, bool) {
	value, ok := o.values[key]
	return value, ok
}

// decodeOrderedJSON decodes JSON into string, float64, bool, nil, []any and *jsonObject values.
func decodeOrderedJSON(text []byte) (*jsonObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	object, ok := value.(*jsonObject)
	if !ok {
		return &jsonObject{values: map[string]any{}}, nil
	}
	return object, nil
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		if value == '[' {
			array := []any{}
			for decoder.More() {
				element, err := decodeJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, element)
			}
			_, err := decoder.Token()
			return array, err
		}
		object := &jsonObject{values: map[string]any{}}
		var integerKeys, otherKeys []string
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key := keyToken.(string)
			element, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			if _, seen := object.values[key]; !seen {
				if isArrayIndex(key) {
					integerKeys = append(integerKeys, key)
				} else {
					otherKeys = append(otherKeys, key)
				}
			}
			object.values[key] = element
		}
		sort.Slice(integerKeys, func(i, j int) bool {
			a, _ := strconv.ParseUint(integerKeys[i], 10, 32)
			b, _ := strconv.ParseUint(integerKeys[j], 10, 32)
			return a < b
		})
		object.keys = append(integerKeys, otherKeys...)
		_, err := decoder.Token()
		return object, err
	case json.Number:
		return value.Float64()
	default:
		return value, nil
	}
}

// isArrayIndex is whether JavaScript orders the key as an integer index.
func isArrayIndex(key string) bool {
	n, err := strconv.ParseUint(key, 10, 32)
	return err == nil && n < 1<<32-1 && strconv.FormatUint(n, 10) == key
}
