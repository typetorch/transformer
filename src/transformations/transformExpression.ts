import ts from "typescript";
import { TransformState } from "../classes/transformState";
import { catchDiagnostic } from "../util/diagnosticsUtils";
import { transformCallExpression } from "./expressions/transformCallExpression";
import { transformNewExpression } from "./expressions/transformNewExpression";
import { transformNode } from "./transformNode";
import { macroOfSymbol, transformMacroField, transformMacroValue } from "./macros/debugMacros";
import { f } from "../util/factory";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const TRANSFORMERS = new Map<ts.SyntaxKind, (state: TransformState, node: any) => ts.Expression>([
	[ts.SyntaxKind.CallExpression, transformCallExpression],
	[ts.SyntaxKind.NewExpression, transformNewExpression],
	[ts.SyntaxKind.Identifier, transformIdentifier],
	[ts.SyntaxKind.PropertyAccessExpression, transformPropertyAccessExpression],
]);

/** The built-in value macro (`$file`, `$package`) an identifier refers to, if any; only `$names` get a symbol lookup. */
function valueMacro(state: TransformState, node: ts.Identifier) {
	if (!node.text.startsWith("$")) return;
	const parent = node.parent;
	if (parent && (ts.isImportSpecifier(parent) || ts.isExportSpecifier(parent) || ts.isImportClause(parent))) return;
	if (parent && ts.isPropertyAccessExpression(parent) && parent.name === node) return;
	if (parent && ts.isPropertyAssignment(parent) && parent.name === node) return;
	const symbol = state.getSymbol(node);
	return symbol ? macroOfSymbol(state, symbol) : undefined;
}

function transformIdentifier(state: TransformState, node: ts.Identifier): ts.Expression {
	const macro = valueMacro(state, node);
	return macro ? transformMacroValue(state, node, macro) : node;
}

function transformPropertyAccessExpression(state: TransformState, node: ts.PropertyAccessExpression): ts.Expression {
	if (f.is.identifier(node.expression) && !node.questionDotToken) {
		const macro = valueMacro(state, node.expression);
		if (macro === "file" || macro === "package") return transformMacroField(state, node, macro);
	}
	return ts.visitEachChild(node, (newNode) => transformNode(state, newNode), state.context);
}

export function transformExpression(state: TransformState, expression: ts.Expression): ts.Expression {
	return catchDiagnostic(expression, () => {
		const transformer = TRANSFORMERS.get(expression.kind);
		if (transformer) {
			return transformer(state, expression);
		}
		return ts.visitEachChild(expression, (newNode) => transformNode(state, newNode), state.context);
	});
}
