import path from "path";
import ts from "typescript";
import { Diagnostics } from "../classes/diagnostics";
import { TransformState } from "../classes/transformState";
import { f } from "./factory";
import { getDeclarationName } from "./functions/getDeclarationName";
import { getPackageJson, PackageJsonResult } from "./functions/getPackageJson";
import { isDefinedType } from "./functions/isDefinedType";
import { isPathDescendantOf, isPathDescendantOfAny } from "./functions/isPathDescendantOf";

/*
 * Ids are stateless and deterministic: the same declaration always gets the same id, whichever project compiles it.
 * There is no build-info file and no random salt (unlike rbxts-transformer-flamework).
 *
 * - A declaration in the project being compiled: `<module>@<name>`, where `<module>` is its output module path
 *   relative to `outDir` (no extension, `index` -> `init`). Packages (roblox-ts `--type package`, or a scoped package
 *   name) prefix it with their package name: `<package>:<module>@<name>`.
 * - A declaration in another package (its `.d.ts` files): `<package>:<module>@<name>`, where `<module>` is relative
 *   to the directory of the package's `types`/`typings` entry (its outDir for roblox-ts packages).
 *
 * So `export class Foo` in `@typetorch/framework`'s `src/net/index.ts` is `@typetorch/framework:net/init@Foo` both
 * while the framework compiles and when a game sees `node_modules/@typetorch/framework/out/net/index.d.ts`.
 *
 * `<name>` includes named ancestors (namespaces): `Outer.Inner`.
 */

const INDEX_REGEX = /(^|[\/\\])index$/;

function toModulePath(relativePath: string) {
	return relativePath.replace(/\\/g, "/").replace(INDEX_REGEX, "$1init");
}

/** The directory a package's declaration files are rooted at (its outDir, for roblox-ts packages). */
function getDeclarationRoot(pkg: PackageJsonResult): string | undefined {
	const typesEntry = pkg.result.types ?? pkg.result.typings;
	if (typeof typesEntry !== "string") return;

	const typesDirectory = path.dirname(path.join(pkg.directory, typesEntry));
	if (typesDirectory === pkg.directory) return;

	return typesDirectory;
}

/**
 * The module path of a file inside a package that isn't the one being compiled.
 */
function getForeignModulePath(pkg: PackageJsonResult, filePath: string) {
	const stripped = filePath.replace(/(\.d)?\.tsx?$/, "");
	const declarationRoot = getDeclarationRoot(pkg);
	if (declarationRoot !== undefined && isPathDescendantOf(stripped, declarationRoot)) {
		return toModulePath(path.relative(declarationRoot, stripped));
	}

	// No usable `types` entry: assume the first directory is the outDir (rbxts-transformer-flamework's convention).
	return toModulePath(path.relative(pkg.directory, stripped)).replace(/^(.*?)\//, "");
}

export function getDeclarationUid(state: TransformState, node: ts.NamedDeclaration) {
	const filePath = state.getSourceFile(node).fileName;
	const fullName = getDeclarationName(node);

	if (isPathDescendantOfAny(filePath, state.rootDirs)) {
		const outputPath = state.pathTranslator.getOutputPath(filePath).replace(/(\.lua|\.d\.ts)$/, "");
		const modulePath = toModulePath(path.relative(state.outDir, outputPath));
		const prefix = state.idPrefix;
		return prefix !== undefined ? `${prefix}:${modulePath}@${fullName}` : `${modulePath}@${fullName}`;
	}

	const pkg = getPackageJson(path.dirname(filePath));
	const packageName = pkg.result.name ?? path.basename(pkg.directory);
	return `${packageName}:${getForeignModulePath(pkg, filePath)}@${fullName}`;
}

export function getSymbolUid(state: TransformState, symbol: ts.Symbol, trace: ts.Node): string;
export function getSymbolUid(state: TransformState, symbol: ts.Symbol, trace?: ts.Node): string | undefined;
export function getSymbolUid(state: TransformState, symbol: ts.Symbol, trace?: ts.Node) {
	if (symbol.valueDeclaration) {
		return getDeclarationUid(state, symbol.valueDeclaration);
	} else if (symbol.declarations?.[0]) {
		return getDeclarationUid(state, symbol.declarations[0]);
	} else if (trace) {
		Diagnostics.error(trace, `Could not find UID for symbol "${symbol.name}"`);
	}
}

export function getTypeUid(state: TransformState, type: ts.Type, trace: ts.Node): string;
export function getTypeUid(state: TransformState, type: ts.Type, trace?: ts.Node): string | undefined;
export function getTypeUid(state: TransformState, type: ts.Type, trace?: ts.Node) {
	if (type.symbol) {
		return getSymbolUid(state, type.symbol, trace);
	} else if (isDefinedType(type)) {
		return `$p:defined`;
	} else if (type.flags & ts.TypeFlags.Intrinsic) {
		return `$p:${(type as ts.IntrinsicType).intrinsicName}`;
	} else if (type.flags & ts.TypeFlags.NumberLiteral) {
		return `$pn:${(type as ts.NumberLiteralType).value}`;
	} else if (type.flags & ts.TypeFlags.StringLiteral) {
		return `$ps:${(type as ts.StringLiteralType).value}`;
	} else if (trace) {
		Diagnostics.error(trace, `Could not find UID for type "${type.checker.typeToString(type)}"`);
	}
}

export function getNodeUid(state: TransformState, node: ts.Node): string {
	if (f.is.namedDeclaration(node)) {
		return getDeclarationUid(state, node);
	}

	// resolve type aliases to the alias declaration
	if (f.is.referenceType(node)) {
		return getNodeUid(state, node.typeName);
	} else if (f.is.queryType(node)) {
		return getNodeUid(state, node.exprName);
	}

	const symbol = state.getSymbol(node);
	if (symbol) {
		return getSymbolUid(state, symbol, node);
	}

	const type = state.typeChecker.getTypeAtLocation(node);
	return getTypeUid(state, type, node);
}
