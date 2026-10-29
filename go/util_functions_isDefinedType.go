// Port of src/util/functions/isDefinedType.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// isDefinedType: https://github.com/roblox-ts/roblox-ts/blob/dc74f34fdab3caf20d65db080cf2dbf5c4f38fdc/src/TSTransformer/util/types.ts#L70
func isDefinedType(state *TransformState, t *ts.Type) bool {
	checker := state.typeChecker
	return t.Flags() == ts.TypeFlagsObject &&
		len(checker.GetPropertiesOfType(t)) == 0 &&
		len(checker.GetSignaturesOfType(t, ts.SignatureKindCall)) == 0 &&
		len(checker.GetSignaturesOfType(t, ts.SignatureKindConstruct)) == 0 &&
		checker.GetNumberIndexType(t) == nil &&
		checker.GetStringIndexType(t) == nil
}
