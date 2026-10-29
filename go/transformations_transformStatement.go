// Port of src/transformations/transformStatement.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

var statementTransformers = map[ts.Kind]func(state *TransformState, node *ts.Node) []*ts.Node{}

func init() {
	statementTransformers[ts.KindClassDeclaration] = transformClassDeclaration
	statementTransformers[ts.KindExpressionStatement] = transformExpressionStatement
	statementTransformers[ts.KindImportDeclaration] = transformImportDeclaration
}

func visitChildren(state *TransformState, node *ts.Node) []*ts.Node {
	return []*ts.Node{state.visitor.VisitEachChild(node)}
}

// transformExpressionStatement: a `$print(...)` / `$warn(...)` / `$dbg(...)` statement may compile to nothing (see
// debugMacros.go).
func transformExpressionStatement(state *TransformState, node *ts.Node) []*ts.Node {
	if expression := node.Expression(); mayBeMacroCall(expression) {
		if macro := macroOfCall(state, expression); macro != "" {
			if result, ok := transformMacroStatement(state, node, macro); ok {
				return result
			}
		}
	}
	return visitChildren(state, node)
}

// transformImportDeclaration: imports of the macro declaration file are dropped, nothing in it exists at runtime.
func transformImportDeclaration(state *TransformState, node *ts.Node) []*ts.Node {
	if result, ok := transformMacroImport(state, node); ok {
		return result
	}
	return visitChildren(state, node)
}

// statementsNode is what the visitor takes for an array of statements: the statement itself when there is one, a
// SyntaxList otherwise (merged into the enclosing list, or lifted to a block).
func statementsNode(state *TransformState, statements []*ts.Node) *ts.Node {
	if len(statements) == 1 {
		return statements[0]
	}
	return state.f.nf.NewSyntaxList(statements)
}

func transformStatement(state *TransformState, statement *ts.Node) *ts.Node {
	return catchDiagnostic(state.diagnostics, statement, func() *ts.Node {
		node, prereqs := state.capture(func() *ts.Node {
			if transformer := statementTransformers[statement.Kind]; transformer != nil {
				return statementsNode(state, transformer(state, statement))
			}
			return state.visitor.VisitEachChild(statement)
		})

		return statementsNode(state, append(prereqs, getNodeList(node)...))
	})
}
