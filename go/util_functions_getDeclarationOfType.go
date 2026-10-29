// Port of src/util/functions/getDeclarationOfType.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func getDeclarationOfType(t *ts.Type) *ts.Node {
	if symbol := t.Symbol(); symbol != nil && len(symbol.Declarations) > 0 {
		return symbol.Declarations[0]
	}
	return nil
}
