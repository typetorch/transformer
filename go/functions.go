// Ports of the small helpers in src/util/functions/: assert.ts, getDeclarationName.ts, getDeclarationOfType.ts,
// getInstanceTypeFromType.ts, getNodeList.ts, getPackageJson.ts (and src/util/cache.ts), gitInfo.ts,
// isDefinedType.ts, isPathDescendantOf.ts, isTupleType.ts, parseCommandLine.ts, tryResolveTS.ts.
//
// Not ported: tryResolve.ts and emitTypescriptMismatch.ts (Node's require and TypeScript version checks: goblox has
// one TypeScript, built in), and src/index.ts (the require hook that makes the transformer use roblox-ts' TypeScript).

package transformer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

/// assert.ts

// assert panics (a transformer bug, reported like any other crash) when value is false.
func assert(value bool, message ...string) {
	if !value {
		panic(fmt.Sprintf("Assertion Failed! %s\nPlease submit a bug report here:\nhttps://github.com/typetorch/transformer/issues",
			strings.Join(message, "")))
	}
}

/// getDeclarationName.ts

// getDeclarationName calculates a name including all named ancestors, such as Enum.Material.Air.
func getDeclarationName(node *ts.Node) string {
	name := node.Name()
	if name == nil || name.Kind != ts.KindIdentifier {
		return "$p:error"
	}
	result := name.Text()
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if isNamedDeclaration(parent) {
			result = ts.GetTextOfNode(parent.Name()) + "." + result
		}
	}
	return result
}

// isNamedDeclaration is TypeScript's `ts.isNamedDeclaration` / `f.is.namedDeclaration`: any node with a `name`.
func isNamedDeclaration(node *ts.Node) bool {
	return node != nil && node.Name() != nil
}

/// getDeclarationOfType.ts

func getDeclarationOfType(t *ts.Type) *ts.Node {
	if symbol := t.Symbol(); symbol != nil && len(symbol.Declarations) > 0 {
		return symbol.Declarations[0]
	}
	return nil
}

/// getInstanceTypeFromType.ts

var nominalRegex = regexp.MustCompile(`_nominal_(.*)`)

func getInstanceTypeFromType(state *TransformState, file *ts.SourceFile, t *ts.Type) *ts.Type {
	checker := state.typeChecker
	assert(checker.GetPropertyOfType(t, "_nominal_Instance") != nil, "non instance type was passed into getInstanceTypeFromType")

	diagnosticsLocation := getDeclarationOfType(t)
	if diagnosticsLocation == nil {
		diagnosticsLocation = file.AsNode()
	}
	nominalProperties := getNominalProperties(state, t)

	specificType, specificTypeCount := t, 0
	for _, property := range nominalProperties {
		match := nominalRegex.FindStringSubmatch(property.Name)
		assert(match != nil)
		noNominalName := match[1]

		instanceSymbol := checker.ResolveName(noNominalName, nil, ts.SymbolFlagsType, false)
		if instanceSymbol == nil || len(instanceSymbol.Declarations) == 0 {
			continue
		}

		instanceType := checker.GetTypeAtLocation(instanceSymbol.Declarations[0])
		subNominalProperties := getNominalProperties(state, instanceType)

		if len(subNominalProperties) > specificTypeCount {
			specificType = instanceType
			specificTypeCount = len(subNominalProperties)
		}
	}

	// intersection between two nominal types?
	for _, property := range nominalProperties {
		if checker.GetPropertyOfType(specificType, property.Name) == nil {
			diagnosticsError(diagnosticsLocation, "Intersection between nominal types is forbidden.")
		}
	}

	return specificType
}

func getNominalProperties(state *TransformState, t *ts.Type) []*ts.Symbol {
	var result []*ts.Symbol
	for _, property := range state.typeChecker.GetPropertiesOfType(t) {
		if strings.HasPrefix(property.Name, "_nominal_") {
			result = append(result, property)
		}
	}
	return result
}

/// getNodeList.ts

// getNodeList flattens what a statement transform returned (a node, or a SyntaxList of them).
func getNodeList(node *ts.Node) []*ts.Node {
	if node == nil {
		return nil
	}
	if node.Kind == ts.KindSyntaxList {
		return node.AsSyntaxList().Children
	}
	return []*ts.Node{node}
}

/// getPackageJson.ts, cache.ts

type packageJson struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Main    string `json:"main"`
	Types   any    `json:"types"`
	Typings any    `json:"typings"`
}

type packageJsonResult struct {
	// directory is the directory containing the package.json.
	directory string
	// path is the path to the package.json.
	path   string
	result packageJson
}

// pkgJsonCache is TypeScript's global `Cache.pkgJsonCache`: only reset when the process restarts.
var pkgJsonCache sync.Map // directory -> *packageJsonResult

// getPackageJson looks at `directory` and its ancestors until a package.json is found.
func getPackageJson(directory string) *packageJsonResult {
	directory = path.Clean(directory)
	if existing, ok := pkgJsonCache.Load(directory); ok {
		return existing.(*packageJsonResult)
	}

	result := getPackageJsonInner(directory)

	pkgJsonCache.Store(directory, result)
	for dir := directory; ; dir = path.Dir(dir) {
		if !isPathDescendantOf(dir, result.directory) {
			break
		}
		pkgJsonCache.Store(dir, result)
		if dir == "/" {
			break
		}
	}

	return result
}

func getPackageJsonInner(directory string) *packageJsonResult {
	for dir := directory; ; dir = path.Dir(dir) {
		packageJsonPath := path.Join(dir, "package.json")
		if text, err := os.ReadFile(packageJsonPath); err == nil {
			result := &packageJsonResult{directory: dir, path: packageJsonPath}
			_ = json.Unmarshal(text, &result.result)
			return result
		}
		if dir == "/" || dir == "." {
			panic(fmt.Sprintf("package.json not found in %s", directory))
		}
	}
}

/// gitInfo.ts

// GitInfo is what `$git()` compiles in. Empty strings (and the current time) when the project is not a git
// repository.
type GitInfo struct {
	Branch    string
	Commit    string
	LatestTag string
	ISODate   string
	Timestamp float64
}

// gitRunner runs git in cwd and returns its trimmed output; ok is false when git failed.
type gitRunner func(cwd string, args ...string) (output string, ok bool)

func runGit(cwd string, args ...string) (string, bool) {
	command := exec.Command("git", args...)
	command.Dir = cwd
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

var commitRegex = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digitsRegex = regexp.MustCompile(`^\d+$`)

// readGitInfo reads the repository `cwd` is in (three git calls; the caller caches the result per compile).
func readGitInfo(cwd string, git gitRunner, now time.Time) GitInfo {
	var head []string
	if output, ok := git(cwd, "log", "-1", "--format=%H%n%aI%n%at"); ok {
		head = strings.Split(output, "\n")
	}
	at := func(i int) string {
		if i < len(head) {
			return head[i]
		}
		return ""
	}
	commit := ""
	if commitRegex.MatchString(at(0)) {
		commit = at(0)
	}
	timestamp := float64(now.UnixMilli()) / 1000
	timestamp = float64(int64(timestamp + 0.5)) // Math.round
	if commit != "" && digitsRegex.MatchString(at(2)) {
		timestamp, _ = strconv.ParseFloat(at(2), 64)
	}
	isoDate := toISOString(now)
	if commit != "" && at(1) != "" {
		if date, err := time.Parse(time.RFC3339, at(1)); err == nil {
			isoDate = toISOString(date)
		}
	}
	// A detached HEAD prints "HEAD"; rbxts-transform-debug compiled "" in for it.
	branch, latestTag := "", ""
	if commit != "" {
		branch, _ = git(cwd, "rev-parse", "--abbrev-ref", "HEAD")
		latestTag, _ = git(cwd, "describe", "--tags", "--abbrev=0")
	}
	if branch == "HEAD" {
		branch = ""
	}
	return GitInfo{Branch: branch, Commit: commit, LatestTag: latestTag, ISODate: isoDate, Timestamp: timestamp}
}

// toISOString is JavaScript's Date.prototype.toISOString.
func toISOString(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

/// isDefinedType.ts

// isDefinedType: https://github.com/roblox-ts/roblox-ts/blob/dc74f34fdab3caf20d65db080cf2dbf5c4f38fdc/src/TSTransformer/util/types.ts#L70
func isDefinedType(state *TransformState, t *ts.Type) bool {
	checker := state.typeChecker
	return t.Flags() == ts.TypeFlagsObject &&
		len(checker.GetPropertiesOfType(t)) == 0 &&
		len(checker.GetSignaturesOfType(t, ts.SignatureKindCall)) == 0 &&
		len(checker.GetSignaturesOfType(t, ts.SignatureKindConstruct)) == 0 &&
		checker.GetNumberIndexType(t) == nil &&
		checker.GetStringIndexType(t) == nil
}

/// isPathDescendantOf.ts

// isPathDescendantOf checks if the `filePath` path is a descendant of the `dirPath` path.
func isPathDescendantOf(filePath string, dirPath string) bool {
	return dirPath == filePath || !strings.HasPrefix(relativePath(dirPath, filePath), "..")
}

// isPathDescendantOfAny checks if `filePath` is a descendant of any of `dirPaths`.
func isPathDescendantOfAny(filePath string, dirPaths []string) bool {
	for _, dirPath := range dirPaths {
		if isPathDescendantOf(filePath, dirPath) {
			return true
		}
	}
	return false
}

// relativePath is Node's path.relative for absolute forward-slash paths.
func relativePath(from string, to string) string {
	from, to = path.Clean(from), path.Clean(to)
	if from == to {
		return ""
	}
	fromParts := splitPath(from)
	toParts := splitPath(to)
	common := 0
	for common < len(fromParts) && common < len(toParts) && fromParts[common] == toParts[common] {
		common++
	}
	parts := make([]string, 0, len(fromParts)-common+len(toParts)-common)
	for range fromParts[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, toParts[common:]...)
	return strings.Join(parts, "/")
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

/// isTupleType.ts

func isTupleType(state *TransformState, t *ts.Type) bool {
	return ts.IsTupleType(t)
}

func isArrayType(state *TransformState, t *ts.Type) bool {
	return state.typeChecker.IsArrayType(t)
}

/// parseCommandLine.ts

// commandLine is what the transformer reads from rbxtsc's command line. In goblox it comes from the
// TransformContext rather than argv.
type commandLine struct {
	tsconfigPath string
	project      string
	// projectType is the roblox-ts `--type` option, if it was passed.
	projectType string
}

/// tryResolveTS.ts

// tryResolveTS resolves a module the way the TypeScript program does (types first). Like TypeScript's
// resolveModuleName, resolution starts in the directory of `containingFile`.
func tryResolveTS(state *TransformState, moduleName string, containingFile string) string {
	return state.moduleResolver().ResolveModuleName(moduleName, containingFile)
}
