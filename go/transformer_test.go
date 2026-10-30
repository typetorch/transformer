package transformer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// fixed mirrors FIXED in testdata/oracle.cjs.
var fixed = struct {
	commit, authorDate, authorTimestamp, branch, tag, uuid string
	now                                                    time.Time
}{
	commit:          "0123456789abcdef0123456789abcdef01234567",
	authorDate:      "2026-01-02T03:04:05+00:00",
	authorTimestamp: "1767323045",
	branch:          "main",
	tag:             "v1.2.3",
	uuid:            "00000000-0000-4000-8000-000000000000",
	now:             time.Date(2026, 3, 4, 5, 6, 7, 890_000_000, time.UTC),
}

func fixedHooks(env map[string]string) Hooks {
	return Hooks{
		Getenv: func(key string) string { return env[key] },
		Now:    func() time.Time { return fixed.now },
		Git: func(cwd string, args ...string) (string, bool) {
			switch command := strings.Join(args, " "); {
			case strings.HasPrefix(command, "log -1"):
				return fixed.commit + "\n" + fixed.authorDate + "\n" + fixed.authorTimestamp, true
			case strings.HasPrefix(command, "rev-parse --git-path objects/"):
				// No such file: the commit counts as packed, so ISODate is the pinned clock.
				return ".git/" + args[2], true
			case command == "rev-parse --abbrev-ref HEAD":
				return fixed.branch, true
			case command == "describe --tags --abbrev=0":
				return fixed.tag, true
			}
			panic("unexpected git call: " + strings.Join(args, " "))
		},
		UUID: func() string { return fixed.uuid },
	}
}

type fixtureCase struct {
	name        string
	project     string
	projectType string
	env         map[string]string
}

var fixtureCases = []fixtureCase{
	{name: "fixture", project: "../test-project"},
	{name: "fixture-nodebug", project: "../test-project", env: map[string]string{"TYPETORCH_DEBUG_MACROS": "0"}},
	{name: "standin", project: "../test-project/framework-standin", projectType: "package"},
}

func formatDiagnostic(projectDir string, diagnostic *ts.Diagnostic, indent string) string {
	categories := map[ts.DiagnosticCategory]string{
		ts.CategoryWarning: "warning", ts.CategoryError: "error", ts.CategorySuggestion: "suggestion", ts.CategoryMessage: "message",
	}
	where := ""
	if file := diagnostic.File(); file != nil {
		line, character := ts.GetLineAndCharacterOfPosition(file, diagnostic.Pos())
		where = fmt.Sprintf("%s:%d:%d+%d: ", relativePath(projectDir, file.FileName()), line+1, character+1, diagnostic.Len())
	}
	out := fmt.Sprintf("%s%s%s: %s\n", indent, where, categories[diagnostic.Category()], diagnostic.MessageText())
	for _, related := range diagnostic.RelatedInformation() {
		out += formatDiagnostic(projectDir, related, indent+"  ")
	}
	return out
}

// runProject transforms a project through goblox (LoadProgram, RunPlugins) and returns its printed files (keyed by
// path relative to rootDir) and diagnostics, formatted like testdata/oracle.cjs does.
func runProject(t *testing.T, c fixtureCase) (map[string]string, string) {
	t.Helper()
	directory, err := filepath.Abs(c.project)
	if err != nil {
		t.Fatal(err)
	}
	directory = ts.NormalizePath(directory)
	program, errs := ts.LoadProgram(ts.LoadOptions{Project: directory, CurrentDirectory: directory, Checkers: 1})
	if program == nil {
		t.Fatalf("LoadProgram: %v", errs)
	}

	config, ok := ConfigFromProgram(program)
	if !ok {
		t.Fatalf("%s: no %s plugin entry", directory, Name)
	}
	config.Hooks = fixedHooks(c.env)

	// What the goblox binary does: the registry makes this plugin from the same tsconfig entry.
	if plugins, errs := ts.PluginsForProgram(program); len(errs) > 0 || len(plugins) != 1 || plugins[0].Name() != Name {
		t.Errorf("PluginsForProgram: %v plugins, errors %v", len(plugins), errs)
	}

	// Every source file under rootDir, like the oracle (and roblox-ts).
	rootDir := program.Options().RootDir
	var sourceFiles []*ts.SourceFile
	for _, file := range ts.CompilableSourceFiles(program) {
		if strings.HasPrefix(file.FileName(), rootDir+"/") {
			sourceFiles = append(sourceFiles, file)
		}
	}

	ctx := context.Background()
	result := ts.RunPlugins(ctx, program, []ts.Plugin{New(config)}, ts.PluginOptions{
		CurrentDirectory: directory,
		ProjectType:      c.projectType,
		Files:            sourceFiles,
	})

	files := map[string]string{}
	for _, file := range sourceFiles {
		text, ok := result.Transformed[file.FileName()]
		if !ok {
			// Unchanged: the oracle prints these too.
			text = ts.NewPrinter(ts.PrinterOptions{}, ts.NewEmitContext()).EmitSourceFile(file)
		}
		files[relativePath(rootDir, file.FileName())] = text
	}

	// What the transformer emits is type-checked again (roblox-ts compiles the re-parsed program): it must not add
	// type errors to the file.
	errorsOf := func(program *ts.Program, name string) []*ts.Diagnostic {
		var errors []*ts.Diagnostic
		for _, diagnostic := range ts.GetPreEmitDiagnostics(ctx, program, program.GetSourceFile(name)) {
			if diagnostic.Category() == ts.CategoryError && diagnostic.File() != nil {
				errors = append(errors, diagnostic)
			}
		}
		return errors
	}
	for name := range result.Transformed {
		if errors := errorsOf(result.Program, name); len(errors) > len(errorsOf(program, name)) {
			var text strings.Builder
			ts.WriteDiagnostics(&text, errors, directory)
			t.Errorf("%s: the transformed file has type errors the original doesn't:\n%s\n--- transformed\n%s", name, text.String(), result.Transformed[name])
		}
	}

	var text strings.Builder
	for _, diagnostic := range result.Diagnostics {
		text.WriteString(formatDiagnostic(directory, diagnostic, ""))
	}
	return files, text.String()
}

// readOracle reads what testdata/oracle.cjs wrote to root.
func readOracle(t *testing.T, root string) (map[string]string, string) {
	t.Helper()
	files := map[string]string{}
	var diagnostics string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Git may check the oracle out with CRLF line endings (core.autocrlf on Windows); TypeScript printed LF.
		text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
		relative, _ := filepath.Rel(root, path)
		if relative == "diagnostics.txt" {
			diagnostics = string(text)
		} else {
			files[filepath.ToSlash(relative)] = string(text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, diagnostics
}

// compareWithOracle runs the case and compares it with the oracle's output in oracleDir. With GOBLOX_ACTUAL_DIR set,
// the Go output is written there too, in the oracle's layout.
func compareWithOracle(t *testing.T, c fixtureCase, oracleDir string) {
	files, diagnostics := runProject(t, c)
	expectedFiles, expectedDiagnostics := readOracle(t, oracleDir)

	if actualDir := os.Getenv("GOBLOX_ACTUAL_DIR"); actualDir != "" {
		for name, text := range files {
			target := filepath.Join(actualDir, c.name, name)
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.WriteFile(target, []byte(text), 0o644)
		}
		_ = os.WriteFile(filepath.Join(actualDir, c.name, "diagnostics.txt"), []byte(diagnostics), 0o644)
	}

	for name, expected := range expectedFiles {
		actual, ok := files[name]
		if !ok {
			t.Errorf("%s: not transformed", name)
			continue
		}
		if normalize(actual) != applyDivergences(c.name+"/"+name, normalize(expected)) {
			t.Errorf("%s differs from the TS transformer's output:\n--- TS\n%s\n--- Go\n%s", name, expected, actual)
		}
	}
	for name := range files {
		if _, ok := expectedFiles[name]; !ok {
			t.Errorf("%s: transformed, but not by the TS transformer", name)
		}
	}
	if diagnostics != expectedDiagnostics {
		t.Errorf("diagnostics differ:\n--- TS\n%s--- Go\n%s", expectedDiagnostics, diagnostics)
	}
}

func TestFixtures(t *testing.T) {
	// The fixtures need their packages (@rbxts types, the packed stand-in @typetorch/framework, the runtime kit
	// copied into the stand-in): without them nothing resolves and the output differs everywhere.
	for _, required := range []string{
		"../test-project/node_modules/@rbxts/types",
		"../test-project/node_modules/@typetorch/framework",
		"../test-project/framework-standin/node_modules/@rbxts/types",
		"../test-project/framework-standin/src/reflection",
	} {
		if _, err := os.Stat(required); err != nil {
			t.Fatalf("%s is missing: run `bun install` and `bun run test` in the repository root once to set up the fixtures", required)
		}
	}
	for _, c := range fixtureCases {
		t.Run(c.name, func(t *testing.T) {
			compareWithOracle(t, c, filepath.Join("testdata", "oracle", c.name))
		})
	}
}

// TestProject compares the Go transformer with the TypeScript one on any roblox-ts project (run testdata/oracle.cjs
// on it first, on a copy if the project mustn't be touched):
//
//	TYPETORCH_PROJECT=<project dir> TYPETORCH_ORACLE=<oracle.cjs output dir> [TYPETORCH_PROJECT_TYPE=package] go test -run TestProject
func TestProject(t *testing.T) {
	project, oracle := os.Getenv("TYPETORCH_PROJECT"), os.Getenv("TYPETORCH_ORACLE")
	if project == "" || oracle == "" {
		t.Skip("TYPETORCH_PROJECT and TYPETORCH_ORACLE are not set")
	}
	compareWithOracle(t, fixtureCase{name: "project", project: project, projectType: os.Getenv("TYPETORCH_PROJECT_TYPE")}, oracle)
}

// knownDivergences are the differences between TypeScript 5.5 (the oracle) and typescript-go that are expected,
// keyed by "<case>/<file>": each pair replaces text in the normalized TS output with what Go produces.
var knownDivergences = map[string][][2]string{
	// typescript-go orders union members deterministically (literals by value); TypeScript 5 by type id, i.e.
	// creation order. `t.literalList` doesn't care.
	"fixture/shared/network.ts":         {{"literalList\"]([2,1,3])", "literalList\"]([1,2,3])"}},
	"fixture-nodebug/shared/network.ts": {{"literalList\"]([2,1,3])", "literalList\"]([1,2,3])"}},
}

func applyDivergences(key string, normalized string) string {
	for _, replacement := range knownDivergences[key] {
		normalized = strings.ReplaceAll(normalized, replacement[0], replacement[1])
	}
	return normalized
}

// normalize removes formatting differences between TypeScript's and typescript-go's printers: whitespace outside
// string and template literals (comments are kept, so a missing or moved comment still shows), line endings, and the
// parentheses typescript-go's printer puts around a non-null assertion that is accessed or called (`(x!).y`,
// `(f!)(a)`; TypeScript prints `x!.y`): it ranks `x!` as a unary expression, below member access.
func normalize(text string) string {
	return unparenthesizeNonNull(stripWhitespace(strings.ReplaceAll(text, "\r\n", "\n")))
}

func stripWhitespace(text string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(text) {
				i++
				out.WriteByte(text[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
			out.WriteByte(c)
		case ' ', '\t', '\n', '\r':
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// unparenthesizeNonNull turns `(x!).y`, `(x!)[i]` and `(x!)(a)` into `x!.y`, `x![i]` and `x!(a)` in whitespace-free
// text. The parentheses' contents are skipped by bracket depth (good enough for test output: brackets inside string
// literals there are balanced or absent).
func unparenthesizeNonNull(text string) string {
	out := []byte(text)
	for i := 0; i+2 < len(out); i++ {
		if out[i] != '!' || out[i+1] != ')' || !strings.ContainsRune(".[(", rune(out[i+2])) {
			continue
		}
		depth, open := 0, -1
		for j := i; j >= 0 && open < 0; j-- {
			switch out[j] {
			case ')', ']', '}':
				depth++
			case '(', '[', '{':
				if depth == 0 {
					open = j
				}
				depth--
			}
		}
		// `f(x!)(a)` calls f's result: the parentheses are a call's, not grouping.
		if open < 0 || out[open] != '(' || isCallee(out[:open]) {
			continue
		}
		out = append(append(append([]byte{}, out[:open]...), out[open+1:i+1]...), out[i+2:]...)
		i -= 2
	}
	return string(out)
}

func TestUnparenthesizeNonNull(t *testing.T) {
	for input, expected := range map[string]string{
		"constt=(this.animator!).Load(a);": "constt=this.animator!.Load(a);",
		"(s.at!)[i]=0;":                    "s.at![i]=0;",
		"return(node.check!)(board);":      "returnnode.check!(board);",
		"((taskasT)[name]!)(task);":        "(taskasT)[name]!(task);",
		"d=(Dep<T>(\"a\")!).x;":            "d=Dep<T>(\"a\")!.x;",
		"f(x!).y;":                         "f(x!).y;",
		"a[0](x!)(b);":                     "a[0](x!)(b);",
		"(x!);":                            "(x!);",
	} {
		if actual := unparenthesizeNonNull(input); actual != expected {
			t.Errorf("unparenthesizeNonNull(%q) = %q, want %q", input, actual, expected)
		}
	}
}

// isCallee reports whether text (whitespace-free) ends with an expression that parentheses after it would call.
func isCallee(text []byte) bool {
	isWordByte := func(c byte) bool {
		return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	end := len(text)
	start := end
	for start > 0 && isWordByte(text[start-1]) {
		start--
	}
	if start < end {
		// The spaces after keywords are gone: `return(x!).y` is `return (x!).y`.
		for _, keyword := range []string{"return", "typeof", "void", "await", "yield", "throw", "delete", "case", "in", "of", "else", "do"} {
			if strings.HasSuffix(string(text[start:end]), keyword) {
				return false
			}
		}
		return true
	}
	return end > 0 && (text[end-1] == ')' || text[end-1] == ']')
}

// ISODate on a real repository, the same as git-repo-info in rbxts-transform-debug: the author date while the commit's
// object is loose, and the compile time once it is packed. Timestamp stays on the commit.
func TestGitInfoLooseAndPacked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	directory := t.TempDir()
	subdirectory := filepath.Join(directory, "packages", "game")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "file.txt"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "user.name=a", "-c", "user.email=a@example.com", "-c", "commit.gpgsign=false", "-c", "gc.auto=0"}, args...)...)
		command.Dir = directory
		command.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-02T06:04:05+03:00", "GIT_COMMITTER_DATE=2026-01-09T06:04:05+03:00")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	mustGit("init", "-q")
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "first")
	linked := filepath.Join(directory, "linked")
	mustGit("worktree", "add", "-q", "--detach", linked)

	const authorDate, authorTimestamp = "2026-01-02T03:04:05.000Z", 1767323045
	check := func(state string, isoDate string) {
		t.Helper()
		for _, cwd := range []string{directory, subdirectory, linked} {
			info := readGitInfo(cwd, runGit, fixed.now)
			if info.ISODate != isoDate || info.Timestamp != authorTimestamp {
				t.Errorf("%s, in %s: ISODate %s, Timestamp %v; want %s, %v", state, cwd, info.ISODate, info.Timestamp, isoDate, authorTimestamp)
			}
		}
	}
	check("loose", authorDate)
	mustGit("gc", "-q")
	check("packed", toISOString(fixed.now))
}
