// Port of src/util/functions/isTupleType.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func isTupleType(state *TransformState, t *ts.Type) bool {
	return ts.IsTupleType(t)
}

func isArrayType(state *TransformState, t *ts.Type) bool {
	return state.typeChecker.IsArrayType(t)
}
