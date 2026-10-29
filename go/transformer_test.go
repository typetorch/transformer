package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// runFixture transforms a fixture project and returns its printed files (keyed by path relative to rootDir) and
// diagnostics, formatted like testdata/oracle.cjs does.
func runFixture(t *testing.T, c fixtureCase) (map[string]string, string) {
	t.Helper()
	directory, err := filepath.Abs(c.project)
	if err != nil {
		t.Fatal(err)
	}
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

	// The fixtures don't list `types`: TypeScript 5 then includes every package of the type roots (@rbxts/types,
	// @rbxts/compiler-types), TypeScript 7 none. Until goblox's LoadProgram does what TypeScript 5 did (REQUESTS.md),
	// load the fixture through a tsconfig that extends it and lists them.
	if program.Options().Types == nil {
		tsconfig := filepath.Join(t.TempDir(), "tsconfig.json")
		types, _ := json.Marshal(automaticTypeDirectiveNames(program.Options().TypeRoots))
		text := fmt.Sprintf(`{ "extends": %q, "compilerOptions": { "types": %s } }`, directory+"/tsconfig.json", types)
		if err := os.WriteFile(tsconfig, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		program, errs = ts.LoadProgram(ts.LoadOptions{Project: tsconfig, CurrentDirectory: directory, Checkers: 1})
		if program == nil {
			t.Fatalf("LoadProgram: %v", errs)
		}
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

	// What the transformer emits is type-checked again (roblox-ts compiles the re-parsed program).
	for name := range result.Transformed {
		file := result.Program.GetSourceFile(name)
		var errors []*ts.Diagnostic
		for _, diagnostic := range ts.GetPreEmitDiagnostics(ctx, result.Program, file) {
			if diagnostic.Category() == ts.CategoryError {
				errors = append(errors, diagnostic)
			}
		}
		if len(errors) > 0 {
			var text strings.Builder
			ts.WriteDiagnostics(&text, errors, directory)
			t.Errorf("%s: the transformed file has type errors:\n%s\n--- transformed\n%s", name, text.String(), result.Transformed[name])
		}
	}

	var text strings.Builder
	for _, diagnostic := range result.Diagnostics {
		text.WriteString(formatDiagnostic(directory, diagnostic, ""))
	}
	return files, text.String()
}

func readOracle(t *testing.T, name string) (map[string]string, string) {
	t.Helper()
	root := filepath.Join("testdata", "oracle", name)
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

func TestFixtures(t *testing.T) {
	actualDir := os.Getenv("GOBLOX_ACTUAL_DIR")
	for _, c := range fixtureCases {
		t.Run(c.name, func(t *testing.T) {
			files, diagnostics := runFixture(t, c)
			expectedFiles, expectedDiagnostics := readOracle(t, c.name)

			if actualDir != "" {
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
		})
	}
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
// string and template literals (comments are kept, so a missing or moved comment still shows).
func normalize(text string) string {
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

// automaticTypeDirectiveNames is TypeScript 5's getAutomaticTypeDirectiveNames: every package directory in the type
// roots, except those whose package.json says `"typings": null`.
func automaticTypeDirectiveNames(typeRoots []string) []string {
	var names []string
	for _, root := range typeRoots {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if text, err := os.ReadFile(filepath.Join(root, entry.Name(), "package.json")); err == nil && strings.Contains(string(text), `"typings": null`) {
				continue
			}
			names = append(names, entry.Name())
		}
	}
	return names
}
