import path from "path";
import ts from "typescript";
import { green, Logger } from "../../classes/logger";
import { TransformState } from "../../classes/transformState";
import { getPackageJson } from "./getPackageJson";
import { isPathDescendantOf } from "./isPathDescendantOf";
import { tryResolve } from "./tryResolve";

function emitMessages(messages: string[]): never {
	Logger.writeLine(...messages);
	process.exit(1);
}

/**
 * Spits out information about the mismatch.
 * This should only be called after a mismatch is detected.
 */
export function emitTypescriptMismatch(state: TransformState, baseMessage: string): never {
	const messages = [baseMessage];

	// Check if they have a local install.
	const robloxTsPath = tryResolve("roblox-ts", state.rootDirectory);
	if (!robloxTsPath) {
		messages.push(
			"It is recommended that you use a local install of roblox-ts.",
			`You can install a local version using ${green("bun add -d roblox-ts")}`,
		);
		emitMessages(messages);
	}

	// Check if they've used a global install.
	if (require.main) {
		if (!isPathDescendantOf(require.main.filename, path.join(state.rootDirectory, "node_modules"))) {
			messages.push(
				"It appears you've run the transformer using a global install.",
				`You can run using the locally installed version using ${green("bunx rbxtsc")}`,
			);
			emitMessages(messages);
		}
	}

	// They're using a local install
	// but they're using the wrong TypeScript version.
	const robloxTsTypeScript = tryResolve("typescript", robloxTsPath);
	if (robloxTsTypeScript) {
		const typescriptPackage = getPackageJson(path.dirname(robloxTsTypeScript));
		const requiredVersion = typescriptPackage.result.version;
		if (requiredVersion && ts.version !== requiredVersion) {
			messages.push(
				`@typetorch/transformer is using TypeScript version ${ts.version}`,
				`roblox-ts requires TypeScript version ${requiredVersion}`,
				`You can fix this by setting your TypeScript version: ${green(`bun add -d typescript@${requiredVersion}`)}`,
			);
		}
	}

	emitMessages(messages);
}
