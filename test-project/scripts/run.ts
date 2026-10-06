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
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, cpSync, statSync } from "fs";
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
function rbxtscCaptured(cwd: string, ...args: string[]) {
	const command = ["node", join(cwd, "node_modules", "roblox-ts", "out", "CLI", "cli.js"), ...args];
	console.log(`$ ${command.join(" ")}   (in ${relative(root, cwd) || "."})`);
	const result = Bun.spawnSync(command, { cwd, stdout: "pipe", stderr: "pipe" });
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
const badCompile = rbxtscCaptured(fixture);
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
const fixtureCompile = rbxtscCaptured(fixture);
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
check("tuple labels macro fills parameter names", /argNames\(\{ "player", "itemId" \}\)/.test(services), excerpt(services, /argNames\(/));

const luauFiles = walk(out).filter((path) => path.endsWith(".luau"));
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
