// Port of src/classes/diagnostics.ts.
//
// `Diagnostics.error` throws a DiagnosticError in TypeScript; here it panics with a *diagnosticError, and
// captureDiagnostic recovers it. Warnings collect on the TransformState's diagnostics (TypeScript keeps a static
// list), which transformFile flushes.

package transformer

import (
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// diagnosticSource is what the TypeScript transformer puts in `code`.
const diagnosticSource = "@typetorch/transformer"

func createDiagnosticAtLocation(node *ts.Node, messageText string, category ts.DiagnosticCategory) *ts.Diagnostic {
	file := ts.GetSourceFileOfNode(node)
	start := node.Pos()
	if file != nil {
		start = ts.GetTokenPosOfNode(node, file)
	}
	return ts.NewDiagnostic(file, start, node.End()-start, category, diagnosticSource, messageText)
}

type diagnosticError struct {
	diagnostic *ts.Diagnostic
}

func (e *diagnosticError) Error() string { return e.diagnostic.MessageText() }

// diagnostics is the TypeScript `Diagnostics` class: a list of collected (non-fatal) diagnostics.
type diagnostics struct {
	list []*ts.Diagnostic
}

func (d *diagnostics) addDiagnostic(diag *ts.Diagnostic) {
	d.list = append(d.list, diag)
}

func createDiagnostic(node *ts.Node, category ts.DiagnosticCategory, messages ...string) *ts.Diagnostic {
	return createDiagnosticAtLocation(node, strings.Join(messages, "\n"), category)
}

// diagnosticsError is `Diagnostics.error`: it does not return.
func diagnosticsError(node *ts.Node, messages ...string) {
	panic(&diagnosticError{createDiagnostic(node, ts.CategoryError, messages...)})
}

func (d *diagnostics) warning(node *ts.Node, messages ...string) {
	d.addDiagnostic(createDiagnostic(node, ts.CategoryWarning, messages...))
}

func (d *diagnostics) flush() []*ts.Diagnostic {
	list := d.list
	d.list = nil
	return list
}
