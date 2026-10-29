// Port of src/transformations/transformNode.ts.

package transformer

import (
	"fmt"
	"runtime/debug"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

// transformNode returns a node, or a SyntaxList of statements where TypeScript returns an array.
func transformNode(state *TransformState, node *ts.Node) (result *ts.Node) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*diagnosticError); !ok {
				diagnosticsError(node, fmt.Sprintf("@typetorch/transformer failed here\n%v\n%s", r, debug.Stack()))
			}
			panic(r)
		}
	}()

	if ts.IsExpression(node) {
		return transformExpression(state, node)
	} else if ts.IsStatement(node) {
		return transformStatement(state, node)
	}

	return state.visitor.VisitEachChild(node)
}
