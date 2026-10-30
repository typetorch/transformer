// Port of src/transformations/expressions/transformCallExpression.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func transformCallExpression(state *TransformState, node *ts.Node) *ts.Node {
	if !state.mayBeMacro(node.Expression()) {
		return state.visitor.VisitEachChild(node)
	}

	if symbol := state.getSymbol(node.Expression()); symbol != nil {
		if macro := macroOfSymbol(state, symbol); macro != "" {
			return transformMacroCall(state, node, macro)
		}

		if state.isUserMacro(symbol) {
			// We skip `super()` expressions as we likely do not have enough information to evaluate it.
			if node.Expression().Kind == ts.KindSuperKeyword {
				return state.transform(node)
			}

			if signature := state.typeChecker.GetResolvedSignature(node); signature != nil {
				if result := transformUserMacro(state, node, signature); result != nil {
					return result
				}
				return state.transform(node)
			}
		}
	}

	return state.visitor.VisitEachChild(node)
}
