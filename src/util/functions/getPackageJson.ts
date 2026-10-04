import ts from "typescript";
import path from "path";
import { isPathDescendantOf } from "./isPathDescendantOf";
import { Cache } from "../cache";

export interface PackageJson {
	name?: string;
	version?: string;
	main?: string;
	types?: string;
	typings?: string;
}

export interface PackageJsonResult {
	/** The directory containing the package.json */
	directory: string;
	/** The path to the package.json */
	path: string;
	result: PackageJson;
}

/**
 * Looks recursively at ancestors until a package.json is found
 * @param directory The directory to start under.
 */
export function getPackageJson(directory: string): PackageJsonResult {
	const existing = Cache.pkgJsonCache.get(path.normalize(directory));
	if (existing) return existing;

	const result = getPackageJsonInner(directory);

	Cache.pkgJsonCache.set(path.normalize(directory), result);
	ts.forEachAncestorDirectory(directory, (dir) => {
		if (isPathDescendantOf(dir, result.directory)) {
			Cache.pkgJsonCache.set(path.normalize(dir), result);
		} else {
			return true;
		}
	});

	return result;
}

function getPackageJsonInner(directory: string): PackageJsonResult {
	const packageJsonPath = ts.findPackageJson(directory, ts.sys as never);
	if (!packageJsonPath) throw new Error(`package.json not found in ${directory}`);

	const text = ts.sys.readFile(packageJsonPath);
	const packageJson = text ? JSON.parse(text) : {};

	return {
		directory: path.dirname(packageJsonPath),
		path: packageJsonPath,
		result: packageJson as PackageJson,
	};
}
