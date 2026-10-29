// Port of src/classes/diagnostics.ts and src/util/diagnosticsUtils.ts.
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

func relocateDiagnostic(diagnostic *ts.Diagnostic, node *ts.Node) {
	file := ts.GetSourceFileOfNode(node)
	start := ts.GetTokenPosOfNode(node, file)
	diagnostic.SetFile(file)
	diagnostic.SetLocation(ts.NewTextRange(start, node.End()))
	panic(&diagnosticError{diagnostic})
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

/// diagnosticsUtils.ts

// captureDiagnostic runs cb; a diagnostic it raises is returned instead of propagating.
func captureDiagnostic[T any](cb func() T) (value T, diagnostic *ts.Diagnostic) {
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(*diagnosticError); ok {
				diagnostic = err.diagnostic
				return
			}
			panic(r)
		}
	}()
	return cb(), nil
}

// catchDiagnostic records a diagnostic raised by cb and returns `fallback` in its place.
func catchDiagnostic[T any](d *diagnostics, fallback T, cb func() T) T {
	value, diagnostic := captureDiagnostic(cb)
	if diagnostic != nil {
		d.addDiagnostic(diagnostic)
		return fallback
	}
	return value
}

// withDiagnosticContext rethrows a diagnostic raised by callback as `message` at `node`, with the original (and its
// related information) as related information.
func withDiagnosticContext[T any](node *ts.Node, message func() string, callback func() T) T {
	value, diagnostic := captureDiagnostic(callback)
	if diagnostic != nil {
		newDiagnostic := createDiagnostic(node, ts.CategoryError, message())
		newDiagnostic.AddRelatedInfo(diagnostic)
		for _, related := range diagnostic.RelatedInformation() {
			newDiagnostic.AddRelatedInfo(related)
		}
		panic(&diagnosticError{newDiagnostic})
	}
	return value
}
