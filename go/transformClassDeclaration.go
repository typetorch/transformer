// Port of src/transformations/statements/transformClassDeclaration.ts.

package transformer

import (
	ts "github.com/microsoft/typescript-go/gobloxshim"
)

// decoratorMarker is the property that marks a decorator made with `Modding.createDecorator` /
// `createMetaDecorator` (runtime kit).
const decoratorMarker = "_typetorch_decorator"

// lazyMarker is the property that marks a lazy module reference (`Lazy<T>` in @typetorch/framework:
// `readonly _typetorch_lazy: T`, type-only). A constructor parameter of such a type records `lazy:<id of T>` in
// `typetorch:parameters`, so the DI hands it a handle that resolves T on first use (and its start order ignores that
// edge) instead of injecting it. Without this, the id would be `Lazy`'s own: type arguments aren't part of a type
// reference's id.
const lazyMarker = "_typetorch_lazy"

// getLazyParameterId is `lazy:<id of T>` for a parameter whose type carries the lazy marker, else "".
func getLazyParameterId(state *TransformState, parameter *ts.Node) string {
	t := state.typeChecker.GetNonNullableType(state.typeChecker.GetTypeAtLocation(parameter))
	target := state.typeChecker.GetTypeOfPropertyOfType(t, lazyMarker)
	if target == nil {
		return ""
	}
	return "lazy:" + mustUid(getTypeUid(state, target, parameter))
}

// Reflect metadata keys. A class (or member) asks for them with JSDoc on itself, on a decorator's declaration, or on
// an interface it implements: `@metadata typetorch:parameters injectable`.
const (
	// metadataKeyIdentifier is always written for classes with a TypeTorch decorator; registers the class in
	// `Reflect.idToObj`.
	metadataKeyIdentifier      = "identifier"
	metadataKeyParameters      = "typetorch:parameters"
	metadataKeyParameterNames  = "typetorch:parameter_names"
	metadataKeyParameterGuards = "typetorch:parameter_guards"
	metadataKeyImplements      = "typetorch:implements"
	metadataKeyType            = "typetorch:type"
	metadataKeyGuard           = "typetorch:guard"
	metadataKeyReturnType      = "typetorch:return_type"
	metadataKeyReturnGuard     = "typetorch:return_guard"
)

// metadataField is one `[key, value]` pair of reflection metadata.
type metadataField struct {
	key   string
	value convertableExpression
}

func transformClassDeclaration(state *TransformState, node *ts.Node) []*ts.Node {
	f := state.f
	symbol := state.getSymbol(node)
	name := node.Name()
	if symbol == nil || name == nil {
		return []*ts.Node{state.transform(node)}
	}

	metadata := newNodeMetadata(state, node)
	hasDecorators := hasTypeTorchDecorators(state, node)
	if !hasDecorators && !metadata.isRequested("reflect") {
		return []*ts.Node{state.transform(node)}
	}

	importIdentifier := state.getReflect(state.getSourceFile(node))
	var reflectStatements, decoratorStatements []*ts.Node

	convertReflectionToStatements := func(metadata []metadataField, property *string) []*ts.Node {
		statements := make([]*ts.Node, len(metadata))
		for i, field := range metadata {
			args := []convertableExpression{name, field.key, field.value}
			if property != nil {
				args = append(args, *property)
			}

			statements[i] = f.statement(f.call(f.field(importIdentifier, "defineMetadata"), args...))
		}

		if len(statements) > 0 {
			addSectionComment(state, statements[0], node, property, "metadata")
		}

		return statements
	}

	reflectStatements = append(reflectStatements, convertReflectionToStatements(generateClassMetadata(state, hasDecorators, metadata, node), nil)...)
	decoratorStatements = append(decoratorStatements, getDecoratorStatements(state, node, node, metadata)...)

	for _, member := range node.Members() {
		if member.Name() == nil {
			continue
		}

		propertyName, ok := ts.GetPropertyNameForPropertyNameNode(member.Name())
		if !ok {
			continue
		}

		reflectStatements = append(reflectStatements, convertReflectionToStatements(getNodeReflection(state, member, nil), &propertyName)...)
		decoratorStatements = append(decoratorStatements, getDecoratorStatements(state, node, member, nil)...)
	}

	return append([]*ts.Node{updateClass(state, node, reflectStatements)}, decoratorStatements...)
}

func generateFieldMetadata(state *TransformState, metadata *nodeMetadata, field *ts.Node) []metadataField {
	var fields []metadataField
	t := state.typeChecker.GetTypeAtLocation(field)
	typeNode := field.Type()

	if metadata.isRequested(metadataKeyType) {
		if typeNode == nil {
			at := field.Name()
			if at == nil {
				at = field
			}
			fields = append(fields, metadataField{metadataKeyType, mustUid(getTypeUid(state, t, at))})
		} else {
			fields = append(fields, metadataField{metadataKeyType, getNodeUid(state, typeNode)})
		}
	}

	if metadata.isRequested(metadataKeyGuard) {
		at := typeNode
		if at == nil {
			at = field
		}
		fields = append(fields, metadataField{metadataKeyGuard, buildGuardFromType(state, at, t)})
	}

	return fields
}

func generateMethodMetadata(state *TransformState, metadata *nodeMetadata, method *ts.Node) []metadataField {
	var fields []metadataField
	baseSignature := state.typeChecker.GetSignatureFromDeclaration(method)
	if baseSignature == nil {
		return nil
	}
	returnType := state.typeChecker.GetReturnTypeOfSignature(baseSignature)
	typeNode := method.Type()

	if metadata.isRequested(metadataKeyReturnType) {
		if typeNode == nil {
			at := method.Name()
			if at == nil {
				at = method
			}
			fields = append(fields, metadataField{metadataKeyReturnType, mustUid(getTypeUid(state, returnType, at))})
		} else {
			fields = append(fields, metadataField{metadataKeyReturnType, getNodeUid(state, typeNode)})
		}
	}

	if metadata.isRequested(metadataKeyReturnGuard) {
		at := typeNode
		if at == nil {
			at = method
		}
		fields = append(fields, metadataField{metadataKeyReturnGuard, buildGuardFromType(state, at, returnType)})
	}

	var parameters, parameterNames []convertableExpression
	var parameterGuards []convertableExpression

	for _, parameter := range method.Parameters() {
		if metadata.isRequested(metadataKeyParameters) {
			if lazyId := getLazyParameterId(state, parameter); lazyId != "" {
				parameters = append(parameters, lazyId)
			} else if parameterType := parameter.Type(); parameterType != nil {
				parameters = append(parameters, getNodeUid(state, parameterType))
			} else {
				t := state.typeChecker.GetTypeAtLocation(parameter)
				parameters = append(parameters, mustUid(getTypeUid(state, t, parameter)))
			}
		}

		if metadata.isRequested(metadataKeyParameterNames) {
			if parameterName := parameter.Name(); parameterName.Kind == ts.KindIdentifier {
				parameterNames = append(parameterNames, parameterName.Text())
			} else {
				parameterNames = append(parameterNames, "_binding_")
			}
		}

		if metadata.isRequested(metadataKeyParameterGuards) {
			t := state.typeChecker.GetTypeAtLocation(parameter)
			parameterGuards = append(parameterGuards, buildGuardFromType(state, parameter, t))
		}
	}

	if len(parameters) > 0 {
		fields = append(fields, metadataField{metadataKeyParameters, parameters})
	}

	if len(parameterNames) > 0 {
		fields = append(fields, metadataField{metadataKeyParameterNames, parameterNames})
	}

	if len(parameterGuards) > 0 {
		fields = append(fields, metadataField{metadataKeyParameterGuards, parameterGuards})
	}

	return fields
}

func generateClassMetadata(state *TransformState, hasDecorators bool, metadata *nodeMetadata, node *ts.Node) []metadataField {
	f := state.f
	var fields []metadataField

	// Decorated classes always get an identifier, so dependency ids resolve through `Reflect.idToObj`.
	if hasDecorators || metadata.isRequested(metadataKeyIdentifier) {
		fields = append(fields, metadataField{metadataKeyIdentifier, getNodeUid(state, node)})
	}

	for _, member := range node.Members() {
		if member.Kind == ts.KindConstructor {
			fields = append(fields, generateMethodMetadata(state, metadata, member)...)
			break
		}
	}

	if heritageClauses := node.AsClassDeclaration().HeritageClauses; heritageClauses != nil {
		var implementClauses []*ts.Node
		for _, clause := range heritageClauses.Nodes {
			heritage := clause.AsHeritageClause()
			if heritage.Token != ts.KindImplementsKeyword {
				continue
			}

			for _, t := range heritage.Types.Nodes {
				implementClauses = append(implementClauses, f.string(getNodeUid(state, t)))
			}
		}

		if len(implementClauses) > 0 && metadata.isRequested(metadataKeyImplements) {
			fields = append(fields, metadataField{metadataKeyImplements, f.array(implementClauses, false)})
		}
	}

	return fields
}

func getNodeReflection(state *TransformState, node *ts.Node, metadata *nodeMetadata) []metadataField {
	if metadata == nil {
		metadata = newNodeMetadata(state, node)
	}
	switch node.Kind {
	case ts.KindMethodDeclaration:
		return generateMethodMetadata(state, metadata, node)
	case ts.KindPropertyDeclaration:
		return generateFieldMetadata(state, metadata, node)
	}
	return nil
}

func getDecoratorStatements(state *TransformState, declaration *ts.Node, node *ts.Node, metadata *nodeMetadata) []*ts.Node {
	f := state.f
	if node.Name() == nil {
		return nil
	}
	if metadata == nil {
		metadata = newNodeMetadata(state, node)
	}

	isClass := node.Kind == ts.KindClassDeclaration
	symbol := state.getSymbol(node.Name())
	propertyName, ok := getNameFromPropertyName(node.Name())
	assert(ok)
	assert(symbol != nil)
	var decoratorStatements []*ts.Node

	if ts.CanHaveDecorators(node) {
		decorators := ts.GetDecorators(node)
		// Decorators apply last->first, so we iterate the decorators in reverse.
		for i := len(decorators) - 1; i >= 0; i-- {
			decorator := decorators[i]
			expr := decorator.Expression()
			if !isTypeTorchDecorator(state, decorator) {
				continue
			}

			identifier := expr
			if expr.Kind == ts.KindCallExpression {
				identifier = expr.Expression()
			}
			decoratorSymbol := state.getSymbol(identifier)
			assert(decoratorSymbol != nil)
			assert(decoratorSymbol.ValueDeclaration != nil)

			importIdentifier := state.getReflect(state.getSourceFile(node))
			args := []convertableExpression{}
			if expr.Kind == ts.KindCallExpression {
				for _, argument := range expr.Arguments() {
					args = append(args, state.transformNode(argument))
				}
			}
			decorateArgs := []convertableExpression{declaration.Name(), mustUid(getSymbolUid(state, decoratorSymbol, identifier)), identifier, args}
			if !isClass {
				decorateArgs = append(decorateArgs, propertyName, ts.HasStaticModifier(node))
			}

			decoratorStatements = append(decoratorStatements, f.statement(f.call(f.field(importIdentifier, "decorate"), decorateArgs...)))
		}
	}

	constraintTypes := metadata.getType("constraint")
	nodeType := state.typeChecker.GetTypeOfSymbolAtLocation(symbol, node)
	for _, constraintType := range constraintTypes {
		if !state.typeChecker.IsTypeAssignableTo(nodeType, constraintType) {
			state.diagnostics.addDiagnostic(getAssignabilityDiagnostics(state, node.Name(), nodeType, constraintType, metadata.getTrace(constraintType)))
		}
	}

	if len(decoratorStatements) > 0 {
		var property *string
		if !isClass {
			property = &propertyName
		}
		addSectionComment(state, decoratorStatements[0], declaration, property, "decorators")
	}
	return decoratorStatements
}

// getNameFromPropertyName is TypeScript's `ts.getNameFromPropertyName`: the text of an identifier, literal or
// literal computed name.
func getNameFromPropertyName(name *ts.Node) (string, bool) {
	if name.Kind == ts.KindComputedPropertyName {
		expression := name.Expression()
		switch expression.Kind {
		case ts.KindStringLiteral, ts.KindNoSubstitutionTemplateLiteral, ts.KindNumericLiteral:
			return expression.Text(), true
		}
		return "", false
	}
	if name.Kind == ts.KindPrivateIdentifier {
		return "", false
	}
	return name.Text(), true
}

func addSectionComment(state *TransformState, node *ts.Node, declaration *ts.Node, property *string, label string) {
	if node == nil {
		return
	}

	elementName := declaration.Name().Text()
	if property != nil {
		elementName += "." + *property
	}
	ts.AddSyntheticLeadingComment(state.f.ec, node, ts.KindSingleLineCommentTrivia, " (TypeTorch) "+elementName+" "+label)
}

func formatType(state *TransformState, t *ts.Type) string {
	return ts.FormatType(state.typeChecker, t)
}

func getAssignabilityDiagnostics(state *TransformState, node *ts.Node, sourceType *ts.Type, constraintType *ts.Type, trace *ts.Node) *ts.Diagnostic {
	diagnostic := createDiagnostic(node, ts.CategoryError,
		"Type '"+formatType(state, sourceType)+"' does not satisfy constraint '"+formatType(state, constraintType)+"'")

	if trace != nil {
		diagnostic.AddRelatedInfo(createDiagnostic(trace, ts.CategoryMessage, "The constraint is defined here."))
	}

	return diagnostic
}

func updateClass(state *TransformState, node *ts.Node, staticStatements []*ts.Node) *ts.Node {
	f := state.f
	nf := f.nf
	modifiers := getAllModifiers(node)
	var members []*ts.Node
	for _, original := range node.Members() {
		member := state.transformNode(original)
		// Strip TypeTorch decorators from members
		if memberModifiers := getAllModifiers(member); memberModifiers != nil {
			filteredModifiers := transformModifiers(state, memberModifiers)
			switch member.Kind {
			case ts.KindPropertyDeclaration:
				property := member.AsPropertyDeclaration()
				member = nf.UpdatePropertyDeclaration(property, emptyModifiersToNil(filteredModifiers), property.Name(), property.PostfixToken, property.Type, property.Initializer)
			case ts.KindMethodDeclaration:
				method := member.AsMethodDeclaration()
				typeParameters := method.TypeParameters
				if typeParameters != nil && len(typeParameters.Nodes) == 0 {
					typeParameters = nil
				}
				member = nf.UpdateMethodDeclaration(method, emptyModifiersToNil(filteredModifiers), method.AsteriskToken, method.Name(), method.PostfixToken, typeParameters, method.Parameters, method.Type, method.FullSignature, method.Body)
			}
		}
		members = append(members, member)
	}

	if len(staticStatements) > 0 {
		members = append(members, f.staticBlockDeclaration(staticStatements))
	}

	class := node.AsClassDeclaration()
	var name *ts.Node
	if class.Name() != nil {
		name = state.transformNode(class.Name())
	}
	var newModifiers *ts.ModifierList
	if modifiers != nil {
		newModifiers = transformModifiers(state, modifiers)
	}
	return nf.UpdateClassDeclaration(class, newModifiers, name, class.TypeParameters, class.HeritageClauses, f.list(members))
}

func emptyModifiersToNil(modifiers *ts.ModifierList) *ts.ModifierList {
	if modifiers == nil || len(modifiers.Nodes) == 0 {
		return nil
	}
	return modifiers
}

func getAllModifiers(node *ts.Node) *ts.ModifierList {
	if ts.CanHaveDecorators(node) || ts.CanHaveModifiers(node) {
		return node.Modifiers()
	}
	return nil
}

func transformModifiers(state *TransformState, modifiers *ts.ModifierList) *ts.ModifierList {
	var result []*ts.Node
	for _, modifier := range modifiers.Nodes {
		if modifier.Kind == ts.KindDecorator && isTypeTorchDecorator(state, modifier) {
			continue
		}
		result = append(result, state.transform(modifier))
	}
	return state.f.nf.NewModifierList(result)
}

func isTypeTorchDecorator(state *TransformState, decorator *ts.Node) bool {
	decoratorType := state.typeChecker.GetTypeAtLocation(decorator.Expression())
	return state.typeChecker.GetPropertyOfType(decoratorType, decoratorMarker) != nil
}

func hasTypeTorchDecorators(state *TransformState, declaration *ts.Node) bool {
	hasAny := func(node *ts.Node) bool {
		if !ts.CanHaveDecorators(node) {
			return false
		}
		for _, decorator := range ts.GetDecorators(node) {
			if isTypeTorchDecorator(state, decorator) {
				return true
			}
		}
		return false
	}

	if hasAny(declaration) {
		return true
	}
	for _, member := range declaration.Members() {
		if hasAny(member) {
			return true
		}
	}
	return false
}
