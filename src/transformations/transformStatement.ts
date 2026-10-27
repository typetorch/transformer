import ts from "typescript";
import { TransformState } from "../classes/transformState";
import { catchDiagnostic } from "../util/diagnosticsUtils";
import { getNodeList } from "../util/functions/getNodeList";
import { transformClassDeclaration } from "./statements/transformClassDeclaration";
import { transformNode } from "./transformNode";
import { macroOfCall, mayBeMacroCall, transformMacroImport, transformMacroStatement } from "./macros/debugMacros";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const TRANSFORMERS = new Map<ts.SyntaxKind, (state: TransformState, node: any) => ts.Statement | ts.Statement[]>([
	[ts.SyntaxKind.ClassDeclaration, transformClassDeclaration],
	[ts.SyntaxKind.ExpressionStatement, transformExpressionStatement],
	[ts.SyntaxKind.ImportDeclaration, transformImportDeclaration],
]);

function visitChildren<T extends ts.Node>(state: TransformState, node: T): T {
	return ts.visitEachChild(node, (newNode) => transformNode(state, newNode), state.context);
}

/** A `$print(...)` / `$warn(...)` / `$dbg(...)` statement may compile to nothing (see debugMacros.ts). */
function transformExpressionStatement(state: TransformState, node: ts.ExpressionStatement): ts.Statement | ts.Statement[] {
	if (mayBeMacroCall(node.expression)) {
		const macro = macroOfCall(state, node.expression);
		if (macro) {
			const result = transformMacroStatement(state, node, macro);
			if (result) return result;
		}
	}
	return visitChildren(state, node);
}

/** Imports of the macro declaration file are dropped: nothing in it exists at runtime. */
function transformImportDeclaration(state: TransformState, node: ts.ImportDeclaration): ts.Statement | ts.Statement[] {
	return transformMacroImport(state, node) ?? visitChildren(state, node);
}

export function transformStatement(state: TransformState, statement: ts.Statement): ts.Statement | ts.Statement[] {
	return catchDiagnostic<ts.Statement | ts.Statement[]>(statement, () => {
		const [node, prereqs] = state.capture(() => {
			const transformer = TRANSFORMERS.get(statement.kind);
			if (transformer) {
				return transformer(state, statement);
			}

			return ts.visitEachChild(statement, (newNode) => transformNode(state, newNode), state.context);
		});

		return [...prereqs, ...getNodeList(node)];
	});
}
