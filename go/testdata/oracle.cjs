// Oracle for the Go port: runs the TypeScript transformer (../../out, i.e. `bun run build` output) over a fixture
// project and writes what it produces, so the Go tests can compare against it.
//
//   node go/testdata/oracle.cjs <project dir> <output dir> [--type package] [--no-debug-macros]
//
// For every source file of the project: `<output dir>/<path relative to rootDir>.ts` is the transformed file printed
// by TypeScript's printer, and `<output dir>/diagnostics.txt` lists the transformer's diagnostics. Git, the clock and
// random UUIDs are pinned (see FIXED below, mirrored in the Go tests) so the output is deterministic.
"use strict";
const fs = require("fs");
const path = require("path");

const [projectArg, outputArg, ...flags] = process.argv.slice(2);
if (!projectArg || !outputArg) {
	console.error("usage: node oracle.cjs <project dir> <output dir> [--type package] [--no-debug-macros]");
	process.exit(2);
}
const projectDir = path.resolve(projectArg);
const outputDir = path.resolve(outputArg);
const transformerDir = path.resolve(__dirname, "..", "..");

const FIXED = {
	commit: "0123456789abcdef0123456789abcdef01234567",
	authorDate: "2026-01-02T03:04:05+00:00",
	authorTimestamp: "1767323045",
	branch: "main",
	tag: "v1.2.3",
	now: Date.parse("2026-03-04T05:06:07.890Z"),
	uuid: "00000000-0000-4000-8000-000000000000",
};

// Pin git (src/util/functions/gitInfo.ts calls execFileSync("git", ...)).
const childProcess = require("child_process");
const realExecFileSync = childProcess.execFileSync;
childProcess.execFileSync = function (file, args, options) {
	if (file !== "git") return realExecFileSync.apply(this, arguments);
	const command = args.join(" ");
	if (command.startsWith("log -1")) return `${FIXED.commit}\n${FIXED.authorDate}\n${FIXED.authorTimestamp}\n`;
	// No such file: the commit counts as packed, so ISODate is the pinned clock.
	if (command.startsWith("rev-parse --git-path objects/")) return `.git/${args[2]}\n`;
	if (command === "rev-parse --abbrev-ref HEAD") return `${FIXED.branch}\n`;
	if (command === "describe --tags --abbrev=0") return `${FIXED.tag}\n`;
	throw new Error(`unexpected git call: ${command}`);
};

// Pin the clock ($compileTime, $git without a repository) and UUIDs (Modding.Caller<"uuid">).
const RealDate = Date;
global.Date = class extends RealDate {
	constructor(...args) {
		if (args.length === 0) super(FIXED.now);
		else super(...args);
	}
	static now() {
		return FIXED.now;
	}
};
require("crypto").randomUUID = () => FIXED.uuid;

// The transformer reads `-p`, `--type` and `--verbose` from argv and the project from the working directory.
const typeIndex = flags.indexOf("--type");
const projectType = typeIndex === -1 ? undefined : flags[typeIndex + 1];
process.chdir(projectDir);
process.argv = [process.argv[0], "rbxtsc", "-p", "tsconfig.json", ...(projectType ? ["--type", projectType] : [])];
if (flags.includes("--no-debug-macros")) process.env.TYPETORCH_DEBUG_MACROS = "0";
else delete process.env.TYPETORCH_DEBUG_MACROS;

// The TypeScript roblox-ts uses (the transformer's require hook picks the same one).
const robloxTs = require.resolve("roblox-ts", { paths: [projectDir] });
const ts = require(require.resolve("typescript", { paths: [path.dirname(robloxTs)] }));
const transformer = require(path.join(transformerDir, "out", "index.js")).default;

const configPath = path.join(projectDir, "tsconfig.json");
const parsed = ts.getParsedCommandLineOfConfigFile(configPath, {}, { ...ts.sys, onUnRecoverableConfigFileDiagnostic: () => {} });
if (!parsed) throw new Error(`could not parse ${configPath}`);
const program = ts.createProgram({ rootNames: parsed.fileNames, options: parsed.options });
const pluginConfig = (parsed.options.plugins ?? []).find((plugin) => plugin.transform) ?? {};

const rootDir = parsed.options.rootDir;
const sourceFiles = program.getSourceFiles().filter((file) => !file.isDeclarationFile && file.fileName.startsWith(rootDir + "/"));
const result = ts.transform(sourceFiles, [transformer(program, pluginConfig)], parsed.options);
const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed });

fs.rmSync(outputDir, { recursive: true, force: true });
for (const file of result.transformed) {
	const relative = path.relative(rootDir, file.fileName);
	const target = path.join(outputDir, relative);
	fs.mkdirSync(path.dirname(target), { recursive: true });
	fs.writeFileSync(target, printer.printFile(file));
}

function formatDiagnostic(diagnostic, indent) {
	const category = ts.DiagnosticCategory[diagnostic.category].toLowerCase();
	let where = "";
	if (diagnostic.file) {
		const { line, character } = diagnostic.file.getLineAndCharacterOfPosition(diagnostic.start);
		where = `${path.relative(projectDir, diagnostic.file.fileName).split(path.sep).join("/")}:${line + 1}:${character + 1}+${diagnostic.length}: `;
	}
	const text = ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");
	let out = `${indent}${where}${category}: ${text}\n`;
	for (const related of diagnostic.relatedInformation ?? []) out += formatDiagnostic(related, indent + "  ");
	return out;
}

const diagnostics = (result.diagnostics ?? []).map((diagnostic) => formatDiagnostic(diagnostic, "")).join("");
fs.writeFileSync(path.join(outputDir, "diagnostics.txt"), diagnostics);
result.dispose();
console.log(`${path.relative(transformerDir, outputDir)}: ${result.transformed.length} files, ${(result.diagnostics ?? []).length} diagnostics`);
