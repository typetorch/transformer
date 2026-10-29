// Port of src/util/functions/getNodeList.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// getNodeList flattens what a statement transform returned (a node, or a SyntaxList of them).
func getNodeList(node *ts.Node) []*ts.Node {
	if node == nil {
		return nil
	}
	if node.Kind == ts.KindSyntaxList {
		return node.AsSyntaxList().Children
	}
	return []*ts.Node{node}
}
