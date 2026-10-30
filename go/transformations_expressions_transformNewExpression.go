// Port of src/transformations/expressions/transformNewExpression.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func transformNewExpression(state *TransformState, node *ts.Node) *ts.Node {
	if !state.mayBeMacro(node.Expression()) {
		return state.visitor.VisitEachChild(node)
	}

	if symbol := state.getSymbol(node.Expression()); symbol != nil {
		if state.isUserMacro(symbol) {
			if signature := state.typeChecker.GetResolvedSignature(node); signature != nil {
				if result := transformUserMacro(state, node, signature); result != nil {
					return result
				}
				return state.transform(node)
			}
		}
	}

	return state.transform(node)
}
