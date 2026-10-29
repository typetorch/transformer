// Port of src/transformations/macros/debugMacros.ts.
//
// The compile-time macros declared in `macros.d.ts` (`import { $print } from "@typetorch/transformer/macros"`), the
// successor of rbxts-transform-debug's. A declaration opts in with the JSDoc tag `@typetorchMacro <name>`; calls
// through rbxts-transform-debug's own declarations (same names) are accepted too, so a game can drop that plugin
// before it rewrites its imports.
//
// `print`, `warn`, `error`, `assert` and `dbg` are the debug macros: they compile the source position in
// (`[src/file.ts:line]`). With `state.debugMacros` off (plugin option `debugMacros: false`, or
// TYPETORCH_DEBUG_MACROS=0, which the TypeTorch CLI sets for prod builds) they compile to nothing, or to the plain
// `assert` / `error` call, so no source path reaches a prod payload.

package transformer

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

type macroName string

const (
	macroPrint       macroName = "print"
	macroWarn        macroName = "warn"
	macroError       macroName = "error"
	macroAssert      macroName = "assert"
	macroDbg         macroName = "dbg"
	macroNameof      macroName = "nameof"
	macroFile        macroName = "file"
	macroPackage     macroName = "package"
	macroGit         macroName = "git"
	macroCompileTime macroName = "compileTime"
)

var macroNames = map[string]bool{
	"print": true, "warn": true, "error": true, "assert": true, "dbg": true,
	"nameof": true, "file": true, "package": true, "git": true, "compileTime": true,
}

const macroTagName = "typetorchMacro"

// legacyModule matches rbxts-transform-debug's declaration file (`node_modules/rbxts-transform-debug/index.d.ts`).
var legacyModule = regexp.MustCompile(`[\\/]rbxts-transform-debug[\\/]`)

func macroTag(declaration *ts.Node) macroName {
	for _, tag := range getJSDocTags(declaration) {
		if tag.TagName().Text() != macroTagName {
			continue
		}
		name := strings.TrimSpace(ts.GetTextOfJSDocComment(tag.CommentList()))
		if macroNames[name] {
			return macroName(name)
		}
	}
	return ""
}

// macroOfSymbol is the macro `symbol` declares, if any (`state.getSymbol` has already followed the import alias).
func macroOfSymbol(state *TransformState, symbol *ts.Symbol) macroName {
	if cached, ok := state.macroCache[symbol]; ok {
		return cached
	}

	var macro macroName
	for _, declaration := range symbol.Declarations {
		if macro = macroTag(declaration); macro != "" {
			break
		}

		if legacyModule.MatchString(ts.GetSourceFileOfNode(declaration).FileName()) {
			name := ""
			if strings.HasPrefix(symbol.Name, "$") {
				name = symbol.Name[1:]
			}
			if macroNames[name] {
				macro = macroName(name)
				break
			}
		}
	}

	state.macroCache[symbol] = macro
	return macro
}

// isMacroFile is whether a file declares macros: imports of it are dropped from the output (nothing in it exists at
// runtime).
func isMacroFile(state *TransformState, file *ts.SourceFile) bool {
	if cached, ok := state.macroFileCache[file.FileName()]; ok {
		return cached
	}

	result := legacyModule.MatchString(file.FileName())
	if !result {
	statements:
		for _, statement := range file.Statements.Nodes {
			if macroTag(statement) != "" {
				result = true
				break
			}
			if statement.Kind == ts.KindVariableStatement {
				for _, declaration := range statement.AsVariableStatement().DeclarationList.AsVariableDeclarationList().Declarations.Nodes {
					if macroTag(declaration) != "" {
						result = true
						break statements
					}
				}
			}
		}
	}

	state.macroFileCache[file.FileName()] = result
	return result
}

// macroOfCall is the macro a call invokes, if any.
func macroOfCall(state *TransformState, node *ts.Node) macroName {
	if symbol := state.getSymbol(node.Expression()); symbol != nil {
		return macroOfSymbol(state, symbol)
	}
	return ""
}

// mayBeMacroCall is true when the callee can be a macro at all (every macro is a `$name`), so plain calls skip the
// symbol lookup.
func mayBeMacroCall(node *ts.Node) bool {
	if node.Kind != ts.KindCallExpression {
		return false
	}
	callee := node.Expression()
	if callee.Kind == ts.KindIdentifier {
		return strings.HasPrefix(callee.Text(), "$")
	}
	if callee.Kind == ts.KindPropertyAccessExpression {
		name := callee.Name()
		return name.Kind == ts.KindIdentifier && strings.HasPrefix(name.Text(), "$")
	}
	return false
}

// sourcePosition is `src/shared/module.ts`, 11 for the (parse tree) node, relative to the project root with forward
// slashes.
func sourcePosition(state *TransformState, node *ts.Node) (file string, line int) {
	original := state.parseTreeNode(node)
	if original == nil {
		original = node
	}
	sourceFile := ts.GetSourceFileOfNode(original)
	if sourceFile == nil {
		return "?", 0
	}
	zeroBased, _ := ts.GetLineAndCharacterOfPosition(sourceFile, ts.GetTokenPosOfNode(original, sourceFile))
	return strings.ReplaceAll(relativePath(state.rootDirectory, sourceFile.FileName()), "\\", "/"), zeroBased + 1
}

// sourcePrefix is `[src/shared/module.ts:11]`, the prefix of every debug macro.
func sourcePrefix(state *TransformState, node *ts.Node) string {
	file, line := sourcePosition(state, node)
	return fmt.Sprintf("[%s:%d]", file, line)
}

func sourceText(state *TransformState, node *ts.Node) string {
	original := state.parseTreeNode(node)
	if original != nil && ts.GetSourceFileOfNode(original) != nil {
		return ts.GetTextOfNode(original)
	}
	return "?"
}

// prefixed is `prefix` followed by the message: one literal when the message is one, else a template (any type
// prints).
func prefixed(state *TransformState, prefix string, message *ts.Node) *ts.Node {
	f := state.f
	if message.Kind == ts.KindStringLiteral || message.Kind == ts.KindNoSubstitutionTemplateLiteral {
		return f.string(prefix + message.Text())
	}
	return f.templateExpression(prefix, message)
}

func transformArguments(state *TransformState, node *ts.Node) []*ts.Node {
	arguments := node.Arguments()
	result := make([]*ts.Node, len(arguments))
	for i, argument := range arguments {
		result[i] = state.transformNode(argument)
	}
	return result
}

func toConvertables(nodes []*ts.Node) []convertableExpression {
	result := make([]convertableExpression, len(nodes))
	for i, node := range nodes {
		result[i] = node
	}
	return result
}

// transformMacroStatement handles the statement forms. `$print(...)` and `$warn(...)` statements vanish without debug
// macros, and a `$dbg(x)` statement prints (or, without debug macros, keeps only a call). ok false: not a
// statement-level macro, transform the expression as usual.
func transformMacroStatement(state *TransformState, node *ts.Node, macro macroName) (statements []*ts.Node, ok bool) {
	f := state.f
	call := node.Expression()
	if macro == macroPrint || macro == macroWarn {
		if state.debugMacros {
			return nil, false
		}
		return []*ts.Node{}, true
	}

	if macro == macroDbg {
		arguments := call.Arguments()
		if len(arguments) == 0 {
			diagnosticsError(call, "$dbg needs an expression")
		}
		if len(arguments) > 1 {
			diagnosticsError(arguments[1], "$dbg's custom handler is not supported")
		}
		expression := arguments[0]
		if state.debugMacros {
			print := f.call("print", f.string(sourcePrefix(state, call)+" "+sourceText(state, expression)+" ="), state.transformNode(expression))
			return []*ts.Node{f.statement(print)}, true
		}
		if expression.Kind == ts.KindCallExpression {
			return []*ts.Node{f.statement(state.transformNode(expression))}, true
		}
		return []*ts.Node{}, true
	}

	return nil, false
}

// transformMacroCall is a macro call in expression position.
func transformMacroCall(state *TransformState, node *ts.Node, macro macroName) *ts.Node {
	f := state.f
	switch macro {
	case macroPrint, macroWarn:
		if !state.debugMacros {
			return f.nil()
		}
		return f.call(string(macro), append([]convertableExpression{f.string(sourcePrefix(state, node))}, toConvertables(transformArguments(state, node))...)...)
	case macroError:
		arguments := transformArguments(state, node)
		if len(arguments) == 0 {
			diagnosticsError(node, "$error needs a message")
		}
		message, rest := arguments[0], arguments[1:]
		text := message
		if state.debugMacros {
			text = prefixed(state, sourcePrefix(state, node)+" ", message)
		}
		return f.call("error", append([]convertableExpression{text}, toConvertables(rest)...)...)
	case macroAssert:
		if len(node.Arguments()) == 0 {
			diagnosticsError(node, "$assert needs a condition")
		}
		condition := node.Arguments()[0]
		arguments := transformArguments(state, node)
		conditionValue := arguments[0]
		var message *ts.Node
		if len(arguments) > 1 {
			message = arguments[1]
		}
		if !state.debugMacros {
			if message != nil {
				return f.call("assert", conditionValue, message)
			}
			return f.call("assert", conditionValue)
		}

		head := sourcePrefix(state, node) + " " + sourceText(state, condition) + ": "
		if message != nil {
			return f.call("assert", conditionValue, prefixed(state, head, message))
		}
		return f.call("assert", conditionValue, f.string(head+"Assertion failed!"))
	case macroDbg:
		arguments := node.Arguments()
		if len(arguments) == 0 {
			diagnosticsError(node, "$dbg needs an expression")
		}
		if len(arguments) > 1 {
			diagnosticsError(arguments[1], "$dbg's custom handler is not supported")
		}
		expression := arguments[0]
		value := state.transformNode(expression)
		if !state.debugMacros {
			return value
		}

		// (() => { const value = <expression>; print("[file:line] <text> =", value); return value; })()
		id := f.identifier("value", true)
		body := f.block([]*ts.Node{
			f.variableStatement(id, value),
			f.statement(f.call("print", f.string(sourcePrefix(state, node)+" "+sourceText(state, expression)+" ="), id)),
			f.returnStatement(id),
		})
		return f.call(f.parenthesized(f.arrowFunction(body)))
	case macroNameof:
		return transformNameof(state, node)
	case macroGit:
		return transformGit(state, node)
	case macroCompileTime:
		return transformCompileTime(state, node)
	case macroFile, macroPackage:
		diagnosticsError(node, "$"+string(macro)+" is a value, not a function")
	}
	panic("unreachable")
}

// kindName is TypeScript's `ts.SyntaxKind[kind]`.
func kindName(kind ts.Kind) string {
	return strings.TrimPrefix(kind.String(), "Kind")
}

func transformNameof(state *TransformState, node *ts.Node) *ts.Node {
	f := state.f
	if typeArguments := node.TypeArguments(); len(typeArguments) > 0 {
		typeArgument := typeArguments[0]
		if typeArgument.Kind == ts.KindTypeQuery {
			diagnosticsError(typeArgument, "$nameof<typeof x>() is not supported; use $nameof(x)")
		}
		if typeArgument.Kind == ts.KindTypeReference || ts.IsKeywordKind(typeArgument.Kind) {
			return f.string(sourceText(state, typeArgument))
		}
		diagnosticsError(typeArgument, "$nameof<T>() does not support "+kindName(typeArgument.Kind))
	}

	arguments := node.Arguments()
	if len(arguments) == 0 {
		diagnosticsError(node, "$nameof needs a type argument or a value")
	}
	argument := arguments[0]
	switch argument.Kind {
	case ts.KindIdentifier, ts.KindStringLiteral:
		return f.string(argument.Text())
	case ts.KindPropertyAccessExpression:
		return f.string(argument.Name().Text())
	case ts.KindThisKeyword:
		if symbol := state.getSymbol(argument); symbol != nil {
			if declaration := symbol.ValueDeclaration; declaration != nil && ts.IsClassLike(declaration) {
				if name := declaration.Name(); name != nil {
					return f.string(name.Text())
				}
				return f.string("<anonymous>")
			}
		}
		diagnosticsError(argument, "$nameof(this) only works inside a class")
	}
	diagnosticsError(argument, "$nameof() does not support "+kindName(argument.Kind))
	panic("unreachable")
}

var gitKeys = []string{"Branch", "Commit", "CommitHash", "LatestTag", "ISODate", "Timestamp"}

func transformGit(state *TransformState, node *ts.Node) *ts.Node {
	f := state.f
	keys := map[string]bool{}
	for _, key := range gitKeys {
		keys[key] = true
	}
	if arguments := node.Arguments(); len(arguments) > 0 {
		requested := map[string]bool{}
		for _, argument := range arguments {
			if argument.Kind != ts.KindStringLiteral || !keys[argument.Text()] {
				diagnosticsError(argument, "$git() takes field names: "+strings.Join(gitKeys, ", "))
			}
			requested[argument.Text()] = true
		}
		keys = requested
	} else if parent := node.Parent; parent != nil && parent.Kind == ts.KindPropertyAccessExpression && parent.Expression() == node {
		// `$git().Branch`: only that field.
		keys = map[string]bool{parent.Name().Text(): true}
	}

	git := state.getGitInfo()
	commit := git.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	values := map[string]*ts.Node{
		"Branch":     f.string(git.Branch),
		"Commit":     f.string(commit),
		"CommitHash": f.string(git.Commit),
		"LatestTag":  f.string(git.LatestTag),
		"ISODate":    f.string(git.ISODate),
		"Timestamp":  f.number(git.Timestamp),
	}
	var properties []*ts.Node
	for _, key := range gitKeys {
		if keys[key] {
			properties = append(properties, f.propertyAssignmentDeclaration(key, values[key]))
		}
	}
	return f.object(properties, false)
}

func transformCompileTime(state *TransformState, node *ts.Node) *ts.Node {
	f := state.f
	var argument *ts.Node
	if arguments := node.Arguments(); len(arguments) > 0 {
		argument = arguments[0]
	}
	kind := ""
	if argument == nil {
		kind = "UnixTimestamp"
	} else if argument.Kind == ts.KindStringLiteral {
		kind = argument.Text()
	}
	now := state.config.Hooks.now()
	switch kind {
	case "UnixTimestamp":
		return f.number(math.Floor(float64(now.UnixMilli()) / 1000))
	case "UnixTimestampMillis":
		return f.number(float64(now.UnixMilli()))
	case "ISO-8601":
		return f.string(toISOString(now))
	case "DateTime":
		return f.bang(f.call(f.field("DateTime", f.identifier("fromIsoDate")), f.string(toISOString(now))))
	}
	at := argument
	if at == nil {
		at = node
	}
	diagnosticsError(at, `$compileTime() takes "UnixTimestamp", "UnixTimestampMillis", "ISO-8601" or "DateTime"`)
	panic("unreachable")
}

// transformMacroValue is `$file` / `$package` as a whole value.
func transformMacroValue(state *TransformState, node *ts.Node, macro macroName) *ts.Node {
	f := state.f
	if macro == macroFile {
		file, line := sourcePosition(state, node)
		return f.object([]*ts.Node{
			f.propertyAssignmentDeclaration("filePath", f.string(file)),
			f.propertyAssignmentDeclaration("lineNumber", f.number(float64(line))),
		}, false)
	}
	if macro == macroPackage {
		return jsonToExpression(state, state.getPackageJson(), node)
	}
	diagnosticsError(node, "$"+string(macro)+" is a function; call it")
	panic("unreachable")
}

// transformMacroField is `$file.lineNumber`, `$package.version`: one field.
func transformMacroField(state *TransformState, node *ts.Node, macro macroName) *ts.Node {
	f := state.f
	field := node.Name().Text()
	if macro == macroFile {
		file, line := sourcePosition(state, node)
		if field == "filePath" {
			return f.string(file)
		}
		if field == "lineNumber" {
			return f.number(float64(line))
		}
		diagnosticsError(node.Name(), `$file has no field "`+field+`" (filePath, lineNumber)`)
	}
	if macro == macroPackage {
		json := state.getPackageJson()
		value, ok := json.get(field)
		if !ok {
			diagnosticsError(node.Name(), `package.json has no field "`+field+`"`)
		}
		return jsonToExpression(state, value, node)
	}
	diagnosticsError(node, "$"+string(macro)+" is a function; call it")
	panic("unreachable")
}

func jsonToExpression(state *TransformState, value any, node *ts.Node) *ts.Node {
	f := state.f
	switch v := value.(type) {
	case string:
		return f.string(v)
	case float64:
		return f.number(v)
	case bool:
		return f.bool(v)
	case nil:
		return f.nil()
	case []any:
		elements := make([]*ts.Node, len(v))
		for i, element := range v {
			elements[i] = jsonToExpression(state, element, node)
		}
		return f.array(elements, false)
	case *jsonObject:
		properties := make([]*ts.Node, len(v.keys))
		for i, key := range v.keys {
			properties[i] = f.propertyAssignmentDeclaration(key, jsonToExpression(state, v.values[key], node))
		}
		return f.object(properties, false)
	}
	diagnosticsError(node, fmt.Sprintf("package.json holds a value the macro can't compile in: %T", value))
	panic("unreachable")
}

// transformMacroImport handles imports from a macro declaration file: dropped, except type-only ones (`import type`,
// `import { type X }`), which roblox-ts erases itself and which may be needed for annotations. ok false: not a macro
// import, leave it alone.
func transformMacroImport(state *TransformState, node *ts.Node) (statements []*ts.Node, ok bool) {
	declaration := node.AsImportDeclaration()
	if declaration.ModuleSpecifier.Kind != ts.KindStringLiteral {
		return nil, false
	}
	clause := declaration.ImportClause
	if clause == nil || clause.IsTypeOnly() {
		return nil, false
	}

	moduleSymbol := state.typeChecker.GetSymbolAtLocation(declaration.ModuleSpecifier)
	if moduleSymbol == nil || len(moduleSymbol.Declarations) == 0 {
		return nil, false
	}
	moduleFile := moduleSymbol.Declarations[0]
	if moduleFile.Kind != ts.KindSourceFile || !isMacroFile(state, moduleFile.AsSourceFile()) {
		return nil, false
	}

	nf := state.f.nf
	bindings := clause.AsImportClause().NamedBindings
	if bindings != nil && bindings.Kind == ts.KindNamedImports {
		var types []*ts.Node
		for _, element := range bindings.AsNamedImports().Elements.Nodes {
			if specifier := element.AsImportSpecifier(); specifier.IsTypeOnly {
				types = append(types, nf.UpdateImportSpecifier(specifier, false, specifier.PropertyName, element.Name()))
			}
		}
		if len(types) > 0 {
			namedImports := nf.UpdateNamedImports(bindings.AsNamedImports(), nf.NewNodeList(types))
			newClause := nf.UpdateImportClause(clause.AsImportClause(), ts.KindTypeKeyword, nil, namedImports)
			return []*ts.Node{
				nf.UpdateImportDeclaration(declaration, declaration.Modifiers(), newClause, declaration.ModuleSpecifier, declaration.Attributes),
			}, true
		}
	}
	return []*ts.Node{}, true
}
