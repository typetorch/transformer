import ts from "typescript";
import { TransformState } from "../../classes/transformState";
import { transformNode } from "../transformNode";
import { transformUserMacro } from "../transformUserMacro";
import { f } from "../../util/factory";
import { macroOfSymbol, transformMacroCall } from "../macros/debugMacros";

export function transformCallExpression(state: TransformState, node: ts.CallExpression) {
	const symbol = state.getSymbol(node.expression);

	if (symbol) {
		const macro = macroOfSymbol(state, symbol);
		if (macro) {
			return transformMacroCall(state, node, macro);
		}

		if (state.isUserMacro(symbol)) {
			// We skip `super()` expressions as we likely do not have enough information to evaluate it.
			if (f.is.superExpression(node.expression)) {
				return state.transform(node);
			}

			const signature = state.typeChecker.getResolvedSignature(node);
			if (signature) {
				return transformUserMacro(state, node, signature) ?? state.transform(node);
			}
		}
	}

	return ts.visitEachChild(node, (node) => transformNode(state, node), state.context);
}
