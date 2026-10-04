import { PackageJsonResult } from "./functions/getPackageJson";

export interface Cache {
	pkgJsonCache: Map<string, PackageJsonResult>;
}

/**
 * Global cache that is only reset when rbxtsc is restarted.
 */
export const Cache: Cache = {
	pkgJsonCache: new Map(),
};
