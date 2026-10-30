/**
 * End-to-end fixture for @typetorch/transformer. Run from the transformer root with `bun run test`, or from here with
 * `bun run fixture`.
 *
 * 1. Builds the transformer and packs it (`bun pm pack`, so only the published `files` are used).
 * 2. Copies ../runtime-kit into framework-standin/src/reflection and compiles the stand-in `@typetorch/framework`
 *    as a roblox-ts package, with the transformer (self-compile: Reflect is imported relatively).
 * 3. Packs the stand-in into node_modules/@typetorch/framework of this Model project and compiles it.
 * 4. Builds the model with Rojo and checks the emitted Luau.
 * 5. Runs the emitted Luau under Lune (scripts/runtime.luau): two generations of the model in one VM, each with a
 *    fresh Reflect registry, decorators, DI ids, guards and macros.
 *
 * Local packages are unpacked into node_modules by hand: Bun can't install `file:` folders on this machine (EPERM),
 * and it caches `file:` tarballs by name, so a rebuilt tarball would never be picked up.
 */
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, cpSync, statSync } from "fs";
import { tmpdir } from "os";
import { join, relative, resolve } from "path";

const fixture = resolve(import.meta.dir, "..");
const root = resolve(fixture, "..");
const standin = join(fixture, "framework-standin");
const packDir = join(fixture, ".pack");

let failures = 0;

function step(name: string) {
	console.log(`\n=== ${name}`);
}

function run(command: string[], cwd: string) {
	console.log(`$ ${command.join(" ")}   (in ${relative(root, cwd) || "."})`);
	const result = Bun.spawnSync(command, { cwd, stdout: "inherit", stderr: "inherit" });
	if (result.exitCode !== 0) {
		console.error(`FAILED (exit ${result.exitCode}): ${command.join(" ")}`);
		process.exit(1);
	}
}

function rbxtsc(cwd: string, ...args: string[]) {
	run(["node", join(cwd, "node_modules", "roblox-ts", "out", "CLI", "cli.js"), ...args], cwd);
}

/** rbxtsc with its output captured (printed too) instead of stopping on failure. */
function rbxtscCaptured(cwd: string, env: Record<string, string>, ...args: string[]) {
	const command = ["node", join(cwd, "node_modules", "roblox-ts", "out", "CLI", "cli.js"), ...args];
	const prefix = Object.entries(env).map(([key, value]) => `${key}=${value} `).join("");
	console.log(`$ ${prefix}${command.join(" ")}   (in ${relative(root, cwd) || "."})`);
	const result = Bun.spawnSync(command, { cwd, stdout: "pipe", stderr: "pipe", env: { ...process.env, ...env } });
	const output = `${result.stdout.toString()}${result.stderr.toString()}`;
	if (output.trim() !== "") console.log(output.trimEnd());
	// Without the colour codes, so checks can match "file.ts:14:3".
	return { exitCode: result.exitCode, output: output.replace(/\x1b\[[0-9;]*m/g, "") };
}

/** `bun pm pack` the package, then unpack it into each destination folder. */
function packInto(packageDir: string, destinations: string[]) {
	rmSync(packDir, { recursive: true, force: true });
	mkdirSync(packDir, { recursive: true });
	run(["bun", "pm", "pack", "--destination", packDir, "--quiet"], packageDir);
	const tarball = readdirSync(packDir).find((name) => name.endsWith(".tgz"));
	if (!tarball) throw new Error(`bun pm pack produced no tarball for ${packageDir}`);

	for (const destination of destinations) {
		rmSync(destination, { recursive: true, force: true });
		mkdirSync(destination, { recursive: true });
		// Relative, forward-slash path: works with both GNU tar (Git Bash) and Windows bsdtar.
		const tarballPath = relative(destination, join(packDir, tarball)).replace(/\\/g, "/");
		run(["tar", "-xzf", tarballPath, "--strip-components=1"], destination);
	}
}

function check(label: string, condition: boolean, detail?: string) {
	console.log(`${condition ? "PASS" : "FAIL"}  ${label}${detail ? `\n      ${detail}` : ""}`);
	if (!condition) failures++;
}

function read(path: string) {
	return readFileSync(path, "utf8");
}

function walk(directory: string, into: string[] = []) {
	for (const name of readdirSync(directory)) {
		const path = join(directory, name);
		if (statSync(path).isDirectory()) walk(path, into);
		else into.push(path);
	}
	return into;
}

function excerpt(source: string, pattern: RegExp, lines = 1) {
	const all = source.split("\n");
	const index = all.findIndex((line) => pattern.test(line));
	return index === -1 ? "(not found)" : all.slice(index, index + lines).join("\n");
}

step("build the transformer");
run(["bun", "run", "build"], root);

step("install fixture dependencies");
for (const directory of [standin, fixture]) {
	if (!existsSync(join(directory, "node_modules"))) run(["bun", "install"], directory);
}

step("pack @typetorch/transformer into the stand-in and the fixture");
packInto(root, [
	join(standin, "node_modules", "@typetorch", "transformer"),
	join(fixture, "node_modules", "@typetorch", "transformer"),
]);

step("compile the stand-in @typetorch/framework (runtime kit + fixture.ts)");
const kitTarget = join(standin, "src", "reflection");
rmSync(kitTarget, { recursive: true, force: true });
cpSync(join(root, "runtime-kit"), kitTarget, { recursive: true });
rmSync(join(standin, "out"), { recursive: true, force: true });
rbxtsc(standin, "--type", "package");

step("pack the stand-in into the fixture");
packInto(standin, [join(fixture, "node_modules", "@typetorch", "framework")]);

step("compile the fixture with the transformer loaded from ../out (its own TypeScript copy, require hook)");
rmSync(join(fixture, "out"), { recursive: true, force: true });
rbxtsc(fixture, "-p", "tsconfig.linked.json");
const linkedOutput = new Map(walk(join(fixture, "out")).map((path) => [relative(fixture, path), read(path)]));

step("network leaf diagnostics: a leaf with no possible guard fails and names the leaf");
const badLeaf = join(fixture, "src", "shared", "bad-leaf.ts");
await Bun.write(
	badLeaf,
	[
		'import { createNetwork } from "@typetorch/framework";',
		"class Weapon {",
		"	damage = 1;",
		"}",
		"interface ClientToServer {",
		"	armory: { equip(weapon: Weapon): void };",
		"}",
		"export const bad = createNetwork<ClientToServer, {}>();",
		"",
	].join("\n"),
);
rmSync(join(fixture, "out"), { recursive: true, force: true });
const badCompile = rbxtscCaptured(fixture, {});
rmSync(badLeaf, { force: true });
check("a leaf whose parameters can't be guarded fails the compile", badCompile.exitCode !== 0);
check(
	"the error names the leaf and lists the supported parameter types",
	badCompile.output.includes('Network leaf "armory.equip": no guard can be generated') &&
		badCompile.output.includes("Supported parameter types:") &&
		badCompile.output.includes("Guards cannot be generated for classes"),
	excerpt(badCompile.output, /Network leaf/, 3),
);

step("compile the fixture (Model project)");
rmSync(join(fixture, "out"), { recursive: true, force: true });
const fixtureCompile = rbxtscCaptured(fixture, {});
if (fixtureCompile.exitCode !== 0) {
	console.error("FAILED: the fixture didn't compile");
	process.exit(1);
}
const warnings = fixtureCompile.output;
check(
	"warning: a generic leaf names the leaf, how its type parameters are checked, and the fix",
	warnings.includes('Network leaf "carry.notify" has a generic or conditional signature') &&
		warnings.includes("T as Model | undefined") &&
		warnings.includes("union of tuples"),
	excerpt(warnings, /carry\.notify/, 2),
);
check(
	"warning: an AnimationTrack parameter never arrives (named, with what to send instead)",
	warnings.includes('Network leaf "patches.looped", parameter "track": an AnimationTrack doesn\'t replicate'),
	excerpt(warnings, /patches\.looped/),
);
check(
	"warning: functions, threads and connections, nested ones too",
	warnings.includes('Network leaf "patches.later", parameter "done": a function can\'t be sent') &&
		warnings.includes('Network leaf "patches.later", parameter "options".co: a thread can\'t be sent') &&
		warnings.includes('Network leaf "patches.later", parameter "options".conn: an RBXScriptConnection can\'t be sent'),
	excerpt(warnings, /patches\.later/, 3),
);
check("no warning for a leaf of supported types", !warnings.includes('"fine"'));
check("the warnings point at the leaf's declaration", /leaf-warnings\.ts:1[0-9]:/.test(warnings), excerpt(warnings, /leaf-warnings\.ts/));
const leafWarnings = read(join(fixture, "out", "shared", "leaf-warnings.luau"));
check(
	"leaves with warnings still get their guards",
	/notify = t\.strictArray\(t\.optional\(t\.instanceIsA\("Model"\)\), t\.optional\(t\.string\)\)/.test(leafWarnings) &&
		/looped = t\.strictArray\(/.test(leafWarnings),
	excerpt(leafWarnings, /notify = /),
);

step("build the model with Rojo");
rmSync(join(fixture, "fixture.rbxm"), { force: true });
run(["rojo", "build", "default.project.json", "--output", "fixture.rbxm"], fixture);

step("check the emitted Luau");
const out = join(fixture, "out");
const network = read(join(out, "shared", "network.luau"));
const services = read(join(out, "server", "services.luau"));
const client = read(join(out, "client", "main.luau"));
const standinFixture = read(join(standin, "out", "fixture.luau"));
const standinReflection = read(join(standin, "out", "reflection", "init.luau"));

const leaves = ["buy", "equip", "drop", "say", "changed", "prices", "notice"];
for (const leaf of leaves) {
	check(`guard for leaf ${leaf}`, new RegExp(`\\b${leaf} = t\\.strictArray\\(`).test(network), excerpt(network, new RegExp(`\\b${leaf} = `)));
}
for (const namespace of ["shop", "inventory", "chat", "stock"]) {
	check(`namespace ${namespace} is a nested table`, new RegExp(`\\b${namespace} = \\{`).test(network));
}
check("network imports t from @rbxts/t", /local t = TS\.import\(.*"@rbxts", "t"/.test(network), excerpt(network, /local t = /));

const loggerId = "@typetorch/framework:fixture@FrameworkLogger";
check(
	"typetorch:parameters lists both dependency ids",
	services.includes(`Reflect.defineMetadata(ShopService, "typetorch:parameters", { "server/services@DataService", "${loggerId}" })`),
	excerpt(services, /typetorch:parameters/),
);
check(
	"a Lazy<T> constructor parameter records lazy:<id of T> (the marker type), next to a plain dependency id",
	services.includes(`Reflect.defineMetadata(TeamService, "typetorch:parameters", { "lazy:server/services@ItemService", "server/services@DataService" })`) &&
		services.includes(`Reflect.defineMetadata(ItemService, "typetorch:parameters", { "lazy:server/services@TeamService" })`),
	excerpt(services, /TeamService, "typetorch:parameters"/),
);
check(
	"decorated class gets an identifier",
	services.includes(`Reflect.defineMetadata(ShopService, "identifier", "server/services@ShopService")`),
	excerpt(services, /"identifier", "server\/services@ShopService"/),
);
check(
	"decorator call is replaced by Reflect.decorate with the package decorator id",
	/Reflect\.decorate\(ShopService, "@typetorch\/framework:decorators@Service", Service, \{ \{\s*loadOrder = 10,?\s*\} \}\)/.test(services),
	excerpt(services, /Reflect\.decorate\(ShopService/),
);
check(
	"Reflect is imported from the stand-in @typetorch/framework",
	/TS\.import\(.*"@typetorch", "framework"/.test(services) && /local Reflect = _framework\.Reflect/.test(services),
	excerpt(services, /"@typetorch", "framework"/),
);
check("idOf<FrameworkLogger>() is the package id", services.includes(`idOf("${loggerId}")`), excerpt(services, /idOf\("/));
check(
	"the package's own compile wrote the same id (stable across packages)",
	standinFixture.includes(`Reflect.defineMetadata(FrameworkLogger, "identifier", "${loggerId}")`),
	excerpt(standinFixture, /"identifier"/),
);
check(
	"the package's own compile imports Reflect relatively",
	/TS\.import\(script, script\.Parent, "reflection", "reflect"\)/.test(standinFixture),
	excerpt(standinFixture, /"reflection", "reflect"/),
);
check(
	"a class extending the package's abstract base with no constructor gets an identifier and no parameters",
	client.includes(`Reflect.defineMetadata(HudController, "identifier", "client/main@HudController")`) &&
		!/defineMetadata\(HudController, "typetorch:parameters"/.test(client),
	excerpt(client, /HudController, "identifier"/),
);
check(
	"a second decorator from the package's decorators module (Controller) is replaced by Reflect.decorate",
	/Reflect\.decorate\(HudController, "@typetorch\/framework:decorators@Controller", Controller, \{\}\)/.test(client),
	excerpt(client, /Reflect\.decorate\(HudController/),
);
check(
	"a derived class's constructor (with super()) lists its injected controller",
	client.includes(`Reflect.defineMetadata(MenuController, "typetorch:parameters", { "client/main@HudController" })`),
	excerpt(client, /MenuController, "typetorch:parameters"/),
);
check(
	"the runtime package re-exports t for generated guards",
	/\bt = TS\.import\(.*"@rbxts", "t"/.test(standinReflection) || /exports\.t = /.test(standinReflection),
	excerpt(standinReflection, /\bt\b/),
);
check("guardOf<Payload>() builds an interface guard", /guardOf\(t\.interface\(/.test(services), excerpt(services, /guardOf\(/));
check("caller macro fills line and text", /here\(\d+, "here\(\)"\)/.test(services), excerpt(services, /here\(/));

step("check the built-in macros (debug macros on)");
const macros = read(join(out, "shared", "macros.luau"));
const debugOn = read(join(out, "shared", "debug-macros.luau"));
check("a dropped macro import keeps the file's header comment above the next statement", /^-- Header comment: stays at the top/m.test(debugOn) && debugOn.indexOf("Header comment") < debugOn.indexOf("local function debugMacros"), excerpt(debugOn, /Header comment/));
check("$print carries the source position", debugOn.includes('print("[src/shared/debug-macros.ts:6]", "print", value)'), excerpt(debugOn, /print\("\[/));
check("$warn carries the source position", debugOn.includes('warn("[src/shared/debug-macros.ts:7]", "warn", value)'), excerpt(debugOn, /warn\("\[/));
check("$git(\"Branch\", \"Commit\") is an object literal with those fields", /local git = \{\s*Branch = "[^"]*",\s*Commit = "[0-9a-f]{0,7}",?\s*\}/.test(debugOn), excerpt(debugOn, /local git = /, 4));
check("$git() has every field", /Timestamp = \d+/.test(debugOn) && /CommitHash = "[0-9a-f]{0,40}"/.test(debugOn) && /ISODate = "/.test(debugOn), excerpt(debugOn, /local full = /, 8));
{
	// ISODate on a real repository, as git-repo-info in rbxts-transform-debug: the author date while the commit's object
	// is loose, and the compile time once it is packed.
	const { readGitInfo } = require(join(root, "out", "util", "functions", "gitInfo.js"));
	const repository = mkdtempSync(join(tmpdir(), "typetorch-git-"));
	const git = (...args: string[]) =>
		Bun.spawnSync(["git", "-c", "user.name=a", "-c", "user.email=a@example.com", "-c", "commit.gpgsign=false", "-c", "gc.auto=0", ...args], {
			cwd: repository,
			env: { ...process.env, GIT_AUTHOR_DATE: "2026-01-02T06:04:05+03:00", GIT_COMMITTER_DATE: "2026-01-09T06:04:05+03:00" },
		});
	git("init", "-q");
	git("commit", "-q", "--allow-empty", "-m", "first");
	const loose = readGitInfo(repository);
	git("gc", "-q");
	const packed = readGitInfo(repository);
	rmSync(repository, { recursive: true, force: true });
	check("$git().ISODate is the author date for a loose commit", loose.isoDate === "2026-01-02T03:04:05.000Z", loose.isoDate);
	check("$git().ISODate is the compile time for a packed commit", packed.isoDate !== loose.isoDate && Math.abs(Date.parse(packed.isoDate) - Date.now()) < 60_000, packed.isoDate);
	check("$git().Timestamp is the author date either way", loose.timestamp === 1767323045 && packed.timestamp === 1767323045, `${loose.timestamp} ${packed.timestamp}`);
}
const exportEquals = read(join(out, "shared", "export-equals.luau"));
check("a macro from a module with `export =` next to types expands", exportEquals.includes('print("[src/shared/export-equals.ts:10]", "print", value)'), excerpt(exportEquals, /print\(/));
check("a dependency from such a module gets the id of its class", exportEquals.includes('"shared/export-equals-service@EqualsService"'), excerpt(exportEquals, /typetorch:parameters/));
check("$compileTime() is a number, $compileTime(\"ISO-8601\") a string", /local built = \d{10}\b/.test(debugOn) && /local iso = "\d{4}-\d\d-\d\dT/.test(debugOn), excerpt(debugOn, /local built = /, 2));
check("$dbg(x) as an expression prints the text and value and yields the value", /local doubled = \(function\(\)[\s\S]*print\("\[src\/shared\/debug-macros\.ts:13\] value \* 2 =", value[\s\S]*return value/.test(debugOn), excerpt(debugOn, /local doubled = /, 5));
check("$dbg(x) as a statement prints", debugOn.includes('print("[src/shared/debug-macros.ts:14] doubled =", doubled)'), excerpt(debugOn, /doubled =", doubled/));
check("$assert with a message", debugOn.includes('"[src/shared/debug-macros.ts:15] value > 0: value must be positive"') && /assert\((_arg0|value > 0), "\[src/.test(debugOn), excerpt(debugOn, /value > 0: /));
check("$assert without a message", debugOn.includes('"[src/shared/debug-macros.ts:16] doubled > 0: Assertion failed!")'), excerpt(debugOn, /doubled > 0: /));
check("$error carries the source position and the level", debugOn.includes('error("[src/shared/debug-macros.ts:17] negative", 2)'), excerpt(debugOn, /error\(/));
check("$nameof<T>() and $nameof(x.y)", debugOn.includes('local name = "GitInfo"') && debugOn.includes('local field = "Branch"'), excerpt(debugOn, /local name = /, 2));
check("$file.lineNumber and $file", debugOn.includes("local line = 20") && /local where = \{\s*filePath = "src\/shared\/debug-macros\.ts",\s*lineNumber = 21,?\s*\}/.test(debugOn), excerpt(debugOn, /local line = /, 5));
check("$package.version is the fixture's version", debugOn.includes('local version = "0.0.0"'), excerpt(debugOn, /local version = /));
check("no import of the macro module remains", !/TS\.import\(.*"transformer"/.test(debugOn) && !/TS\.import\(.*"transformer"/.test(macros) && !/TS\.import\(.*"transformer"/.test(client), excerpt(debugOn, /TS\.import/));
check("$assert in a user macro keeps the position", macros.includes('"[src/shared/macros.ts:10] id: id macro was not filled in by the transformer"'), excerpt(macros, /macros\.ts:10/));
check("$print in a client module", client.includes('print("[src/client/main.ts:9]", "hud started")'), excerpt(client, /hud started/));

step("compile with TYPETORCH_DEBUG_MACROS=0 (prod builds): the debug macros are stripped");
rmSync(join(fixture, "out"), { recursive: true, force: true });
const prodCompile = rbxtscCaptured(fixture, { TYPETORCH_DEBUG_MACROS: "0" });
check("the prod compile succeeds", prodCompile.exitCode === 0, prodCompile.output.trim().split("\n").slice(-5).join("\n      "));
const debugOff = read(join(out, "shared", "debug-macros.luau"));
const macrosOff = read(join(out, "shared", "macros.luau"));
const clientOff = read(join(out, "client", "main.luau"));
check("no source path in any emitted file", !walk(out).some((path) => path.endsWith(".luau") && /\[src\//.test(read(path))), walk(out).filter((path) => /\[src\//.test(read(path))).join(", "));
check("$print and $warn statements are gone", !/\bprint\(/.test(debugOff) && !/\bwarn\(/.test(debugOff) && !/print\(/.test(clientOff), excerpt(debugOff, /print\(|warn\(/));
check("$dbg(x) is x; a $dbg statement vanishes", debugOff.includes("local doubled = value * 2") && !/doubled =", doubled/.test(debugOff), excerpt(debugOff, /local doubled = /));
check("$assert is a plain assert", /assert\(_arg0, "value must be positive"\)|assert\(value > 0, "value must be positive"\)/.test(debugOff) && /assert\(_arg0_1\)|assert\(doubled > 0\)/.test(debugOff), excerpt(debugOff, /assert\(/, 2));
check("$error is a plain error", debugOff.includes('error("negative", 2)'), excerpt(debugOff, /error\(/));
check("$git, $compileTime, $nameof, $file and $package still expand", /Commit = "/.test(debugOff) && /local built = \d{10}\b/.test(debugOff) && debugOff.includes('local name = "GitInfo"') && debugOff.includes("local line = 20") && debugOff.includes('local version = "0.0.0"'), excerpt(debugOff, /local built = /));
check("a user macro's $assert is a plain assert", /assert\(_id ~= "" and _id, "id macro was not filled in by the transformer"\)/.test(macrosOff), excerpt(macrosOff, /assert\(_id/));

step("compile the fixture again with the debug macros on (for Rojo and Lune)");
rmSync(join(fixture, "out"), { recursive: true, force: true });
if (rbxtscCaptured(fixture, {}).exitCode !== 0) {
	console.error("FAILED: the fixture didn't compile");
	process.exit(1);
}
check("tuple labels macro fills parameter names", /argNames\(\{ "player", "itemId" \}\)/.test(services), excerpt(services, /argNames\(/));

// debug-macros.luau holds $compileTime() and changes between compiles.
const luauFiles = walk(out).filter((path) => path.endsWith(".luau") && !path.endsWith("debug-macros.luau"));
const differing = luauFiles.filter((path) => linkedOutput.get(relative(fixture, path)) !== read(path));
check(
	"the linked transformer (require hook) emits identical Luau",
	luauFiles.length > 0 && differing.length === 0,
	differing.length > 0 ? differing.join(", ") : `${luauFiles.length} files compared`,
);

const emitted = [...walk(out), ...walk(join(fixture, "node_modules", "@typetorch", "framework", "out"))];
// Attribution comments ("Adapted from Flamework's Reflect") are fine; imports, keys and markers are not.
const FLAMEWORK = /@flamework|flamework:|_flamework_|rbxts-transformer-flamework/i;
const mentions = emitted.filter((path) => FLAMEWORK.test(read(path)));
check("no @flamework imports, flamework: keys or _flamework_ markers in the emitted code", mentions.length === 0, mentions.join(", "));

const pmLs = Bun.spawnSync(["bun", "pm", "ls", "--all"], { cwd: fixture }).stdout.toString();
check("bun pm ls shows no Flamework packages", pmLs.length > 0 && !/flamework/i.test(pmLs), `${pmLs.trim().split("\n").length} lines listed`);

const model = join(fixture, "fixture.rbxm");
check("rojo built the model", existsSync(model) && statSync(model).size > 0, `${statSync(model).size} bytes`);

step("run the emitted Luau under Lune (two generations, fresh registries)");
const lune = Bun.spawnSync(["lune", "run", "scripts/runtime.luau", "fixture.rbxm"], { cwd: fixture, stdout: "pipe", stderr: "pipe" });
const luneOutput = `${lune.stdout.toString()}${lune.stderr.toString()}`;
for (const line of luneOutput.split("\n")) {
	const match = /^(PASS|FAIL) {2}(.*)$/.exec(line.trimEnd());
	if (match) check(`runtime: ${match[2]}`, match[1] === "PASS");
}
check(
	"the Lune runtime check exited cleanly",
	lune.exitCode === 0,
	lune.exitCode === 0 ? undefined : luneOutput.trim().split("\n").slice(-8).join("\n      "),
);

console.log(failures === 0 ? "\nAll checks passed." : `\n${failures} check(s) failed.`);
process.exit(failures === 0 ? 0 : 1);
