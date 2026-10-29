// Port of src/util/functions/buildGuardFromType.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/gobloxshim"
)

// buildGuardsFromType converts a type into a list of property guards (`isInterfaceType` omits unknown fields).
func buildGuardsFromType(state *TransformState, node *ts.Node, t *ts.Type, file *ts.SourceFile, isInterfaceType bool) []*ts.Node {
	if file == nil {
		file = state.getSourceFile(node)
	}
	generator := createGuardGenerator(state, file, node)
	return generator.buildGuardsFromType(t, isInterfaceType)
}

// rbxTypesNew compile directly to `t.typeof` for any userdata that `t` does not have an alias for, or users might not
// have yet.
var rbxTypesNew = []string{"buffer"}

var rbxTypes = append([]string{
	"UDim",
	"UDim2",
	"BrickColor",
	"Color3",
	"Vector2",
	"Vector3",
	"NumberSequence",
	"NumberSequenceKeypoint",
	"ColorSequence",
	"ColorSequenceKeypoint",
	"NumberRange",
	"Rect",
	"DockWidgetPluginGuiInfo",
	"CFrame",
	"Axes",
	"Faces",
	"Font",
	"Instance",
	"Ray",
	"Random",
	"Region3",
	"Region3int16",
	"Enum",
	"TweenInfo",
	"PhysicalProperties",
	"Vector3int16",
	"Vector2int16",
	"PathWaypoint",
	"EnumItem",
	"RBXScriptSignal",
	"RBXScriptConnection",
	"FloatCurveKey",
	"OverlapParams",
	"thread",
}, rbxTypesNew...)

const objectIgnoredFieldTypes = ts.TypeFlagsUnknown | ts.TypeFlagsNever | ts.TypeFlagsUniqueESSymbol
const dedupHeuristicLimit = 5
const dedupHeuristicFlags = ts.TypeFlagsObject | ts.TypeFlagsUnionOrIntersection

func getTypesRequiringDedupHeuristic(state *TransformState, root *ts.Type, dedupLimit int) map[*ts.Type]bool {
	checker := state.typeChecker
	seenCount := map[*ts.Type]int{}
	// Insertion order, as JavaScript's Map iterates.
	var seenOrder []*ts.Type

	var recurse func(t *ts.Type, modifier int)
	recurse = func(t *ts.Type, modifier int) {
		if t.Flags()&dedupHeuristicFlags != 0 {
			if _, ok := seenCount[t]; !ok {
				seenOrder = append(seenOrder, t)
			}
			seenCount[t] += modifier
		}

		if t.Flags()&ts.TypeFlagsUnionOrIntersection != 0 {
			for _, member := range t.Types() {
				recurse(member, modifier)
			}
		} else if t.Flags()&ts.TypeFlagsObject != 0 && !isInstanceType(state, t) {
			for _, property := range checker.GetPropertiesOfType(t) {
				propertyType := checker.GetTypeOfPropertyOfType(t, property.Name)
				if propertyType == nil {
					continue
				}
				recurse(propertyType, modifier)
			}

			for _, indexInfo := range checker.GetIndexInfosOfType(t) {
				recurse(indexInfo.KeyType(), modifier)
				recurse(indexInfo.ValueType(), modifier)
			}
		}
	}

	recurse(root, 1)

	requiresDedup := map[*ts.Type]bool{}

	for i := 0; i < len(seenOrder); i++ {
		t := seenOrder[i]
		count := seenCount[t]
		if count >= dedupLimit {
			requiresDedup[t] = true

			// We subtract all the children, as deduplicating the parent effectively removes `count - 1` of any
			// children from the emit.
			recurse(t, -(count - 1))
		}
	}

	return requiresDedup
}

// buildGuardFromType converts a type into a type guard.
func buildGuardFromType(state *TransformState, node *ts.Node, t *ts.Type) *ts.Node {
	generator := createGuardGenerator(state, state.getSourceFile(node), node)
	return generator.buildGuard(t)
}

type guardResult struct {
	guard      *ts.Node
	statements []*ts.Node
}

// buildGuardFromTypeWithDedup converts a type into a type guard, deduplicating large guards.
func buildGuardFromTypeWithDedup(state *TransformState, node *ts.Node, t *ts.Type) guardResult {
	generator := createGuardGenerator(state, state.getSourceFile(node), node)
	if optimizations := state.config.Optimizations; optimizations != nil && optimizations.GuardGenerationDedupLimit != nil {
		generator.calculateDedup(t, max(*optimizations.GuardGenerationDedupLimit, 1))
	}

	guard := generator.buildGuard(t)
	return guardResult{guard: guard, statements: generator.dedupStatements}
}

type trackedType struct {
	node *ts.Node
	t    *ts.Type
}

// guardGenerator is the stateful guard generator of createGuardGenerator.
type guardGenerator struct {
	state           *TransformState
	file            *ts.SourceFile
	diagnosticNode  *ts.Node
	tracking        []trackedType
	dedupStatements []*ts.Node
	dedupIds        map[*ts.Type]*ts.Node
	requiresDedup   map[*ts.Type]bool
}

func createGuardGenerator(state *TransformState, file *ts.SourceFile, diagnosticNode *ts.Node) *guardGenerator {
	return &guardGenerator{
		state:          state,
		file:           file,
		diagnosticNode: diagnosticNode,
		dedupIds:       map[*ts.Type]*ts.Node{},
		requiresDedup:  map[*ts.Type]bool{},
	}
}

func (g *guardGenerator) fail(err string) {
	basicDiagnostic := createDiagnostic(g.diagnosticNode, ts.CategoryError, err)
	var previousType *ts.Type
	for _, location := range g.tracking {
		if location.t == previousType {
			continue
		}

		previousType = location.t
		at := location.node
		if isNamedDeclaration(at) {
			at = at.Name()
		}
		basicDiagnostic.AddRelatedInfo(createDiagnostic(at, ts.CategoryError,
			"Type was defined here: "+g.state.typeChecker.TypeToString(location.t)))
	}
	panic(&diagnosticError{basicDiagnostic})
}

func (g *guardGenerator) pop(declaration *ts.Node) {
	last := g.tracking[len(g.tracking)-1]
	g.tracking = g.tracking[:len(g.tracking)-1]
	assert(last.node == declaration, "Popped value was not expected")
}

func (g *guardGenerator) calculateDedup(t *ts.Type, dedupLimit int) {
	g.requiresDedup = getTypesRequiringDedupHeuristic(g.state, t, dedupLimit)
}

func (g *guardGenerator) buildGuard(t *ts.Type) *ts.Node {
	if g.requiresDedup[t] {
		if existingId := g.dedupIds[t]; existingId != nil {
			return existingId
		}
	}

	declaration := getDeclarationOfType(t)
	if declaration != nil {
		g.tracking = append(g.tracking, trackedType{declaration, t})
	}

	guard := g.buildGuardInner(t)

	if declaration != nil {
		g.pop(declaration)
	}

	if g.requiresDedup[t] {
		name := "dedup"
		if alias := t.Alias(); alias != nil && alias.Symbol() != nil {
			name = alias.Symbol().Name
		}
		dedupId := g.state.f.identifier(name, true)
		g.dedupIds[t] = dedupId

		g.dedupStatements = append(g.dedupStatements, g.state.f.variableStatement(dedupId, guard))

		return dedupId
	}

	return guard
}

func (g *guardGenerator) buildGuardInner(t *ts.Type) *ts.Node {
	state, f := g.state, g.state.f
	typeChecker := state.typeChecker
	tId := state.getGuardLibrary(g.file)

	if t.IsUnion() {
		return g.buildUnionGuard(t)
	}

	if isInstanceType(state, t) {
		instanceType := getInstanceTypeFromType(state, g.file, t)
		var additionalGuards []*ts.Node

		for _, property := range typeChecker.GetPropertiesOfType(t) {
			propertyType := typeChecker.GetTypeOfPropertyOfType(t, property.Name)
			if propertyType != nil && typeChecker.GetPropertyOfType(instanceType, property.Name) == nil {
				// assume intersections are children
				additionalGuards = append(additionalGuards, f.propertyAssignmentDeclaration(property.Name, g.buildGuard(propertyType)))
			}
		}

		baseGuard := f.call(f.field(tId, "instanceIsA"), instanceType.Symbol().Name)
		if len(additionalGuards) == 0 {
			return baseGuard
		}
		return g.listLikeGuard("intersection", []*ts.Node{
			baseGuard,
			f.call(f.field(tId, "children"), f.object(additionalGuards, true)),
		})
	}

	if t.IsIntersection() {
		return g.buildIntersectionGuard(t)
	}

	if isConditionalType(t) {
		return g.listLikeGuard("union", []*ts.Node{
			g.buildGuard(typeChecker.GetTrueTypeOfConditionalType(t)),
			g.buildGuard(typeChecker.GetFalseTypeOfConditionalType(t)),
		})
	}

	if t.Flags()&ts.TypeFlagsTypeVariable != 0 {
		constraint := typeChecker.GetBaseConstraintOfType(t)
		if constraint == nil {
			g.fail("could not find constraint of type parameter")
		}

		return g.buildGuard(constraint)
	}

	if literals := getLiteral(state, t, false); literals != nil {
		return g.listLikeGuard("literal", literals)
	}

	if ts.IsTupleType(t) {
		typeArgs := typeChecker.GetTypeArguments(t)
		guards := make([]convertableExpression, len(typeArgs))
		for i, typeArg := range typeArgs {
			guards[i] = g.buildGuard(typeArg)
		}
		return f.call(f.field(tId, "strictArray"), guards...)
	}

	if typeChecker.IsArrayType(t) {
		if typeArgs := typeChecker.GetTypeArguments(t); len(typeArgs) > 0 {
			return f.call(f.field(tId, "array"), g.buildGuard(typeArgs[0]))
		}
		return f.call(f.field(tId, "array"), f.field(tId, "any"))
	}

	if len(typeChecker.GetSignaturesOfType(t, ts.SignatureKindCall)) > 0 {
		return f.field(tId, "callback")
	}

	if t == typeChecker.GetVoidType() || t == typeChecker.GetUndefinedType() {
		return f.field(tId, "none")
	}

	if t == typeChecker.GetAnyType() {
		return f.field(tId, "any")
	}

	if t == typeChecker.GetStringType() {
		return f.field(tId, "string")
	}

	if t == typeChecker.GetNumberType() {
		return f.field(tId, "number")
	}

	if t.Flags()&ts.TypeFlagsUnknown != 0 {
		return g.listLikeGuard("union", []*ts.Node{f.field(tId, "any"), f.field(tId, "none")})
	}

	if t.Flags()&ts.TypeFlagsTemplateLiteral != 0 {
		g.fail("The guard generator encountered a template literal which is unsupported: " + typeChecker.TypeToString(t))
	}

	symbol := t.Symbol()
	if symbol == nil {
		g.fail("An unknown type was encountered with no symbol: " + typeChecker.TypeToString(t))
	}

	resolveType := func(name string) *ts.Symbol {
		return typeChecker.ResolveName(name, nil, ts.SymbolFlagsType, false)
	}

	if symbol == resolveType("Map") || symbol == resolveType("ReadonlyMap") || symbol == resolveType("WeakMap") {
		typeArgs := typeChecker.GetTypeArguments(t)
		keyGuard, valueGuard := f.field(tId, "any"), f.field(tId, "any")
		if len(typeArgs) > 0 {
			keyGuard = g.buildGuard(typeArgs[0])
		}
		if len(typeArgs) > 1 {
			valueGuard = g.buildGuard(typeArgs[1])
		}
		return f.call(f.field(tId, "map"), keyGuard, valueGuard)
	}

	if symbol == resolveType("Set") || symbol == resolveType("ReadonlySet") {
		if typeArgs := typeChecker.GetTypeArguments(t); len(typeArgs) > 0 {
			return f.call(f.field(tId, "set"), g.buildGuard(typeArgs[0]))
		}
		return f.call(f.field(tId, "set"), f.field(tId, "any"))
	}

	if symbol == resolveType("Promise") {
		return f.field("Promise", "is")
	}

	for _, guard := range rbxTypes {
		guardSymbol := resolveType(guard)
		if guardSymbol == nil && symbol.Name == guard {
			g.fail("Could not find symbol for " + guard)
		}

		if symbol == guardSymbol {
			for _, newType := range rbxTypesNew {
				if newType == guard {
					return f.call(f.field(tId, "typeof"), guard)
				}
			}
			return f.field(tId, guard)
		}
	}

	if t.IsClass() {
		g.fail(`Class "` + t.Symbol().Name + `" was encountered. Guards cannot be generated for classes.`)
	}

	isObject := isObjectType(t)
	indexInfos := typeChecker.GetIndexInfosOfType(t)
	apparentProperties := typeChecker.GetApparentProperties(t)
	if isObject && len(apparentProperties) == 0 && len(indexInfos) == 0 {
		return f.field(tId, "any")
	}

	if isObject || t.ObjectFlags()&ts.ObjectFlagsClassOrInterface != 0 {
		var guards []*ts.Node

		if len(apparentProperties) > 0 {
			guards = append(guards, f.call(f.field(tId, "interface"), f.object(g.buildGuardsFromType(t, true), true)))
		}

		if len(indexInfos) > 0 {
			if len(indexInfos) > 1 {
				g.fail("Guards cannot be generated for types with multiple index signatures.")
			}

			indexInfo := indexInfos[0]
			guards = append(guards, f.call(f.field(tId, "map"), g.buildGuard(indexInfo.KeyType()), g.buildGuard(indexInfo.ValueType())))
		}

		if len(guards) > 1 {
			return g.listLikeGuard("intersection", guards)
		}
		return guards[0]
	}

	g.fail("An unknown type was encountered: " + typeChecker.TypeToString(t))
	panic("unreachable")
}

func (g *guardGenerator) buildUnionGuard(t *ts.Type) *ts.Node {
	state, f := g.state, g.state.f
	tId := state.getGuardLibrary(g.file)

	if t == state.typeChecker.GetBooleanType() {
		return f.field(tId, "boolean")
	}

	enums, literals, simplifiedTypes := simplifyUnion(state, t)
	isOptional, types := extractTypes(state, simplifiedTypes)
	guards := make([]*ts.Node, 0, len(types)+len(enums)+1)
	for _, member := range types {
		guards = append(guards, g.buildGuard(member))
	}
	for _, enumId := range enums {
		guards = append(guards, f.call(f.field(tId, "enum"), f.field("Enum", enumId)))
	}

	if len(literals) > 0 {
		guards = append(guards, g.listLikeGuard("literal", literals))
	}

	var union *ts.Node
	if len(guards) > 1 {
		union = g.listLikeGuard("union", guards)
	} else if len(guards) == 1 {
		union = guards[0]
	} else {
		return f.field(tId, "none")
	}

	if isOptional {
		return f.call(f.field(tId, "optional"), union)
	}
	return union
}

func (g *guardGenerator) buildIntersectionGuard(t *ts.Type) *ts.Node {
	if len(g.state.typeChecker.GetIndexInfosOfType(t)) > 1 {
		g.fail("Guards cannot be generated for intersections with multiple index signatures.")
	}

	// We find any disjoint types (strings, numbers, etc) as intersections with them are invalid.
	// Most intersections with disjoint types are used to introduce nominal fields.
	for _, member := range t.Types() {
		if member.Flags()&ts.TypeFlagsDisjointDomains != 0 {
			return g.buildGuard(member)
		}
	}

	guards := make([]*ts.Node, len(t.Types()))
	for i, member := range t.Types() {
		guards[i] = g.buildGuard(member)
	}
	return g.listLikeGuard("intersection", guards)
}

func (g *guardGenerator) buildGuardsFromType(t *ts.Type, isInterfaceType bool) []*ts.Node {
	state, f := g.state, g.state.f
	typeChecker := state.typeChecker

	declaration := getDeclarationOfType(t)
	if declaration != nil {
		g.tracking = append(g.tracking, trackedType{declaration, t})
	}

	var guards []*ts.Node
	for _, property := range typeChecker.GetPropertiesOfType(t) {
		propertyDeclaration := property.ValueDeclaration
		propertyType := typeChecker.GetTypeOfPropertyOfType(t, property.Name)
		if propertyType == nil {
			g.fail("Could not find type for field")
		}

		if isInterfaceType && propertyType.Flags()&objectIgnoredFieldTypes != 0 {
			continue
		}

		if propertyDeclaration != nil {
			g.tracking = append(g.tracking, trackedType{propertyDeclaration, propertyType})
		}

		attribute := g.buildGuard(propertyType)
		guards = append(guards, f.propertyAssignmentDeclaration(property.Name, attribute))

		if propertyDeclaration != nil {
			g.pop(propertyDeclaration)
		}
	}

	if declaration != nil {
		g.pop(declaration)
	}

	return guards
}

// listLikeGuard creates a guard using either the vararg function or list (array) version.
//
// This is a relatively naive method of checking as it does not keep track of the real register count, but fixing
// this fully would likely involve moving away from `t`.
func (g *guardGenerator) listLikeGuard(guard string, list []*ts.Node) *ts.Node {
	f := g.state.f
	tId := g.state.getGuardLibrary(g.file)

	if len(list) <= 2 {
		return f.call(f.field(tId, guard), toConvertables(list)...)
	}

	return f.call(f.field(tId, guard+"List"), f.array(list, true))
}

func simplifyUnion(state *TransformState, t *ts.Type) (enums []string, literals []*ts.Node, types []*ts.Type) {
	f := state.f
	checker := state.typeChecker
	enumType := checker.ResolveName("Enum", nil, ts.SymbolFlagsType, false)
	if alias := t.Alias(); alias != nil {
		if aliasSymbol := alias.Symbol(); aliasSymbol != nil && aliasSymbol.Parent != nil && checker.GetMergedSymbol(aliasSymbol.Parent) == enumType {
			return []string{aliasSymbol.Name}, nil, nil
		}
	}

	currentTypes := t.Types()
	// Insertion order, as JavaScript's Map iterates.
	var enumOrder []*ts.Symbol
	possibleEnums := map[*ts.Symbol][]*ts.Type{}
	booleanLiterals := 0
	for _, member := range currentTypes {
		if member.Flags()&ts.TypeFlagsBooleanLiteral != 0 {
			booleanLiterals++
		}
	}
	isBoolean := booleanLiterals == 2

	if isBoolean {
		types = append(types, checker.GetBooleanType())
	}

	for _, member := range currentTypes {
		// We do not need to generate symbol types as they don't exist in Lua.
		if member.Flags()&ts.TypeFlagsESSymbolLike != 0 {
			continue
		}

		// This is a full `boolean`, so we can skip the individual literals.
		if isBoolean && member.Flags()&ts.TypeFlagsBooleanLiteral != 0 {
			continue
		}

		if literal := getLiteral(state, member, true); literal != nil {
			literals = append(literals, literal...)
			continue
		}

		symbol := member.Symbol()
		if symbol == nil || symbol.Parent == nil {
			types = append(types, member)
			continue
		}

		enumKind := symbol.Parent
		if enumKind.Parent == nil || checker.GetMergedSymbol(enumKind.Parent) != enumType {
			types = append(types, member)
			continue
		}

		if enumKind.Exports != nil && symbol == enumKind.Exports[symbol.Name] {
			if _, ok := possibleEnums[enumKind]; !ok {
				enumOrder = append(enumOrder, enumKind)
			}
			if !containsType(possibleEnums[enumKind], member) {
				possibleEnums[enumKind] = append(possibleEnums[enumKind], member)
			}
		}
	}

	for _, symbol := range enumOrder {
		set := possibleEnums[symbol]
		// Add 1 to account for GetEnumItems()
		if len(set)+1 == len(symbol.Exports) {
			enums = append(enums, symbol.Name)
		} else {
			for _, member := range set {
				literals = append(literals, f.field(f.field("Enum", symbol.Name), member.Symbol().Name))
			}
		}
	}

	return enums, literals, types
}

func containsType(types []*ts.Type, t *ts.Type) bool {
	for _, member := range types {
		if member == t {
			return true
		}
	}
	return false
}

func extractTypes(state *TransformState, types []*ts.Type) (isOptional bool, result []*ts.Type) {
	undefinedType := state.typeChecker.GetUndefinedType()
	voidType := state.typeChecker.GetVoidType()

	for _, t := range types {
		if t == undefinedType || t == voidType {
			isOptional = true
		} else {
			result = append(result, t)
		}
	}
	return isOptional, result
}

// isRegularBooleanLiteral is `type === checker.getTrueType()` (or getFalseType()): without an argument TypeScript's
// getters return the regular literal types.
func isRegularBooleanLiteral(t *ts.Type, value bool) bool {
	if t.Flags()&ts.TypeFlagsBooleanLiteral == 0 {
		return false
	}
	literal := t.AsLiteralType()
	return literal.Value() == value && literal.RegularType() == t
}

func getLiteral(state *TransformState, t *ts.Type, withoutEnums bool) []*ts.Node {
	f := state.f
	checker := state.typeChecker
	if t.Flags()&(ts.TypeFlagsStringLiteral|ts.TypeFlagsNumberLiteral) != 0 {
		return []*ts.Node{literalValue(f, t.AsLiteralType().Value())}
	}

	if isRegularBooleanLiteral(t, true) {
		return []*ts.Node{f.bool(true)}
	}

	if isRegularBooleanLiteral(t, false) {
		return []*ts.Node{f.bool(false)}
	}

	if t.Flags()&ts.TypeFlagsEnum != 0 {
		declarations := t.Symbol().Declarations
		if len(declarations) != 1 || declarations[0].Kind != ts.KindEnumDeclaration {
			return nil
		}

		var memberValues []*ts.Node
		for _, member := range declarations[0].AsEnumDeclaration().Members.Nodes {
			constant := checker.GetConstantValue(member)
			if constant == nil {
				return nil
			}
			memberValues = append(memberValues, literalValue(f, constant))
		}

		return memberValues
	}

	if !withoutEnums {
		symbol := t.Symbol()
		if symbol == nil {
			return nil
		}

		enumType := checker.ResolveName("Enum", nil, ts.SymbolFlagsType, false)
		if symbol.Parent != nil && symbol.Parent.Parent != nil && checker.GetMergedSymbol(symbol.Parent.Parent) == enumType {
			return []*ts.Node{f.field(f.field("Enum", symbol.Parent.Name), symbol.Name)}
		}
	}

	return nil
}

// literalValue is a string or number literal of a checker value.
func literalValue(f *factory, value any) *ts.Node {
	if text, ok := value.(string); ok {
		return f.string(text)
	}
	number, _ := ts.NumberValue(value)
	return f.number(number)
}

func isObjectType(t *ts.Type) bool {
	return t.Flags()&ts.TypeFlagsObject != 0
}

func isInstanceType(state *TransformState, t *ts.Type) bool {
	return state.typeChecker.GetPropertyOfType(t, "_nominal_Instance") != nil
}

func isConditionalType(t *ts.Type) bool {
	return t.Flags()&ts.TypeFlagsConditional != 0
}
