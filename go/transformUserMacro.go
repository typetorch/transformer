// Port of src/transformations/transformUserMacro.ts.

package transformer

import (
	"crypto/rand"
	"fmt"
	"strconv"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

// networkContext is where a macro value sits in a `@metadata macro network` call (createNetwork,
// createFlameworkCompat): the dotted path of the leaf and its declaration, so guard problems name the leaf.
type networkContext struct {
	path        []string
	declaration *ts.Node
}

// Property names that mark TypeTorch's macro types (see `Modding` in the runtime kit). They only exist in the type
// system: a parameter typed `Modding.Generic<T, "guard">` has a `_typetorch_macro_generic: [T, "guard"]` property.
const (
	macroMarkerGeneric     = "_typetorch_macro_generic"
	macroMarkerMany        = "_typetorch_macro_many"
	macroMarkerCaller      = "_typetorch_macro_caller"
	macroMarkerTupleLabels = "_typetorch_macro_tuple_labels"
)

type userMacroKind int

const (
	userMacroGeneric userMacroKind = iota
	userMacroCaller
	userMacroMany
	userMacroLiteral
)

// userMacro is TypeScript's `UserMacro` union.
type userMacro struct {
	kind userMacroKind

	// generic: the type the metadata is about; generic and caller: what is requested ("id", "guard", "line", ...).
	target   *ts.Type
	metadata string

	// many: an array (isArray) or an object, its members in order (memberNames for objects).
	isArray     bool
	members     []*userMacro
	memberNames []string
	// declarations: object members, where each was declared (network leaves report there).
	declarations map[string]*ts.Node

	// literal: string, float64, bool or nil (undefined).
	value any
}

func literalMacro(value any) *userMacro {
	return &userMacro{kind: userMacroLiteral, value: value}
}

// transformUserMacro fills in the omitted macro parameters of a call to a function declared with `@metadata macro`.
// Nil: not transformed.
func transformUserMacro(state *TransformState, node *ts.Node, signature *ts.Signature) *ts.Node {
	f := state.f
	var args []*ts.Node
	if arguments := node.ArgumentList(); arguments != nil {
		args = append(args, arguments.Nodes...)
	}
	parameters := map[int]*userMacro{}

	highestParameterIndex := -1
	for i := 0; i < getParameterCount(state, signature); i++ {
		// This parameter is passed explicitly, so we don't need to evaluate it.
		if i < len(args) && !isUndefinedArgument(args[i]) {
			continue
		}

		targetParameter := state.typeChecker.GetNonNullableType(state.typeChecker.GetTypeAtPosition(signature, i))
		if macro := getUserMacroOfUnion(state, node, targetParameter); macro != nil {
			parameters[i] = macro
			highestParameterIndex = max(highestParameterIndex, i)
		}
	}

	// `@metadata macro network`: the macro's trees are network leaves (guards get leaf-level diagnostics).
	declaration := signature.Declaration()
	network := declaration != nil && newNodeMetadata(state, declaration).isRequested("network")

	for i := 0; i <= highestParameterIndex; i++ {
		for len(args) <= i {
			args = append(args, nil)
		}
		if macro := parameters[i]; macro != nil {
			var context *networkContext
			if network {
				context = &networkContext{path: []string{}}
			}
			args[i] = buildUserMacro(state, node, macro, context)
		} else if args[i] != nil {
			args[i] = state.transform(args[i])
		} else {
			args[i] = f.nil()
		}
	}

	name := state.transformNode(node.Expression())

	switch node.Kind {
	case ts.KindNewExpression:
		return f.nf.UpdateNewExpression(node.AsNewExpression(), name, node.TypeArgumentList(), f.list(args))
	case ts.KindCallExpression:
		call := node.AsCallExpression()
		return f.nf.UpdateCallExpression(call, name, call.QuestionDotToken, node.TypeArgumentList(), f.list(args), node.Flags)
	}
	diagnosticsError(node, "Macro could not be transformed.")
	return nil
}

func isUndefinedArgument(argument *ts.Node) bool {
	return argument == nil || (argument.Kind == ts.KindIdentifier && argument.Text() == "undefined")
}

func getLabels(state *TransformState, t *ts.Type) *userMacro {
	if !isTupleType(state, t) {
		return literalMacro(nil)
	}

	// TypeScript's `labeledElementDeclarations` is undefined when no element has a label.
	elementInfos := t.TargetTupleType().ElementInfos()
	labeled := false
	for i := range elementInfos {
		labeled = labeled || elementInfos[i].LabeledDeclaration() != nil
	}
	if !labeled {
		return literalMacro(nil)
	}

	names := make([]*userMacro, 0, len(elementInfos))
	for i := range elementInfos {
		// TypeScript 5.0+ allows nameless tuple elements, so we'll default to an empty string in that case.
		name := ""
		if declaration := elementInfos[i].LabeledDeclaration(); declaration != nil {
			name = declaration.Name().Text()
		}
		names = append(names, literalMacro(name))
	}

	return &userMacro{kind: userMacroMany, isArray: true, members: names}
}

func buildUserMacro(state *TransformState, node *ts.Node, macro *userMacro, network *networkContext) *ts.Node {
	f := state.f
	switch macro.kind {
	case userMacroGeneric:
		if metadata := getGenericMetadata(state, node, macro, network); metadata != nil {
			return f.asNever(metadata)
		}
	case userMacroCaller:
		if metadata := getCallerMetadata(state, node, macro); metadata != nil {
			return f.asNever(metadata)
		}
	case userMacroMany:
		if macro.isArray {
			elements := make([]*ts.Node, len(macro.members))
			for index, member := range macro.members {
				var memberNetwork *networkContext
				if network != nil {
					memberNetwork = &networkContext{path: appendPath(network.path, strconv.Itoa(index+1))}
				}
				elements[index] = buildUserMacro(state, node, member, memberNetwork)
			}
			return f.asNever(f.array(elements, true))
		}

		var elements []*ts.Node
		for i, name := range macro.memberNames {
			var memberNetwork *networkContext
			if network != nil {
				declaration := macro.declarations[name]
				if declaration == nil {
					declaration = network.declaration
				}
				memberNetwork = &networkContext{path: appendPath(network.path, name), declaration: declaration}
			}
			expression := buildUserMacro(state, node, macro.members[i], memberNetwork)
			if inner := expression.Expression(); inner.Kind == ts.KindIdentifier && inner.Text() == "undefined" {
				continue
			}

			elements = append(elements, f.propertyAssignmentDeclaration(f.string(name), expression))
		}

		return f.asNever(f.object(elements, false))
	case userMacroLiteral:
		switch value := macro.value.(type) {
		case string:
			return f.asNever(f.string(value))
		case float64:
			return f.asNever(f.number(value))
		case bool:
			return f.asNever(f.bool(value))
		}
		return f.asNever(f.nil())
	}

	return f.asNever(f.nil())
}

func appendPath(path []string, element string) []string {
	return append(append([]string{}, path...), element)
}

func getGenericMetadata(state *TransformState, node *ts.Node, macro *userMacro, network *networkContext) *ts.Node {
	f := state.f
	switch macro.metadata {
	case "id":
		return f.string(mustUid(getTypeUid(state, macro.target, node)))
	case "guard":
		if network != nil && len(network.path) > 0 {
			return buildNetworkLeafGuard(state, node, macro.target, network)
		}

		result := buildGuardFromTypeWithDedup(state, node, macro.target)
		state.prereqList(result.statements)

		return result.guard
	case "text":
		return f.string(state.typeChecker.TypeToString(macro.target))
	}
	return nil
}

// buildNetworkLeafGuard is a network leaf's guard: warnings for signatures the guard can't enforce or values that
// never arrive, and a guard error that names the leaf and lists the supported parameter types.
func buildNetworkLeafGuard(state *TransformState, node *ts.Node, target *ts.Type, network *networkContext) *ts.Node {
	leaf := joinPath(network.path)
	at := node
	if network.declaration != nil && ts.GetSourceFileOfNode(network.declaration) != nil {
		at = network.declaration
	}
	checkNetworkLeaf(state, at, leaf, target, network.declaration)
	result := withDiagnosticContext(
		node,
		func() string {
			return `Network leaf "` + leaf + `": no guard can be generated for its parameters (details below). ` + supportedNetworkTypes
		},
		func() guardResult { return buildGuardFromTypeWithDedup(state, node, target) },
	)
	state.prereqList(result.statements)
	return result.guard
}

func joinPath(path []string) string {
	result := ""
	for i, element := range path {
		if i > 0 {
			result += "."
		}
		result += element
	}
	return result
}

func getCallerMetadata(state *TransformState, node *ts.Node, macro *userMacro) *ts.Node {
	f := state.f
	file := ts.GetSourceFileOfNode(node)
	start := ts.GetTokenPosOfNode(node, file)
	line, character := ts.GetLineAndCharacterOfPosition(file, start)

	switch macro.metadata {
	case "line":
		return f.number(float64(line + 1))
	case "character":
		return f.number(float64(character + 1))
	case "width":
		return f.number(float64(node.End() - start))
	case "uuid":
		if state.config.Hooks.UUID != nil {
			return f.string(state.config.Hooks.UUID())
		}
		return f.string(randomUUID())
	case "text":
		return f.string(ts.GetTextOfNode(node))
	}
	return nil
}

// randomUUID is crypto.randomUUID: a version 4 UUID.
func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func getMetadataFromType(t *ts.Type) string {
	if t.Flags()&ts.TypeFlagsStringLiteral != 0 {
		return t.AsLiteralType().Value().(string)
	}
	return ""
}

func getUserMacroOfMany(state *TransformState, node *ts.Node, target *ts.Type) *userMacro {
	checker := state.typeChecker
	if basic := getBasicUserMacro(state, node, target); basic != nil {
		return basic
	}

	if manyMetadata := checker.GetTypeOfPropertyOfType(target, macroMarkerMany); manyMetadata != nil {
		return getUserMacroOfMany(state, node, manyMetadata)
	}

	if isTupleType(state, target) {
		var members []*userMacro
		for _, member := range checker.GetTypeArguments(target) {
			macro := getUserMacroOfMany(state, node, member)
			if macro == nil {
				return nil
			}
			members = append(members, macro)
		}

		return &userMacro{kind: userMacroMany, isArray: true, members: members}
	} else if checker.IsArrayType(target) {
		targetType := checker.GetTypeArguments(target)[0]
		constituents := []*ts.Type{targetType}
		if targetType.IsUnion() {
			constituents = targetType.Types()
		}
		members := []*userMacro{}

		for _, member := range constituents {
			// `never` may be encountered when a union has no constituents, so we should just return an empty array.
			if member.Flags()&ts.TypeFlagsNever != 0 {
				break
			}

			macro := getUserMacroOfMany(state, node, member)
			if macro == nil {
				return nil
			}
			members = append(members, macro)
		}

		return &userMacro{kind: userMacroMany, isArray: true, members: members}
	} else if isObjectTypeOrIntersection(target) {
		result := &userMacro{kind: userMacroMany, declarations: map[string]*ts.Node{}}

		for _, member := range checker.GetPropertiesOfType(target) {
			memberType := checker.GetTypeOfPropertyOfType(target, member.Name)
			if memberType == nil {
				return nil
			}

			macro := getUserMacroOfMany(state, node, memberType)
			if macro == nil {
				return nil
			}

			result.memberNames = append(result.memberNames, member.Name)
			result.members = append(result.members, macro)
			// A mapped type's property keeps the declaration it was mapped from (the leaf in the game's interface).
			declaration := member.ValueDeclaration
			if declaration == nil && len(member.Declarations) > 0 {
				declaration = member.Declarations[0]
			}
			if declaration != nil {
				result.declarations[member.Name] = declaration
			}
		}

		return result
	} else if target.Flags()&(ts.TypeFlagsStringLiteral|ts.TypeFlagsNumberLiteral) != 0 {
		value := target.AsLiteralType().Value()
		if number, ok := ts.NumberValue(value); ok {
			return literalMacro(number)
		}
		return literalMacro(value)
	} else if target.Flags()&ts.TypeFlagsUndefined != 0 {
		return literalMacro(nil)
	} else if target.Flags()&ts.TypeFlagsBooleanLiteral != 0 {
		return literalMacro(target.AsLiteralType().Value() == true)
	}

	diagnosticsError(node, "Unknown type '"+checker.TypeToString(target)+"' encountered")
	return nil
}

func getBasicUserMacro(state *TransformState, node *ts.Node, target *ts.Type) *userMacro {
	checker := state.typeChecker
	if genericMetadata := checker.GetTypeOfPropertyOfType(target, macroMarkerGeneric); genericMetadata != nil {
		targetType := checker.GetTypeOfPropertyOfType(genericMetadata, "0")
		metadataType := checker.GetTypeOfPropertyOfType(genericMetadata, "1")
		if targetType == nil || metadataType == nil {
			return nil
		}

		metadata := getMetadataFromType(metadataType)
		if metadata == "" {
			diagnosticsError(node, "@typetorch/transformer encountered invalid metadata: '"+checker.TypeToString(metadataType)+"'")
		}

		return &userMacro{kind: userMacroGeneric, target: targetType, metadata: metadata}
	}

	if callerMetadata := checker.GetTypeOfPropertyOfType(target, macroMarkerCaller); callerMetadata != nil {
		metadata := getMetadataFromType(callerMetadata)
		if metadata == "" {
			return nil
		}

		return &userMacro{kind: userMacroCaller, metadata: metadata}
	}

	nonNullableTarget := checker.GetNonNullableType(target)
	if labelMetadata := checker.GetTypeOfPropertyOfType(nonNullableTarget, macroMarkerTupleLabels); labelMetadata != nil {
		return getLabels(state, labelMetadata)
	}

	return nil
}

func getUserMacroOfType(state *TransformState, node *ts.Node, target *ts.Type) *userMacro {
	if manyMetadata := state.typeChecker.GetTypeOfPropertyOfType(target, macroMarkerMany); manyMetadata != nil {
		return getUserMacroOfMany(state, node, manyMetadata)
	}
	return getBasicUserMacro(state, node, target)
}

// getUserMacroOfUnion allows user macros to specify signatures that can accept non-metadata. Multiple modding types
// in a single parameter aren't supported, and the first one is chosen.
//
// For example, `string | Modding.Generic<T, "id">`, will generate the ID for `T`, but also allow users to pass in
// one manually.
func getUserMacroOfUnion(state *TransformState, node *ts.Node, target *ts.Type) *userMacro {
	if !target.IsUnion() {
		return getUserMacroOfType(state, node, target)
	}

	for _, constituent := range target.Types() {
		if macro := getUserMacroOfType(state, node, constituent); macro != nil {
			return macro
		}
	}
	return nil
}

// isObjectTypeOrIntersection is transformUserMacro.ts' isObjectType: intersections count when all members do.
func isObjectTypeOrIntersection(t *ts.Type) bool {
	if t.IsIntersection() {
		for _, member := range t.Types() {
			if !isObjectTypeOrIntersection(member) {
				return false
			}
		}
		return true
	}
	return t.Flags()&ts.TypeFlagsObject != 0
}

func getParameterCount(state *TransformState, signature *ts.Signature) int {
	parameters := signature.Parameters()
	length := len(parameters)
	if signature.HasRestParameter() {
		restType := state.typeChecker.GetTypeOfSymbol(parameters[length-1])
		if isTupleType(state, restType) {
			tuple := restType.TargetTupleType()
			hasRestElement := false
			for _, flags := range tuple.ElementFlags() {
				hasRestElement = hasRestElement || flags&ts.ElementFlagsVariable != 0
			}
			if hasRestElement {
				return length + tuple.FixedLength()
			}
			return length + tuple.FixedLength() - 1
		}
	}
	return length
}
