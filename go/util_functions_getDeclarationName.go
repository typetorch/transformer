// Port of src/util/functions/getDeclarationName.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// getDeclarationName calculates a name including all named ancestors, such as Enum.Material.Air.
func getDeclarationName(node *ts.Node) string {
	name := node.Name()
	if name == nil || name.Kind != ts.KindIdentifier {
		return "$p:error"
	}
	result := name.Text()
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if isNamedDeclaration(parent) {
			result = ts.GetTextOfNode(parent.Name()) + "." + result
		}
	}
	return result
}

// isNamedDeclaration is TypeScript's `ts.isNamedDeclaration` / `f.is.namedDeclaration`: any node with a `name`.
func isNamedDeclaration(node *ts.Node) bool {
	return node != nil && node.Name() != nil
}
