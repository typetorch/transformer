// Port of src/util/functions/checkNetworkLeaf.ts.

package transformer

import (
	"regexp"
	"strconv"
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// supportedNetworkTypes is what a network leaf's parameters may be (shown with every network guard error): what the
// guard builder can check and Roblox can send over a RemoteEvent.
const supportedNetworkTypes = "Supported parameter types: string, number, boolean, undefined/optional, literals and TS enums, Enum items, " +
	"Roblox datatypes (Vector3, CFrame, Color3, UDim2, ...), Instances by class (BasePart, Model, Player, ...), " +
	"arrays, tuples, Maps, Sets, plain objects/interfaces, unions, buffer, and unknown/any."

// unsendable lists types that never arrive on the other side of a RemoteEvent (Roblox sends nil), with what to send
// instead.
var unsendable = map[string]string{
	"thread":              "a thread can't be sent over the network (it arrives as nil)",
	"RBXScriptSignal":     "an RBXScriptSignal can't be sent over the network (it arrives as nil): send the data instead",
	"RBXScriptConnection": "an RBXScriptConnection can't be sent over the network (it arrives as nil)",
}

// localOnlyInstances lists Instance classes that exist only where they were made, so a reference to one arrives as
// nil. (A slice: TypeScript iterates the object in order.)
var localOnlyInstances = []struct{ className, reason string }{
	{"AnimationTrack", "an AnimationTrack doesn't replicate (it arrives as nil): send the Animation or its AnimationId and load the track on the other side"},
}

const maxDepth = 4

var rbxtsTypesRegex = regexp.MustCompile(`[\\/]node_modules[\\/]@rbxts[\\/](types|compiler-types)[\\/]`)

func typeName(t *ts.Type) string {
	if alias := t.Alias(); alias != nil && alias.Symbol() != nil {
		return alias.Symbol().Name
	}
	if symbol := t.Symbol(); symbol != nil {
		return symbol.Name
	}
	return ""
}

// unsendableReason is why a value of `t` can't cross the network, if it can't.
func unsendableReason(state *TransformState, t *ts.Type) string {
	checker := state.typeChecker
	if isInstanceType(state, t) {
		for _, local := range localOnlyInstances {
			if checker.GetPropertyOfType(t, "_nominal_"+local.className) != nil {
				return local.reason
			}
		}
		return ""
	}
	if reason, ok := unsendable[typeName(t)]; ok {
		return reason
	}
	if len(checker.GetSignaturesOfType(t, ts.SignatureKindCall)) > 0 {
		return "a function can't be sent over the network (it arrives as nil)"
	}
	return ""
}

// isPlainObject: plain object types whose properties are worth looking into (not Instances, datatypes, Maps or
// arrays).
func isPlainObject(state *TransformState, t *ts.Type) bool {
	if t.Flags()&ts.TypeFlagsObject == 0 || isInstanceType(state, t) {
		return false
	}
	if state.typeChecker.IsArrayType(t) || ts.IsTupleType(t) {
		return false
	}
	// Roblox datatypes and engine types come from the @rbxts type packages: their fields aren't sent one by one.
	if symbol := t.Symbol(); symbol != nil && len(symbol.Declarations) > 0 {
		if rbxtsTypesRegex.MatchString(ts.GetSourceFileOfNode(symbol.Declarations[0]).FileName()) {
			return false
		}
	}
	return true
}

func appendUnique(list []string, text string) []string {
	for _, existing := range list {
		if existing == text {
			return list
		}
	}
	return append(list, text)
}

// checkNetworkLeaf warns about a network leaf (a `Modding.Generic<A, "guard">` inside a `@metadata macro network`
// call's tree) whose guard compiles but can't do its job: a generic or conditional signature (the guard checks each
// type parameter as its constraint and each conditional type as either branch, so the arguments' relation isn't
// enforced), and parameters that never arrive (functions, threads, signals, connections, AnimationTracks: Roblox
// sends nil, so a client -> server guard rejects every message and the server -> client side gets nil).
func checkNetworkLeaf(state *TransformState, at *ts.Node, leaf string, params *ts.Type, declaration *ts.Node) {
	checker := state.typeChecker
	var generic, unsendableProblems []string
	seen := map[*ts.Type]bool{}

	asConstraint := func(parameter *ts.Type) string {
		constraint := checker.GetBaseConstraintOfType(parameter)
		text := "unknown"
		if constraint != nil {
			text = checker.TypeToString(constraint)
		}
		return checker.TypeToString(parameter) + " as " + text
	}

	// `infer A` erases a generic signature (each type parameter becomes its constraint), so look at the declared leaf.
	if declaration != nil {
		for _, signature := range checker.GetSignaturesOfType(checker.GetTypeAtLocation(declaration), ts.SignatureKindCall) {
			for _, parameter := range signature.TypeParameters() {
				generic = appendUnique(generic, asConstraint(parameter))
			}
		}
	}

	var walk func(t *ts.Type, where string, depth int)
	walk = func(t *ts.Type, where string, depth int) {
		if depth > maxDepth || seen[t] {
			return
		}
		seen[t] = true
		if t.Flags()&ts.TypeFlagsTypeParameter != 0 {
			generic = appendUnique(generic, asConstraint(t))
			return
		}
		if t.Flags()&ts.TypeFlagsConditional != 0 {
			generic = appendUnique(generic, checker.TypeToString(t)+" as either branch")
			return
		}
		if t.Flags()&ts.TypeFlagsUnionOrIntersection != 0 {
			for _, member := range t.Types() {
				walk(member, where, depth)
			}
			return
		}
		if reason := unsendableReason(state, t); reason != "" {
			unsendableProblems = append(unsendableProblems, where+": "+reason+".")
			return
		}
		if checker.IsArrayType(t) {
			if typeArgs := checker.GetTypeArguments(t); len(typeArgs) > 0 && typeArgs[0] != nil {
				walk(typeArgs[0], where+"[]", depth+1)
			}
			return
		}
		if isPlainObject(state, t) {
			for _, property := range checker.GetPropertiesOfType(t) {
				if propertyType := checker.GetTypeOfPropertyOfType(t, property.Name); propertyType != nil {
					walk(propertyType, where+"."+property.Name, depth+1)
				}
			}
		}
	}

	if isTupleType(state, params) {
		elementInfos := params.TargetTupleType().ElementInfos()
		for index, element := range checker.GetTypeArguments(params) {
			name := strconv.Itoa(index + 1)
			if index < len(elementInfos) {
				if labeled := elementInfos[index].LabeledDeclaration(); labeled != nil {
					if label := labeled.Name(); label != nil && label.Kind == ts.KindIdentifier {
						name = `"` + label.Text() + `"`
					}
				}
			}
			walk(element, "parameter "+name, 0)
		}
	} else {
		walk(params, "its parameters", 0)
	}

	if len(generic) > 0 {
		state.diagnostics.warning(at,
			`Network leaf "`+leaf+`" has a generic or conditional signature: its guard checks `+strings.Join(generic, ", ")+" "+
				"(and a conditional type as either branch), so it can't enforce how the arguments go together. "+
				"Declare a union of tuples instead, "+
				"e.g. `(...args: [box: Model, group: string] | [box: undefined]) => void`, or use separate leaves.")
	}
	for _, problem := range unsendableProblems {
		state.diagnostics.warning(at, `Network leaf "`+leaf+`", `+problem)
	}
}
