// Port of src/transformations/transformExpression.ts.

package transformer

import (
	"strings"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

var expressionTransformers = map[ts.Kind]func(state *TransformState, node *ts.Node) *ts.Node{}

func init() {
	expressionTransformers[ts.KindCallExpression] = transformCallExpression
	expressionTransformers[ts.KindNewExpression] = transformNewExpression
	expressionTransformers[ts.KindIdentifier] = transformIdentifier
	expressionTransformers[ts.KindPropertyAccessExpression] = transformPropertyAccessExpression
}

// valueMacro is the built-in value macro (`$file`, `$package`) an identifier refers to, if any; only `$names` get a
// symbol lookup.
func valueMacro(state *TransformState, node *ts.Node) macroName {
	if !strings.HasPrefix(node.Text(), "$") {
		return ""
	}
	if parent := node.Parent; parent != nil {
		switch parent.Kind {
		case ts.KindImportSpecifier, ts.KindExportSpecifier, ts.KindImportClause:
			return ""
		case ts.KindPropertyAccessExpression, ts.KindPropertyAssignment:
			if parent.Name() == node {
				return ""
			}
		}
	}
	if symbol := state.getSymbol(node); symbol != nil {
		return macroOfSymbol(state, symbol)
	}
	return ""
}

func transformIdentifier(state *TransformState, node *ts.Node) *ts.Node {
	if macro := valueMacro(state, node); macro != "" {
		return transformMacroValue(state, node, macro)
	}
	return node
}

func transformPropertyAccessExpression(state *TransformState, node *ts.Node) *ts.Node {
	access := node.AsPropertyAccessExpression()
	if access.Expression.Kind == ts.KindIdentifier && access.QuestionDotToken == nil {
		if macro := valueMacro(state, access.Expression); macro == macroFile || macro == macroPackage {
			return transformMacroField(state, node, macro)
		}
	}
	return state.visitor.VisitEachChild(node)
}

func transformExpression(state *TransformState, expression *ts.Node) *ts.Node {
	return catchDiagnostic(state.diagnostics, expression, func() *ts.Node {
		if transformer := expressionTransformers[expression.Kind]; transformer != nil {
			return transformer(state, expression)
		}
		return state.visitor.VisitEachChild(expression)
	})
}
