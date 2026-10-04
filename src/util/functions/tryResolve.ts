/**
 * Resolves a module the way Node would from `path`. Must not import TypeScript: the entry point uses it before it
 * decides which TypeScript instance to load.
 */
export function tryResolve(moduleName: string, path: string): string | undefined {
	try {
		return require.resolve(moduleName, { paths: [path] });
	} catch (e) {}
}
