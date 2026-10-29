// Port of src/util/uid.ts.
//
// Ids are stateless and deterministic: the same declaration always gets the same id, whichever project compiles it.
// There is no build-info file and no random salt (unlike rbxts-transformer-flamework).
//
//   - A declaration in the project being compiled: `<module>@<name>`, where `<module>` is its output module path
//     relative to `outDir` (no extension, `index` -> `init`). Packages (roblox-ts `--type package`, or a scoped
//     package name) prefix it with their package name: `<package>:<module>@<name>`.
//   - A declaration in another package (its `.d.ts` files): `<package>:<module>@<name>`, where `<module>` is relative
//     to the directory of the package's `types`/`typings` entry (its outDir for roblox-ts packages).
//
// So `export class Foo` in `@typetorch/framework`'s `src/net/index.ts` is `@typetorch/framework:net/init@Foo` both
// while the framework compiles and when a game sees `node_modules/@typetorch/framework/out/net/index.d.ts`.
//
// `<name>` includes named ancestors (namespaces): `Outer.Inner`.

package transformer

import (
	"path"
	"regexp"
	"strings"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

var indexRegex = regexp.MustCompile(`(^|[/\\])index$`)
var declarationExtRegex = regexp.MustCompile(`(\.d)?\.tsx?$`)
var outputExtRegex = regexp.MustCompile(`(\.lua|\.d\.ts)$`)
var firstDirectoryRegex = regexp.MustCompile(`^(.*?)/`)

func toModulePath(relative string) string {
	return indexRegex.ReplaceAllString(strings.ReplaceAll(relative, "\\", "/"), "${1}init")
}

// getDeclarationRoot is the directory a package's declaration files are rooted at (its outDir, for roblox-ts
// packages).
func getDeclarationRoot(pkg *packageJsonResult) (string, bool) {
	typesEntry, ok := pkg.result.Types.(string)
	if !ok {
		if typesEntry, ok = pkg.result.Typings.(string); !ok {
			return "", false
		}
	}

	typesDirectory := path.Dir(path.Join(pkg.directory, typesEntry))
	if typesDirectory == pkg.directory {
		return "", false
	}

	return typesDirectory, true
}

// getForeignModulePath is the module path of a file inside a package that isn't the one being compiled.
func getForeignModulePath(pkg *packageJsonResult, filePath string) string {
	stripped := declarationExtRegex.ReplaceAllString(filePath, "")
	if declarationRoot, ok := getDeclarationRoot(pkg); ok && isPathDescendantOf(stripped, declarationRoot) {
		return toModulePath(relativePath(declarationRoot, stripped))
	}

	// No usable `types` entry: assume the first directory is the outDir (rbxts-transformer-flamework's convention).
	return firstDirectoryRegex.ReplaceAllString(toModulePath(relativePath(pkg.directory, stripped)), "")
}

func getDeclarationUid(state *TransformState, node *ts.Node) string {
	filePath := state.getSourceFile(node).FileName()
	fullName := getDeclarationName(node)

	if isPathDescendantOfAny(filePath, state.rootDirs) {
		outputPath := outputExtRegex.ReplaceAllString(state.pathTranslator.getOutputPath(filePath), "")
		modulePath := toModulePath(relativePath(state.outDir, outputPath))
		if prefix, ok := state.idPrefix(); ok {
			return prefix + ":" + modulePath + "@" + fullName
		}
		return modulePath + "@" + fullName
	}

	pkg := getPackageJson(path.Dir(filePath))
	packageName := pkg.result.Name
	if packageName == "" {
		packageName = path.Base(pkg.directory)
	}
	return packageName + ":" + getForeignModulePath(pkg, filePath) + "@" + fullName
}

// getSymbolUid reports an error at `trace` (when given) if the symbol has no declaration.
func getSymbolUid(state *TransformState, symbol *ts.Symbol, trace *ts.Node) (string, bool) {
	if symbol.ValueDeclaration != nil {
		return getDeclarationUid(state, symbol.ValueDeclaration), true
	} else if len(symbol.Declarations) > 0 {
		return getDeclarationUid(state, symbol.Declarations[0]), true
	} else if trace != nil {
		diagnosticsError(trace, `Could not find UID for symbol "`+symbol.Name+`"`)
	}
	return "", false
}

// getTypeUid reports an error at `trace` (when given) if the type has no id.
func getTypeUid(state *TransformState, t *ts.Type, trace *ts.Node) (string, bool) {
	if symbol := t.Symbol(); symbol != nil {
		return getSymbolUid(state, symbol, trace)
	} else if isDefinedType(state, t) {
		return "$p:defined", true
	} else if t.Flags()&ts.TypeFlagsIntrinsic != 0 {
		return "$p:" + t.AsIntrinsicType().IntrinsicName(), true
	} else if t.Flags()&ts.TypeFlagsNumberLiteral != 0 {
		value, _ := ts.NumberValue(t.AsLiteralType().Value())
		return "$pn:" + ts.NumberToString(value), true
	} else if t.Flags()&ts.TypeFlagsStringLiteral != 0 {
		return "$ps:" + t.AsLiteralType().Value().(string), true
	} else if trace != nil {
		diagnosticsError(trace, `Could not find UID for type "`+state.typeChecker.TypeToString(t)+`"`)
	}
	return "", false
}

func mustUid(uid string, _ bool) string { return uid }

func getNodeUid(state *TransformState, node *ts.Node) string {
	if isNamedDeclaration(node) {
		return getDeclarationUid(state, node)
	}

	// resolve type aliases to the alias declaration
	if node.Kind == ts.KindTypeReference {
		return getNodeUid(state, node.AsTypeReferenceNode().TypeName)
	} else if node.Kind == ts.KindTypeQuery {
		return getNodeUid(state, node.AsTypeQueryNode().ExprName)
	}

	if symbol := state.getSymbol(node); symbol != nil {
		return mustUid(getSymbolUid(state, symbol, node))
	}

	t := state.typeChecker.GetTypeAtLocation(node)
	return mustUid(getTypeUid(state, t, node))
}
