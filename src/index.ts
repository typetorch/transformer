/* eslint-disable @typescript-eslint/no-var-requires */
import { existsSync, readFileSync, realpathSync } from "fs";
import { Module } from "module";
import path from "path";
import { isPathDescendantOf } from "./util/functions/isPathDescendantOf";
import { Logger } from "./classes/logger";
import { tryResolve } from "./util/functions/tryResolve";

/** Reads the version of the package that owns `modulePath` (TypeScript's main is `lib/typescript.js`). */
function readVersion(modulePath: string): string | undefined {
	try {
		const packageJson = path.join(path.dirname(modulePath), "..", "package.json");
		return JSON.parse(readFileSync(packageJson, "utf8")).version;
	} catch (e) {}
}

/**
 * roblox-ts loads transformers with a plain `require`, so this package could end up with its own copy of TypeScript
 * (for example when it is linked, or installed with a different `typescript` version). Two TypeScript instances do
 * not mix, so while the transformer loads, `require("typescript")` from inside this package is redirected to the
 * copy roblox-ts uses. Same approach as rbxts-transformer-flamework.
 */
const cwd = process.cwd();
const originalRequire = Module.prototype.require;

function shouldTryHooking() {
	// rbxtsc rejects unknown flags, so the switch is an environment variable: 0 disables the hook, 1 forces it.
	const setting = process.env.TYPETORCH_TRANSFORMER_HOOK;
	if (setting === "0") {
		return false;
	}

	if (setting === "1") {
		return true;
	}

	// Ensure we're running in the context of a project and not a multiplace repository or something,
	// as we don't have access to the project directory until roblox-ts invokes the transformer.
	if (
		!existsSync(path.join(cwd, "tsconfig.json")) ||
		!existsSync(path.join(cwd, "package.json")) ||
		!existsSync(path.join(cwd, "node_modules"))
	) {
		return false;
	}

	return true;
}

function hook() {
	const robloxTsPath = tryResolve("roblox-ts", cwd);
	if (!robloxTsPath) {
		return;
	}

	const robloxTsTypeScriptPath = tryResolve("typescript", robloxTsPath);
	if (!robloxTsTypeScriptPath) {
		return;
	}

	// The transformer and roblox-ts are referencing the same TypeScript module.
	const ownTypeScriptPath = tryResolve("typescript", __dirname);
	if (ownTypeScriptPath !== undefined && realpathSync(ownTypeScriptPath) === realpathSync(robloxTsTypeScriptPath)) {
		return;
	}

	const robloxTsTypeScript = require(robloxTsTypeScriptPath);
	const ownVersion = ownTypeScriptPath ? readVersion(ownTypeScriptPath) : undefined;
	if (ownVersion !== undefined && !ownVersion.startsWith(`${robloxTsTypeScript.versionMajorMinor}.`)) {
		if (Logger.verbose) {
			Logger.write("\n");
		}

		Logger.warn(
			"TypeScript version differs",
			`@typetorch/transformer: v${ownVersion}, roblox-ts: v${robloxTsTypeScript.version}`,
			`The transformer will switch to v${robloxTsTypeScript.version}, ` +
				`but you can get rid of this warning by running: bun add -d typescript@${robloxTsTypeScript.version}`,
		);
	}

	if (Logger.verbose) {
		Logger.info(`Using roblox-ts' TypeScript (${robloxTsTypeScriptPath})`);
	}

	Module.prototype.require = function typetorchHook(this: NodeJS.Module, id) {
		// Overwrite any TypeScript imports from this package to roblox-ts' version.
		// To be on the safe side, this won't hook it in other packages.
		if (id === "typescript" && isPathDescendantOf(this.filename, __dirname)) {
			return robloxTsTypeScript;
		}

		return originalRequire.call(this, id);
	} as NodeJS.Require;
}

if (shouldTryHooking()) {
	hook();
}

const transformer = require("./transformer");

// After loading the transformer, we can unhook require.
Module.prototype.require = originalRequire;

export = transformer;
