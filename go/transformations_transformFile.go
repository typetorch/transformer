// Port of src/transformations/transformFile.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func transformFile(state *TransformState, file *ts.SourceFile) *ts.SourceFile {
	f := state.f
	statements := transformStatementList(state, file.Statements.Nodes)

	if imports := state.fileImports[file.FileName()]; imports != nil {
		var firstStatement *ts.Node
		if len(statements) > 0 {
			firstStatement = statements[0]
		}

		declarations := make([]*ts.Node, len(imports))
		for i, info := range imports {
			entries := make([]importSpecifier, len(info.entries))
			for j, entry := range info.entries {
				entries[j] = importSpecifier{name: entry.name, identifier: entry.identifier}
			}
			declarations[i] = f.importDeclaration(info.path, entries)
		}
		statements = append(declarations, statements...)

		// steal comments from original first statement so that comment directives work properly
		if firstStatement != nil && len(statements) > 0 {
			original := state.parseTreeNode(firstStatement)

			ts.MoveSyntheticComments(f.ec, statements[0], firstStatement)

			if original != nil {
				ts.CopyComments(f.ec, original, statements[0])
				ts.RemoveAllComments(f.ec, original)
			}
		}
	}

	for _, diag := range state.diagnostics.flush() {
		state.addDiagnostic(diag)
	}

	return f.nf.UpdateSourceFile(file, f.list(statements), file.EndOfFileToken).AsSourceFile()
}
