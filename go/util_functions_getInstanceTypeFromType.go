// Port of src/util/functions/getInstanceTypeFromType.ts.

package transformer

import (
	"regexp"
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

var nominalRegex = regexp.MustCompile(`_nominal_(.*)`)

func getInstanceTypeFromType(state *TransformState, file *ts.SourceFile, t *ts.Type) *ts.Type {
	checker := state.typeChecker
	assert(checker.GetPropertyOfType(t, "_nominal_Instance") != nil, "non instance type was passed into getInstanceTypeFromType")

	diagnosticsLocation := getDeclarationOfType(t)
	if diagnosticsLocation == nil {
		diagnosticsLocation = file.AsNode()
	}
	nominalProperties := getNominalProperties(state, t)

	specificType, specificTypeCount := t, 0
	for _, property := range nominalProperties {
		match := nominalRegex.FindStringSubmatch(property.Name)
		assert(match != nil)
		noNominalName := match[1]

		instanceSymbol := checker.ResolveName(noNominalName, nil, ts.SymbolFlagsType, false)
		if instanceSymbol == nil || len(instanceSymbol.Declarations) == 0 {
			continue
		}

		instanceType := checker.GetTypeAtLocation(instanceSymbol.Declarations[0])
		subNominalProperties := getNominalProperties(state, instanceType)

		if len(subNominalProperties) > specificTypeCount {
			specificType = instanceType
			specificTypeCount = len(subNominalProperties)
		}
	}

	// intersection between two nominal types?
	for _, property := range nominalProperties {
		if checker.GetPropertyOfType(specificType, property.Name) == nil {
			diagnosticsError(diagnosticsLocation, "Intersection between nominal types is forbidden.")
		}
	}

	return specificType
}

func getNominalProperties(state *TransformState, t *ts.Type) []*ts.Symbol {
	var result []*ts.Symbol
	for _, property := range state.typeChecker.GetPropertiesOfType(t) {
		if strings.HasPrefix(property.Name, "_nominal_") {
			result = append(result, property)
		}
	}
	return result
}
