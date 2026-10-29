# @typetorch/transformer for goblox

A Go port of `@typetorch/transformer` (the TypeScript sources in [`../src`](../src)) for
goblox-ts, which is roblox-ts on typescript-go. It runs natively inside the
goblox compiler as a `goblox.Plugin` and produces the same output as the TypeScript plugin:

- user macros: generated `t` guards and stable type ids
- DI class metadata
- the `$print` / `$warn` / `$git` / ... debug macros

## Status

- **Complete port of the transformer.** Every file of `src/` has a Go counterpart (see [Layout](#layout)), apart from
  the two Node-only helpers listed under [Not ported](#not-ported).
- **Same output as the TypeScript plugin.** The test suite runs both plugins on the same projects and compares their
  output after normalising formatting:
  - `TestFixtures` runs 3 fixture cases: `test-project`, `test-project` with `TYPETORCH_DEBUG_MACROS=0`, and the
    framework stand-in as a package. That is 23 files plus their diagnostics, and all of them match.
  - On **gnome-game** (406 files) the output matches too, with no diagnostics on either side. The Go run, including
    re-type-checking the output, takes about 2 s.
- **Built on the public `pkg/goblox` API** of goblox-ts (v2): `LoadProgram`, `RunPlugins`, `PluginEntries` and
  `RegisterPlugin`. It uses no internal packages and no shim.

## Usage

Importing the package registers it with goblox under its npm name:

```go
import _ "github.com/typetorch/transformer/go"
```

After that, goblox runs it for every tsconfig that lists it, with the same options as the TypeScript plugin:

```jsonc
// tsconfig.json
{
	"compilerOptions": {
		"plugins": [{ "transform": "@typetorch/transformer", "debugMacros": true }]
	}
}
```

To run it by hand:

```go
program, _ := goblox.LoadProgram(goblox.LoadOptions{Project: dir, CurrentDirectory: dir})
config, _ := transformer.ConfigFromProgram(program)
result := goblox.RunPlugins(ctx, program, []goblox.Plugin{transformer.New(config)}, goblox.PluginOptions{CurrentDirectory: dir})
// result.Transformed: file name -> transformed text; result.Program: the re-parsed program; result.Diagnostics
```

### API

| Go | TypeScript |
| --- | --- |
| `New(config TransformerConfig) goblox.Plugin` | `export default function (program, config?: TransformerConfig)` in `src/transformer.ts` |
| `TransformerConfig` (JSON tags as in the tsconfig entry) | `TransformerConfig` in `src/classes/transformState.ts` |
| `TransformerConfig.SemanticDiagnostics`, `NoSemanticDiagnostics`, `DebugMacros`, `RuntimeModule`, `RuntimeReflectPath`, `GuardModule`, `Optimizations.GuardGenerationDedupLimit` | the same options |
| `DefaultRuntimeModule`, `DefaultRuntimeReflectPath` | `DEFAULT_RUNTIME_MODULE`, `DEFAULT_RUNTIME_REFLECT_PATH` |
| `Name` (`"@typetorch/transformer"`) | the package name, the plugin entry's `transform` |
| `ConfigFromProgram(program)`, `ConfigFromPluginEntry(entry)` | roblox-ts passing the tsconfig entry as `config` |
| `TransformerConfig.Hooks` (`Getenv`, `Now`, `Git`, `UUID`) | Go only: replaces the environment, the clock, `git` and uuid for tests; nil means the real one |

Environment variables work as they do in TypeScript: `TYPETORCH_DEBUG_MACROS=0` turns the debug macros off. `--verbose`
logging follows goblox's `TransformContext.Verbose`.

## Layout

Each Go file ports one TypeScript file and is named after its path, with `/` replaced by `_`. For example,
`src/util/functions/gitInfo.ts` becomes `util_functions_gitInfo.go`, and `src/classes/pathTranslator/index.ts` becomes
`classes_pathTranslator_index.go`. Functions keep their TypeScript names (in Go's case), so a fix to the TypeScript
plugin can be ported to the matching Go file by hand.

`index.go` stands for `src/index.ts`. Instead of installing a require hook, it registers the plugin with goblox.

## Not ported

- **`src/index.ts`'s require hook** (and `TYPETORCH_TRANSFORMER_HOOK`). It makes the plugin use roblox-ts's copy of
  TypeScript. goblox has one TypeScript, built in.
- **`src/util/functions/tryResolve.ts`** (Node's `require.resolve`). It only serves the hook. Module resolution goes
  through the program instead (`util_functions_tryResolveTS.go`).
- **`src/util/functions/emitTypescriptMismatch.ts`.** There is no second TypeScript version to mismatch.
- **`parseCommandLine`** takes the project and options from goblox's `TransformContext` instead of `process.argv`.

The files for the first two are comment-only stubs, so the layout stays 1:1.

## Known differences from the TypeScript plugin

These are differences between TypeScript 5 (under the TypeScript plugin) and typescript-go (under goblox), not in the
plugin. The tests allow them:

- **Union member order.** typescript-go orders union members deterministically, with literals sorted by value.
  TypeScript 5 orders them by type id, which is creation order. Where the plugin lists a union's literals, the order
  can differ, for example `t.literalList([1, 2, 3])` instead of `([2, 1, 3])`. Guards don't depend on the order.
- **Printing.** typescript-go's printer prints `x!.y` as `(x!).y` and `f!(a)` as `(f!)(a)`. This only affects printed
  text, not the AST that goblox compiles. It is filed in goblox-ts's `pkg/goblox/REQUESTS.md`.
- **Formatting** (indentation, line breaks, quotes) follows typescript-go's printer.

## Tests

The fixtures live in [`../test-project`](../test-project). Set them up once from the repository root:

```sh
bun install
bun run test   # builds the TS plugin, packs the framework stand-in, runs the TS e2e suite
```

Then run the Go tests:

```sh
cd go
go test ./...
```

`go.mod` replaces `github.com/microsoft/typescript-go` with `../../goblox-ts`, so goblox-ts must be checked out next
to this repository.

The expected output in `testdata/oracle/` comes from the TypeScript plugin, run by `testdata/oracle.cjs` with git, the
clock and uuid pinned to the values in `fixedHooks`. To regenerate it after changing the TypeScript plugin:

```sh
bun run build
node go/testdata/oracle.cjs test-project go/testdata/oracle/fixture
node go/testdata/oracle.cjs test-project go/testdata/oracle/fixture-nodebug --no-debug-macros
node go/testdata/oracle.cjs test-project/framework-standin go/testdata/oracle/standin --type package
```

### Any other project

`TestProject` compares the two plugins on any roblox-ts project that uses the transformer. Run the oracle on a copy
if the project mustn't be touched:

```sh
node go/testdata/oracle.cjs <project> <oracle dir>
cd go
TYPETORCH_PROJECT=<project> TYPETORCH_ORACLE=<oracle dir> [TYPETORCH_PROJECT_TYPE=package] go test -run TestProject
```

Set `GOBLOX_ACTUAL_DIR=<dir>` to also write the Go output there, in the oracle's layout.

Both tests also type-check the re-parsed output and fail if the plugin adds type errors to a file.

## Next steps

- Wire the plugin into the goblox CLI's build (blank import in the `rbxtsc` main) once goblox runs registered plugins
  end to end, then compare the Luau output with roblox-ts on gnome-game.
- Port further TypeScript changes file by file (the layout is 1:1), regenerating the oracles each time.
