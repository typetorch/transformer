import path from "path";
import ts from "typescript";
import { Diagnostics } from "../../classes/diagnostics";
import { TransformState } from "../../classes/transformState";
import { f } from "../../util/factory";

/**
 * The compile-time macros declared in `macros.d.ts` (`import { $print } from "@typetorch/transformer/macros"`), the
 * successor of rbxts-transform-debug's. A declaration opts in with the JSDoc tag `@typetorchMacro <name>`; calls
 * through rbxts-transform-debug's own declarations (same names) are accepted too, so a game can drop that plugin
 * before it rewrites its imports.
 *
 * `print`, `warn`, `error`, `assert` and `dbg` are the debug macros: they compile the source position in
 * (`[src/file.ts:line]`). With `state.debugMacros` off (plugin option `debugMacros: false`, or
 * TYPETORCH_DEBUG_MACROS=0, which the TypeTorch CLI sets for prod builds) they compile to nothing, or to the plain
 * `assert` / `error` call, so no source path reaches a prod payload.
 */
export type MacroName =
	| "print"
	| "warn"
	| "error"
	| "assert"
	| "dbg"
	| "nameof"
	| "file"
	| "package"
	| "git"
	| "compileTime";

const MACRO_NAMES: ReadonlySet<string> = new Set<MacroName>([
	"print",
	"warn",
	"error",
	"assert",
	"dbg",
	"nameof",
	"file",
	"package",
	"git",
	"compileTime",
]);

export const MACRO_TAG = "typetorchMacro";

/** rbxts-transform-debug's declaration file (`node_modules/rbxts-transform-debug/index.d.ts`). */
const LEGACY_MODULE = /[\\/]rbxts-transform-debug[\\/]/;

function macroTag(declaration: ts.Node): MacroName | undefined {
	for (const tag of ts.getJSDocTags(declaration)) {
		if (tag.tagName.text !== MACRO_TAG) continue;
		const name = ts.getTextOfJSDocComment(tag.comment)?.trim();
		if (name !== undefined && MACRO_NAMES.has(name)) return name as MacroName;
	}
}

/** The macro `symbol` declares, if any (`state.getSymbol` has already followed the import alias). */
export function macroOfSymbol(state: TransformState, symbol: ts.Symbol): MacroName | undefined {
	const cached = state.macroCache.get(symbol);
	if (cached !== undefined) return cached ?? undefined;

	let macro: MacroName | undefined;
	for (const declaration of symbol.declarations ?? []) {
		macro = macroTag(declaration);
		if (macro) break;

		if (LEGACY_MODULE.test(declaration.getSourceFile().fileName)) {
			const name = symbol.name.startsWith("$") ? symbol.name.slice(1) : "";
			if (MACRO_NAMES.has(name)) {
				macro = name as MacroName;
				break;
			}
		}
	}

	state.macroCache.set(symbol, macro ?? null);
	return macro;
}

/** Whether a file declares macros: imports of it are dropped from the output (nothing in it exists at runtime). */
export function isMacroFile(state: TransformState, file: ts.SourceFile): boolean {
	const cached = state.macroFileCache.get(file.fileName);
	if (cached !== undefined) return cached;

	const result =
		LEGACY_MODULE.test(file.fileName) ||
		file.statements.some((statement) => {
			if (macroTag(statement)) return true;
			if (ts.isVariableStatement(statement)) {
				return statement.declarationList.declarations.some((declaration) => macroTag(declaration));
			}
			return false;
		});

	state.macroFileCache.set(file.fileName, result);
	return result;
}

/** The macro a call invokes, if any. */
export function macroOfCall(state: TransformState, node: ts.CallExpression): MacroName | undefined {
	const symbol = state.getSymbol(node.expression);
	return symbol ? macroOfSymbol(state, symbol) : undefined;
}

/** True when the callee can be a macro at all (every macro is a `$name`), so plain calls skip the symbol lookup. */
export function mayBeMacroCall(node: ts.Node): node is ts.CallExpression {
	if (!ts.isCallExpression(node)) return false;
	const callee = node.expression;
	if (ts.isIdentifier(callee)) return callee.text.startsWith("$");
	if (ts.isPropertyAccessExpression(callee)) return ts.isIdentifier(callee.name) && callee.name.text.startsWith("$");
	return false;
}

/** `src/shared/module.ts:11` for the (parse tree) node, relative to the project root with forward slashes. */
function sourcePosition(state: TransformState, node: ts.Node): { file: string; line: number } {
	const original = ts.getParseTreeNode(node) ?? node;
	const sourceFile = original.getSourceFile();
	const line = sourceFile ? sourceFile.getLineAndCharacterOfPosition(original.getStart(sourceFile)).line + 1 : 0;
	const file = sourceFile ? path.relative(state.rootDirectory, sourceFile.fileName).replace(/\\/g, "/") : "?";
	return { file, line };
}

/** `[src/shared/module.ts:11]`, the prefix of every debug macro. */
export function sourcePrefix(state: TransformState, node: ts.Node): string {
	const { file, line } = sourcePosition(state, node);
	return `[${file}:${line}]`;
}

function sourceText(node: ts.Node): string {
	const original = ts.getParseTreeNode(node);
	return original && original.getSourceFile() ? original.getText() : "?";
}

/** `prefix` followed by the message: one literal when the message is one, else a template (any type prints). */
function prefixed(prefix: string, message: ts.Expression): ts.Expression {
	if (ts.isStringLiteral(message) || ts.isNoSubstitutionTemplateLiteral(message)) return f.string(prefix + message.text);
	return ts.factory.createTemplateExpression(ts.factory.createTemplateHead(prefix), [
		ts.factory.createTemplateSpan(message, ts.factory.createTemplateTail("")),
	]);
}

function transformArguments(state: TransformState, node: ts.CallExpression): ts.Expression[] {
	return node.arguments.map((argument) => state.transformNode(argument));
}

/**
 * The statement forms. `$print(...)` and `$warn(...)` statements vanish without debug macros, and a `$dbg(x)`
 * statement prints (or, without debug macros, keeps only a call). Undefined: not a statement-level macro, transform
 * the expression as usual.
 */
export function transformMacroStatement(
	state: TransformState,
	node: ts.ExpressionStatement,
	macro: MacroName,
): ts.Statement[] | undefined {
	const call = node.expression as ts.CallExpression;
	if (macro === "print" || macro === "warn") {
		return state.debugMacros ? undefined : [];
	}

	if (macro === "dbg") {
		const expression = call.arguments[0];
		if (!expression) Diagnostics.error(call, "$dbg needs an expression");
		if (call.arguments.length > 1) Diagnostics.error(call.arguments[1], "$dbg's custom handler is not supported");
		if (state.debugMacros) {
			const print = f.call("print", [f.string(`${sourcePrefix(state, call)} ${sourceText(expression)} =`), state.transformNode(expression)]);
			return [f.statement(print)];
		}
		return ts.isCallExpression(expression) ? [f.statement(state.transformNode(expression))] : [];
	}
}

/** A macro call in expression position. */
export function transformMacroCall(state: TransformState, node: ts.CallExpression, macro: MacroName): ts.Expression {
	switch (macro) {
		case "print":
		case "warn": {
			if (!state.debugMacros) return f.nil();
			return f.call(macro, [f.string(sourcePrefix(state, node)), ...transformArguments(state, node)]);
		}
		case "error": {
			const [message, ...rest] = transformArguments(state, node);
			if (!message) Diagnostics.error(node, "$error needs a message");
			const text = state.debugMacros ? prefixed(`${sourcePrefix(state, node)} `, message) : message;
			return f.call("error", [text, ...rest]);
		}
		case "assert": {
			const condition = node.arguments[0];
			if (!condition) Diagnostics.error(node, "$assert needs a condition");
			const [conditionValue, message] = transformArguments(state, node);
			if (!state.debugMacros) return f.call("assert", message ? [conditionValue, message] : [conditionValue]);

			const head = `${sourcePrefix(state, node)} ${sourceText(condition)}: `;
			return f.call("assert", [conditionValue, message ? prefixed(head, message) : f.string(`${head}Assertion failed!`)]);
		}
		case "dbg": {
			const expression = node.arguments[0];
			if (!expression) Diagnostics.error(node, "$dbg needs an expression");
			if (node.arguments.length > 1) Diagnostics.error(node.arguments[1], "$dbg's custom handler is not supported");
			const value = state.transformNode(expression);
			if (!state.debugMacros) return value;

			// (() => { const value = <expression>; print("[file:line] <text> =", value); return value; })()
			const id = f.identifier("value", true);
			const body = f.block([
				f.variableStatement(id, value),
				f.statement(f.call("print", [f.string(`${sourcePrefix(state, node)} ${sourceText(expression)} =`), id])),
				f.returnStatement(id),
			]);
			return f.call(ts.factory.createParenthesizedExpression(f.arrowFunction(body)));
		}
		case "nameof":
			return transformNameof(state, node);
		case "git":
			return transformGit(state, node);
		case "compileTime":
			return transformCompileTime(node);
		case "file":
		case "package":
			Diagnostics.error(node, `$${macro} is a value, not a function`);
	}
}

function transformNameof(state: TransformState, node: ts.CallExpression): ts.Expression {
	const typeArgument = node.typeArguments?.[0];
	if (typeArgument) {
		if (ts.isTypeQueryNode(typeArgument)) {
			Diagnostics.error(typeArgument, `$nameof<typeof x>() is not supported; use $nameof(x)`);
		}
		if (ts.isTypeReferenceNode(typeArgument) || ts.isKeyword(typeArgument.kind)) {
			return f.string(sourceText(typeArgument));
		}
		Diagnostics.error(typeArgument, `$nameof<T>() does not support ${ts.SyntaxKind[typeArgument.kind]}`);
	}

	const argument = node.arguments[0];
	if (!argument) Diagnostics.error(node, "$nameof needs a type argument or a value");
	if (ts.isIdentifier(argument)) return f.string(argument.text);
	if (ts.isPropertyAccessExpression(argument)) return f.string(argument.name.text);
	if (ts.isStringLiteral(argument)) return f.string(argument.text);
	if (argument.kind === ts.SyntaxKind.ThisKeyword) {
		const declaration = state.getSymbol(argument)?.valueDeclaration;
		if (declaration && ts.isClassLike(declaration)) return f.string(declaration.name?.text ?? "<anonymous>");
		Diagnostics.error(argument, "$nameof(this) only works inside a class");
	}
	Diagnostics.error(argument, `$nameof() does not support ${ts.SyntaxKind[argument.kind]}`);
}

const GIT_KEYS = ["Branch", "Commit", "CommitHash", "LatestTag", "ISODate", "Timestamp"] as const;

function transformGit(state: TransformState, node: ts.CallExpression): ts.Expression {
	let keys: ReadonlySet<string> = new Set(GIT_KEYS);
	if (node.arguments.length > 0) {
		const requested = new Set<string>();
		for (const argument of node.arguments) {
			if (!ts.isStringLiteral(argument) || !keys.has(argument.text)) {
				Diagnostics.error(argument, `$git() takes field names: ${GIT_KEYS.join(", ")}`);
			}
			requested.add(argument.text);
		}
		keys = requested;
	} else if (ts.isPropertyAccessExpression(node.parent) && node.parent.expression === node) {
		// `$git().Branch`: only that field.
		keys = new Set([node.parent.name.text]);
	}

	const git = state.getGitInfo();
	const values: Record<(typeof GIT_KEYS)[number], ts.Expression> = {
		Branch: f.string(git.branch),
		Commit: f.string(git.commit.slice(0, 7)),
		CommitHash: f.string(git.commit),
		LatestTag: f.string(git.latestTag),
		ISODate: f.string(git.isoDate),
		Timestamp: f.number(git.timestamp),
	};
	return f.object(
		GIT_KEYS.filter((key) => keys.has(key)).map((key) => f.propertyAssignmentDeclaration(key, values[key])),
		false,
	);
}

function transformCompileTime(node: ts.CallExpression): ts.Expression {
	const argument = node.arguments[0];
	const kind = argument === undefined ? "UnixTimestamp" : ts.isStringLiteral(argument) ? argument.text : undefined;
	const now = new Date();
	switch (kind) {
		case "UnixTimestamp":
			return f.number(Math.floor(now.getTime() / 1000));
		case "UnixTimestampMillis":
			return f.number(now.getTime());
		case "ISO-8601":
			return f.string(now.toISOString());
		case "DateTime":
			return f.bang(f.call(f.field("DateTime", f.identifier("fromIsoDate")), [f.string(now.toISOString())]));
		default:
			Diagnostics.error(argument ?? node, `$compileTime() takes "UnixTimestamp", "UnixTimestampMillis", "ISO-8601" or "DateTime"`);
	}
}

/** `$file` / `$package` as a whole value. */
export function transformMacroValue(state: TransformState, node: ts.Identifier, macro: MacroName): ts.Expression {
	if (macro === "file") {
		const { file, line } = sourcePosition(state, node);
		return f.object([f.propertyAssignmentDeclaration("filePath", f.string(file)), f.propertyAssignmentDeclaration("lineNumber", f.number(line))], false);
	}
	if (macro === "package") {
		return jsonToExpression(state.getPackageJson(), node);
	}
	Diagnostics.error(node, `$${macro} is a function; call it`);
}

/** `$file.lineNumber`, `$package.version`: one field. */
export function transformMacroField(state: TransformState, node: ts.PropertyAccessExpression, macro: MacroName): ts.Expression {
	const field = node.name.text;
	if (macro === "file") {
		const { file, line } = sourcePosition(state, node);
		if (field === "filePath") return f.string(file);
		if (field === "lineNumber") return f.number(line);
		Diagnostics.error(node.name, `$file has no field "${field}" (filePath, lineNumber)`);
	}
	if (macro === "package") {
		const json = state.getPackageJson();
		if (!(field in json)) Diagnostics.error(node.name, `package.json has no field "${field}"`);
		return jsonToExpression(json[field], node);
	}
	Diagnostics.error(node, `$${macro} is a function; call it`);
}

function jsonToExpression(value: unknown, node: ts.Node): ts.Expression {
	if (typeof value === "string") return f.string(value);
	if (typeof value === "number") return f.number(value);
	if (typeof value === "boolean") return f.bool(value);
	if (value === null || value === undefined) return f.nil();
	if (Array.isArray(value)) return f.array(value.map((element) => jsonToExpression(element, node)), false);
	if (typeof value === "object") {
		return f.object(
			Object.entries(value).map(([key, element]) => f.propertyAssignmentDeclaration(key, jsonToExpression(element, node))),
			false,
		);
	}
	Diagnostics.error(node, `package.json holds a value the macro can't compile in: ${typeof value}`);
}

/**
 * Imports from a macro declaration file: dropped, except type-only ones (`import type`, `import { type X }`), which
 * roblox-ts erases itself and which may be needed for annotations. Undefined: not a macro import, leave it alone.
 */
export function transformMacroImport(state: TransformState, node: ts.ImportDeclaration): ts.Statement[] | undefined {
	if (!ts.isStringLiteral(node.moduleSpecifier)) return;
	const clause = node.importClause;
	if (!clause || clause.isTypeOnly) return;

	const moduleSymbol = state.typeChecker.getSymbolAtLocation(node.moduleSpecifier);
	const moduleFile = moduleSymbol?.declarations?.[0];
	if (!moduleFile || !ts.isSourceFile(moduleFile) || !isMacroFile(state, moduleFile)) return;

	if (clause.namedBindings && ts.isNamedImports(clause.namedBindings)) {
		const types = clause.namedBindings.elements.filter((element) => element.isTypeOnly);
		if (types.length > 0) {
			return [
				ts.factory.updateImportDeclaration(
					node,
					node.modifiers,
					ts.factory.updateImportClause(clause, true, undefined, ts.factory.updateNamedImports(clause.namedBindings, types.map((element) => ts.factory.updateImportSpecifier(element, false, element.propertyName, element.name)))),
					node.moduleSpecifier,
					node.attributes,
				),
			];
		}
	}
	return [];
}
