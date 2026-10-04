import ts from "typescript";
import path from "path";
import { transformNode } from "../transformations/transformNode";
import { getPackageJson } from "../util/functions/getPackageJson";
import { f } from "../util/factory";
import { parseCommandLine } from "../util/functions/parseCommandLine";
import { createPathTranslator } from "../util/functions/createPathTranslator";
import { NodeMetadata } from "./nodeMetadata";
import { PathTranslator } from "./pathTranslator";
import { assert } from "../util/functions/assert";
import { tryResolveTS } from "../util/functions/tryResolveTS";
import { Diagnostics } from "./diagnostics";

/** The package generated code imports the runtime (`Reflect`, and `t` as a fallback) from. */
export const DEFAULT_RUNTIME_MODULE = "@typetorch/framework";

/** Inside the runtime package itself: the rootDir-relative file (no extension) that exports `Reflect`. */
export const DEFAULT_RUNTIME_REFLECT_PATH = "reflection/reflect";

/** roblox-ts treats a project whose package name matches this as a package (see createProjectData). */
const PACKAGE_REGEX = /^@[a-z0-9-]*\//;

/**
 * Options from the plugin entry in tsconfig.json:
 * `{ "transform": "@typetorch/transformer", ...options }`
 */
export interface TransformerConfig {
	/**
	 * Disables TypeScript's own semantic diagnostics.
	 * Improves performance, but results in increased risk of incorrect compilation as well as messed up diagnostic spans.
	 */
	noSemanticDiagnostics?: boolean;

	/**
	 * The module generated code imports `Reflect` from (and `t`, when `@rbxts/t` can't be imported directly).
	 * Defaults to `@typetorch/framework`.
	 */
	runtimeModule?: string;

	/**
	 * Only used while compiling the runtime package itself (package name === `runtimeModule`): the file that exports
	 * `Reflect`, relative to `rootDir`, without extension. Generated code imports it relatively instead of importing
	 * the package from inside itself. Defaults to `reflection/reflect`.
	 */
	runtimeReflectPath?: string;

	/**
	 * The module generated guards import `t` from. By default `@rbxts/t` is used when the file resolves the same copy
	 * the runtime package uses, otherwise `t` is imported from `runtimeModule` (which re-exports it).
	 */
	guardModule?: string;

	/**
	 * Some experimental optimizations
	 */
	optimizations?: {
		/**
		 * Types used at least this many times inside one generated guard are hoisted into a local and reused.
		 */
		guardGenerationDedupLimit?: number;
	};
}

export class TransformState {
	public parsedCommandLine = parseCommandLine();
	public currentDirectory = this.parsedCommandLine.project;
	public options = this.program.getCompilerOptions();
	public srcDir = this.options.rootDir ?? this.currentDirectory;
	public outDir = this.options.outDir ?? this.currentDirectory;
	public rootDirs = this.options.rootDirs ? this.options.rootDirs : [this.srcDir];
	public typeChecker = this.program.getTypeChecker();

	public pathTranslator: PathTranslator;

	/** The directory holding the project's package.json */
	public rootDirectory: string;
	public packageName: string;

	/**
	 * Whether the project compiles as a roblox-ts package. Ids of a package's own declarations are prefixed with the
	 * package name, the same id consumers compute from its published declaration files.
	 */
	public isPackage: boolean;

	public runtimeModule: string;
	public isRuntimePackage: boolean;

	public isUserMacroCache = new Map<ts.Symbol, boolean>();
	private guardLibraryPath?: string;

	constructor(
		public program: ts.Program,
		public context: ts.TransformationContext,
		public config: TransformerConfig,
	) {
		const { result: packageJson, directory } = getPackageJson(this.currentDirectory);
		assert(packageJson.name, `package.json in ${directory} has no name`);

		this.rootDirectory = directory;
		this.packageName = packageJson.name;
		this.pathTranslator = createPathTranslator(this.program);

		const projectType = this.parsedCommandLine.type;
		this.isPackage =
			projectType !== undefined ? projectType === "package" : PACKAGE_REGEX.test(this.packageName);

		this.runtimeModule = config.runtimeModule ?? DEFAULT_RUNTIME_MODULE;
		this.isRuntimePackage = this.packageName === this.runtimeModule;
	}

	/** The prefix for ids of this project's own declarations (`<prefix>:<path>@<name>`), if any. */
	get idPrefix(): string | undefined {
		return this.isPackage ? this.packageName : undefined;
	}

	isUserMacro(symbol: ts.Symbol) {
		const cached = this.isUserMacroCache.get(symbol);
		if (cached !== undefined) return cached;

		if (symbol.declarations) {
			for (const declaration of symbol.declarations) {
				const metadata = new NodeMetadata(this, declaration);
				if (metadata.isRequested("macro")) {
					this.isUserMacroCache.set(symbol, true);
					return true;
				}
			}
		}

		this.isUserMacroCache.set(symbol, false);
		return false;
	}

	public fileImports = new Map<string, ImportInfo[]>();
	addFileImport(file: ts.SourceFile, importPath: string, name: string): ts.Identifier {
		let importInfos = this.fileImports.get(file.fileName);
		if (!importInfos) this.fileImports.set(file.fileName, (importInfos = []));

		let importInfo = importInfos.find((x) => x.path === importPath);
		if (!importInfo) importInfos.push((importInfo = { path: importPath, entries: [] }));

		let identifier = importInfo.entries.find((x) => x.name === name)?.identifier;

		if (!identifier) {
			start: for (const statement of file.statements) {
				if (!f.is.importDeclaration(statement)) break;
				if (!f.is.string(statement.moduleSpecifier)) continue;
				if (!f.is.importClauseDeclaration(statement.importClause)) continue;
				if (!f.is.namedImports(statement.importClause.namedBindings)) continue;
				if (statement.importClause.isTypeOnly) continue;
				if (statement.moduleSpecifier.text !== importPath) continue;

				for (const importElement of statement.importClause.namedBindings.elements) {
					if (importElement.isTypeOnly) {
						continue;
					}

					if (importElement.propertyName) {
						if (importElement.propertyName.text === name) {
							identifier = importElement.name;
							break start;
						}
					} else {
						if (importElement.name.text === name) {
							identifier = importElement.name;
							break start;
						}
					}
				}
			}
		}

		if (!identifier) {
			importInfo.entries.push({ name, identifier: (identifier = f.identifier(name, true)) });
		}

		return identifier;
	}

	/**
	 * Imports `Reflect` from the runtime package. While compiling the runtime package itself, the import points at
	 * the file that defines it instead (a package can't import itself by name).
	 */
	getReflect(file: ts.SourceFile): ts.Identifier {
		if (!this.isRuntimePackage) {
			return this.addFileImport(file, this.runtimeModule, "Reflect");
		}

		const reflectFile = path.join(this.srcDir, this.config.runtimeReflectPath ?? DEFAULT_RUNTIME_REFLECT_PATH);
		let importPath = path.relative(path.dirname(file.fileName), reflectFile).replace(/\\/g, "/");
		if (!importPath.startsWith(".")) importPath = `./${importPath}`;

		return this.addFileImport(file, importPath, "Reflect");
	}

	/**
	 * Imports `t` (the guard library) into the file.
	 */
	getGuardLibrary(file: ts.SourceFile): ts.Identifier {
		if (this.config.guardModule) {
			return this.addFileImport(file, this.config.guardModule, "t");
		}

		if (this.guardLibraryPath) {
			return this.addFileImport(file, this.guardLibraryPath, "t");
		}

		const fileGuardPath = tryResolveTS(this, "@rbxts/t", path.dirname(file.fileName));
		if (this.isRuntimePackage) {
			this.guardLibraryPath = "@rbxts/t";
			return this.addFileImport(file, this.guardLibraryPath, "t");
		}

		const runtimePath = tryResolveTS(this, this.runtimeModule, file.fileName);
		if (runtimePath === undefined) {
			if (fileGuardPath === undefined) {
				Diagnostics.warning(
					file.endOfFileToken,
					`Neither @rbxts/t nor ${this.runtimeModule} was found, guard generation may not work.`,
				);
			}
			return this.addFileImport(file, "@rbxts/t", "t");
		}

		const runtimeGuardPath = tryResolveTS(this, "@rbxts/t", runtimePath);
		if (fileGuardPath !== undefined && fileGuardPath === runtimeGuardPath) {
			// The runtime package and this project use the same @rbxts/t.
			this.guardLibraryPath = "@rbxts/t";
		} else {
			// Use the runtime's own copy, re-exported from its entry point.
			this.guardLibraryPath = this.runtimeModule;
		}

		return this.addFileImport(file, this.guardLibraryPath, "t");
	}

	getSourceFile(node: ts.Node) {
		const parseNode = ts.getParseTreeNode(node);
		if (!parseNode) throw new Error(`Could not find parse tree node`);

		return ts.getSourceFileOfNode(parseNode);
	}

	getSymbol(node: ts.Node, followAlias = true): ts.Symbol | undefined {
		if (f.is.namedDeclaration(node)) {
			return this.getSymbol(node.name);
		}

		const symbol = this.typeChecker.getSymbolAtLocation(node);

		if (symbol && followAlias) {
			return ts.skipAlias(symbol, this.typeChecker);
		} else {
			return symbol;
		}
	}

	addDiagnostic(diag: ts.DiagnosticWithLocation) {
		this.context.addDiagnostic(diag);
	}

	private prereqStack = new Array<Array<ts.Statement>>();
	capture<T>(cb: () => T): [T, ts.Statement[]] {
		this.prereqStack.push([]);
		const result = cb();
		return [result, this.prereqStack.pop()!];
	}

	prereq(statement: ts.Statement) {
		const stack = this.prereqStack[this.prereqStack.length - 1];
		if (stack) stack.push(statement);
	}

	prereqList(statements: ts.Statement[]) {
		const stack = this.prereqStack[this.prereqStack.length - 1];
		if (stack) stack.push(...statements);
	}

	isCapturing(threshold = 1) {
		return this.prereqStack.length > threshold;
	}

	transform<T extends ts.Node>(node: T): T {
		return ts.visitEachChild(node, (newNode) => transformNode(this, newNode), this.context);
	}

	transformNode<T extends ts.Node>(node: T): T {
		// Technically this isn't guaranteed to return `T`, and TypeScript 5.0+ updated the signature to disallow this,
		// but we don't care so we'll just cast it.
		return ts.visitNode(node, (newNode) => transformNode(this, newNode)) as T;
	}
}

interface ImportItem {
	name: string;
	identifier: ts.Identifier;
}

interface ImportInfo {
	path: string;
	entries: Array<ImportItem>;
}
