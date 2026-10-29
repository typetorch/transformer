// Port of src/transformations/transformStatementList.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/gobloxshim"
)

func transformStatementList(state *TransformState, statements []*ts.Node) []*ts.Node {
	var result []*ts.Node
	// Statements that compiled to nothing (a dropped macro import, a stripped `$print`): their comments, which may
	// be the file's header or a directive, move to the next statement that is emitted.
	var dropped []*ts.Node

	for _, statement := range statements {
		newStatements, prereqs := state.capture(func() *ts.Node { return transformStatement(state, statement) })
		produced := append(prereqs, getNodeList(newStatements)...)

		if len(produced) == 0 {
			dropped = append(dropped, statement)
			continue
		}

		for _, node := range dropped {
			ts.CopyComments(state.f.ec, node, produced[0])
		}
		dropped = nil
		result = append(result, produced...)
	}

	return result
}
