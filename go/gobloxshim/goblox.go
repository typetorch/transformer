// Package goblox is a stand-in for goblox-ts' public `pkg/goblox` package until it lands.
//
// Go forbids importing typescript-go's internal/ packages from another module, but it decides by import path: this
// module's path is under github.com/microsoft/typescript-go/, so it may. It re-exports what @typetorch/transformer's
// Go port needs (type aliases, constants in constants_generated.go, a few free functions) and defines the plugin API
// the port is written against. Once pkg/goblox exists, the port imports that instead (see ../README.md).
package goblox

import (
	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/checker"
	"github.com/microsoft/typescript-go/internal/compiler"
	"github.com/microsoft/typescript-go/internal/core"
	"github.com/microsoft/typescript-go/internal/diagnostics"
	"github.com/microsoft/typescript-go/internal/jsnum"
	"github.com/microsoft/typescript-go/internal/module"
	"github.com/microsoft/typescript-go/internal/nodebuilder"
	"github.com/microsoft/typescript-go/internal/printer"
	"github.com/microsoft/typescript-go/internal/scanner"
	"github.com/microsoft/typescript-go/internal/tspath"
)

// AST.
type (
	Node                      = ast.Node
	NodeList                  = ast.NodeList
	ModifierList              = ast.ModifierList
	SourceFile                = ast.SourceFile
	Symbol                    = ast.Symbol
	SymbolTable               = ast.SymbolTable
	Kind                      = ast.Kind
	NodeFlags                 = ast.NodeFlags
	ModifierFlags             = ast.ModifierFlags
	SymbolFlags               = ast.SymbolFlags
	TokenFlags                = ast.TokenFlags
	NodeVisitor               = ast.NodeVisitor
	NodeVisitorHooks          = ast.NodeVisitorHooks
	Diagnostic                = ast.Diagnostic
	DiagnosticCategory        = diagnostics.Category
	TextRange                 = core.TextRange
	CompilerOptions           = core.CompilerOptions
	ASTNodeFactory            = ast.NodeFactory
	KeywordTypeSyntaxKind     = ast.KeywordTypeSyntaxKind
	TokenSyntaxKind           = ast.TokenSyntaxKind
	ImportPhaseModifierKind   = ast.ImportPhaseModifierSyntaxKind
	ConcreteImportClause      = ast.ImportClause
	ConcreteImportSpecifier   = ast.ImportSpecifier
	ConcreteImportDeclaration = ast.ImportDeclaration
)

// Checker.
type (
	Checker          = checker.Checker
	Type             = checker.Type
	TypeFlags        = checker.TypeFlags
	ObjectFlags      = checker.ObjectFlags
	Signature        = checker.Signature
	SignatureKind    = checker.SignatureKind
	IndexInfo        = checker.IndexInfo
	TupleType        = checker.TupleType
	LiteralType      = checker.LiteralType
	ElementFlags     = checker.ElementFlags
	Program          = compiler.Program
	NodeBuilderFlags = nodebuilder.Flags
)

// Printer.
type (
	EmitContext              = printer.EmitContext
	NodeFactory              = printer.NodeFactory
	Printer                  = printer.Printer
	PrinterOptions           = printer.PrinterOptions
	AutoGenerateOptions      = printer.AutoGenerateOptions
	GeneratedIdentifierFlags = printer.GeneratedIdentifierFlags
)

// TransformContext is what a plugin gets for each source file it transforms.
type TransformContext struct {
	Program     *compiler.Program
	Checker     *checker.Checker
	Options     *core.CompilerOptions
	EmitContext *printer.EmitContext
	Factory     *printer.NodeFactory
	// CurrentDirectory is the directory of the project's tsconfig.json.
	CurrentDirectory string
	// ProjectType is roblox-ts' `--type` option ("game", "model", "package"), empty when not passed.
	ProjectType string
	// Verbose is roblox-ts' `--verbose`.
	Verbose bool
	// AddDiagnostic reports a diagnostic (errors fail the compile).
	AddDiagnostic func(*ast.Diagnostic)
}

// Plugin is a before-transform run over every source file of the program, in place of a ts-patch style JS
// transformer (`{ "transform": "..." }` in tsconfig's plugins).
type Plugin interface {
	// Name is how the plugin is referred to in diagnostics and logs.
	Name() string
	// TransformSourceFile returns the transformed file (or the file itself when nothing changed).
	TransformSourceFile(ctx *TransformContext, file *ast.SourceFile) *ast.SourceFile
}

// Free functions of typescript-go the port uses.

func GetSourceFileOfNode(node *ast.Node) *ast.SourceFile { return ast.GetSourceFileOfNode(node) }

// GetTokenPosOfNode is TypeScript's `node.getStart()`.
func GetTokenPosOfNode(node *ast.Node, file *ast.SourceFile) int {
	return scanner.GetTokenPosOfNode(node, file, false)
}

// GetLineAndCharacterOfPosition is TypeScript's: zero-based line, UTF-16 character.
func GetLineAndCharacterOfPosition(file *ast.SourceFile, pos int) (int, int) {
	line, character := scanner.GetECMALineAndUTF16CharacterOfPosition(file, pos)
	return line, int(character)
}

// GetTextOfNode is TypeScript's `node.getText()`.
func GetTextOfNode(node *ast.Node) string { return scanner.GetTextOfNode(node) }

func GetTextOfJSDocComment(comment *ast.NodeList) string {
	return scanner.GetTextOfJSDocComment(comment)
}

func GetNextJSDocCommentLocation(node *ast.Node) *ast.Node {
	return ast.GetNextJSDocCommentLocation(node)
}

func IsTupleType(t *checker.Type) bool { return checker.IsTupleType(t) }

// GetPropertyNameForPropertyNameNode is TypeScript's: false for a computed name that isn't a literal.
func GetPropertyNameForPropertyNameNode(name *ast.Node) (string, bool) {
	text := ast.GetPropertyNameForPropertyNameNode(name)
	return text, text != ast.InternalSymbolNameMissing
}

func IsExpression(node *ast.Node) bool      { return ast.IsExpression(node) }
func IsStatement(node *ast.Node) bool       { return ast.IsStatement(node) }
func IsClassElement(node *ast.Node) bool    { return ast.IsClassElement(node) }
func IsClassLike(node *ast.Node) bool       { return ast.IsClassLike(node) }
func IsKeywordKind(kind ast.Kind) bool      { return ast.IsKeywordKind(kind) }
func IsDeclaration(node *ast.Node) bool     { return ast.IsDeclaration(node) }
func CanHaveDecorators(node *ast.Node) bool { return ast.CanHaveDecorators(node) }
func CanHaveModifiers(node *ast.Node) bool  { return ast.CanHaveModifiers(node) }
func HasStaticModifier(node *ast.Node) bool { return ast.HasStaticModifier(node) }

// GetEffectiveImplementsTypeNodes is TypeScript's (without the JSDoc `@implements` part, TS files only).
func GetEffectiveImplementsTypeNodes(node *ast.Node) []*ast.Node {
	if clause := ast.GetHeritageClause(node, ast.KindImplementsKeyword); clause != nil {
		return clause.AsHeritageClause().Types.Nodes
	}
	return nil
}
func GetDecorators(node *ast.Node) []*ast.Node { return node.Decorators() }

// NewDiagnostic is a diagnostic with a plain message (no message catalogue entry), as plugins report them.
func NewDiagnostic(file *ast.SourceFile, start int, length int, category diagnostics.Category, source string, message string) *ast.Diagnostic {
	d := ast.NewDiagnosticFromSerialized(file, core.NewTextRange(start, start+length), 0, category, "", nil, nil, nil, false, false, false)
	return d.SetExternalData(source, message)
}

func NewEmitContext() *printer.EmitContext { return printer.NewEmitContext() }

func NewPrinter(options printer.PrinterOptions, emitContext *printer.EmitContext) *printer.Printer {
	return printer.NewPrinter(options, printer.PrintHandlers{}, emitContext)
}

func NormalizePath(path string) string { return tspath.NormalizePath(path) }

func GetNormalizedAbsolutePath(path string, currentDirectory string) string {
	return tspath.GetNormalizedAbsolutePath(path, currentDirectory)
}

// NumberToString formats a number the way JavaScript's String(number) does.
func NumberToString(value float64) string { return jsnum.Number(value).String() }

// NumberValue is the float64 of a checker number (literal type values, enum constants), ok false otherwise.
func NumberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case jsnum.Number:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func NewTextRange(pos int, end int) core.TextRange { return core.NewTextRange(pos, end) }

// ModuleResolver resolves module names the way the program does (TypeScript's ts.resolveModuleName).
type ModuleResolver struct{ resolver *module.Resolver }

func NewModuleResolver(program *compiler.Program) *ModuleResolver {
	return &ModuleResolver{module.NewResolver(program.Host(), program.Options(), "", "", nil)}
}

// ResolveModuleName returns the resolved file name, "" when the module isn't found. `containingFile`'s directory is
// where resolution starts.
func (r *ModuleResolver) ResolveModuleName(moduleName string, containingFile string) string {
	resolved, _ := r.resolver.ResolveModuleName(moduleName, containingFile, core.ResolutionModeNone, nil)
	if resolved == nil || !resolved.IsResolved() {
		return ""
	}
	return resolved.ResolvedFileName
}

// Comments (TypeScript's ts.copyComments, ts.moveSyntheticComments, ts.removeAllComments).

func addComments(ec *printer.EmitContext, target *ast.Node, text string, ranges func(yield func(ast.CommentRange) bool), leading bool) {
	for comment := range ranges {
		pos, end := comment.Pos()+2, comment.End()
		if comment.Kind == ast.KindMultiLineCommentTrivia {
			end -= 2
		}
		if leading {
			ec.AddSyntheticLeadingComment(target, comment.Kind, text[pos:end], comment.HasTrailingNewLine)
		} else {
			ec.AddSyntheticTrailingComment(target, comment.Kind, text[pos:end], comment.HasTrailingNewLine)
		}
	}
}

// CopyComments adds the source comments of `source` (a parse tree node) to `target` as synthetic comments.
func CopyComments(ec *printer.EmitContext, source *ast.Node, target *ast.Node) {
	file := ast.GetSourceFileOfNode(source)
	if file == nil || source.Pos() < 0 {
		return
	}
	text := file.Text()
	addComments(ec, target, text, scanner.GetLeadingCommentRanges(&ec.Factory.NodeFactory, text, source.Pos()), true)
	addComments(ec, target, text, scanner.GetTrailingCommentRanges(&ec.Factory.NodeFactory, text, source.End()), false)
}

// MoveSyntheticComments moves the synthetic comments of `original` to `node`.
func MoveSyntheticComments(ec *printer.EmitContext, node *ast.Node, original *ast.Node) {
	ec.SetSyntheticLeadingComments(node, ec.GetSyntheticLeadingComments(original))
	ec.SetSyntheticTrailingComments(node, ec.GetSyntheticTrailingComments(original))
	ec.SetSyntheticLeadingComments(original, nil)
	ec.SetSyntheticTrailingComments(original, nil)
}

// RemoveAllComments stops the node's source comments from being emitted.
func RemoveAllComments(ec *printer.EmitContext, node *ast.Node) {
	ec.AddEmitFlags(node, printer.EFNoComments)
}

// AddSyntheticLeadingComment is TypeScript's: `kind` is a single- or multi-line comment trivia kind.
func AddSyntheticLeadingComment(ec *printer.EmitContext, node *ast.Node, kind ast.Kind, text string) {
	ec.AddSyntheticLeadingComment(node, kind, text, kind == ast.KindSingleLineCommentTrivia)
}

// FormatType prints `t` as a type node (TypeScript's
// `printer.printNode(typeToTypeNode(t, undefined, InTypeAlias | IgnoreErrors))`).
func FormatType(c *checker.Checker, t *checker.Type) string {
	typeNode := c.TypeToTypeNode(t, nil, nodebuilder.FlagsInTypeAlias|nodebuilder.FlagsIgnoreErrors, nil)
	if typeNode == nil {
		return c.TypeToString(t)
	}
	ec := printer.NewEmitContext()
	return printer.NewPrinter(printer.PrinterOptions{}, printer.PrintHandlers{}, ec).Emit(typeNode, nil)
}
