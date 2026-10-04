import ts from "typescript";
import type { TransformState } from "../../classes/transformState";

/** Resolves a module the way the TypeScript program does (types first). */
export function tryResolveTS(state: TransformState, moduleName: string, path: string): string | undefined {
	const module = ts.resolveModuleName(moduleName, path, state.options, ts.sys);
	return module.resolvedModule?.resolvedFileName;
}
