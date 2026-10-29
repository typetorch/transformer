// Port of src/util/functions/tryResolveTS.ts.

package transformer

// tryResolveTS resolves a module the way the TypeScript program does (types first). Like TypeScript's
// resolveModuleName, resolution starts in the directory of `containingFile`.
func tryResolveTS(state *TransformState, moduleName string, containingFile string) string {
	return state.moduleResolver().ResolveModuleName(moduleName, containingFile)
}
