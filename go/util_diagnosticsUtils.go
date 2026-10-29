// Port of src/util/diagnosticsUtils.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func relocateDiagnostic(diagnostic *ts.Diagnostic, node *ts.Node) {
	file := ts.GetSourceFileOfNode(node)
	start := ts.GetTokenPosOfNode(node, file)
	diagnostic.SetFile(file)
	diagnostic.SetLocation(ts.NewTextRange(start, node.End()))
	panic(&diagnosticError{diagnostic})
}

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
