import assert from "assert";
import ts from "typescript";
import { Diagnostics } from "../../classes/diagnostics";
import { NodeMetadata } from "../../classes/nodeMetadata";
import { TransformState } from "../../classes/transformState";
import { f } from "../../util/factory";
import { buildGuardFromType } from "../../util/functions/buildGuardFromType";
import { getNodeUid, getSymbolUid, getTypeUid } from "../../util/uid";

/**
 * The property that marks a decorator made with `Modding.createDecorator` / `createMetaDecorator` (runtime kit).
 */
export const DECORATOR_MARKER = "_typetorch_decorator";

/**
 * Reflect metadata keys. A class (or member) asks for them with JSDoc on itself, on a decorator's declaration, or on
 * an interface it implements: `@metadata typetorch:parameters injectable`.
 */
export const MetadataKeys = {
	/** Always written for classes with a TypeTorch decorator; registers the class in `Reflect.idToObj`. */
	identifier: "identifier",
	parameters: "typetorch:parameters",
	parameterNames: "typetorch:parameter_names",
	parameterGuards: "typetorch:parameter_guards",
	implements: "typetorch:implements",
	type: "typetorch:type",
	guard: "typetorch:guard",
	returnType: "typetorch:return_type",
	returnGuard: "typetorch:return_guard",
} as const;

export function transformClassDeclaration(state: TransformState, node: ts.ClassDeclaration) {
	const symbol = state.getSymbol(node);
	if (!symbol || !node.name) return state.transform(node);

	const metadata = new NodeMetadata(state, node);
	const hasDecorators = hasTypeTorchDecorators(state, node);
	if (!hasDecorators && !metadata.isRequested("reflect")) return state.transform(node);

	const importIdentifier = state.getReflect(state.getSourceFile(node));
	const reflectStatements = new Array<ts.Statement>();
	const decoratorStatements = new Array<ts.Statement>();

	reflectStatements.push(
		...convertReflectionToStatements(generateClassMetadata(state, hasDecorators, metadata, node)),
	);
	decoratorStatements.push(...getDecoratorStatements(state, node, node, metadata));

	for (const member of node.members) {
		if (!member.name) {
			continue;
		}

		const propertyName = ts.getPropertyNameForPropertyNameNode(member.name);
		if (!propertyName) {
			continue;
		}

		reflectStatements.push(...convertReflectionToStatements(getNodeReflection(state, member) ?? [], propertyName));
		decoratorStatements.push(...getDecoratorStatements(state, node, member));
	}

	return [updateClass(state, node, reflectStatements), ...decoratorStatements];

	function convertReflectionToStatements(metadata: [string, f.ConvertableExpression][], property?: string) {
		const statements = metadata.map(([name, value]) => {
			const args = [node.name!, name, value];
			if (property !== undefined) {
				args.push(property);
			}

			return f.statement(f.call(f.field(importIdentifier, "defineMetadata"), args));
		});

		addSectionComment(statements[0], node, property, "metadata");

		return statements;
	}
}

function generateFieldMetadata(state: TransformState, metadata: NodeMetadata, field: ts.PropertyDeclaration) {
	const fields = new Array<[string, f.ConvertableExpression]>();
	const type = state.typeChecker.getTypeAtLocation(field);

	if (metadata.isRequested(MetadataKeys.type)) {
		if (!field.type) {
			const id = getTypeUid(state, type, field.name ?? field);
			fields.push([MetadataKeys.type, id]);
		} else {
			const id = getNodeUid(state, field.type);
			fields.push([MetadataKeys.type, id]);
		}
	}

	if (metadata.isRequested(MetadataKeys.guard)) {
		const guard = buildGuardFromType(state, field.type ?? field, type);
		fields.push([MetadataKeys.guard, guard]);
	}

	return fields;
}

function generateMethodMetadata(state: TransformState, metadata: NodeMetadata, method: ts.FunctionLikeDeclaration) {
	const fields = new Array<[string, f.ConvertableExpression]>();
	const baseSignature = state.typeChecker.getSignatureFromDeclaration(method);
	if (!baseSignature) return [];

	if (metadata.isRequested(MetadataKeys.returnType)) {
		if (!method.type) {
			const id = getTypeUid(state, baseSignature.getReturnType(), method.name ?? method);
			fields.push([MetadataKeys.returnType, id]);
		} else {
			const id = getNodeUid(state, method.type);
			fields.push([MetadataKeys.returnType, id]);
		}
	}

	if (metadata.isRequested(MetadataKeys.returnGuard)) {
		const guard = buildGuardFromType(state, method.type ?? method, baseSignature.getReturnType());
		fields.push([MetadataKeys.returnGuard, guard]);
	}

	const parameters = new Array<string>();
	const parameterNames = new Array<string>();
	const parameterGuards = new Array<ts.Expression>();

	for (const parameter of method.parameters) {
		if (metadata.isRequested(MetadataKeys.parameters)) {
			if (parameter.type) {
				const id = getNodeUid(state, parameter.type);
				parameters.push(id);
			} else {
				const type = state.typeChecker.getTypeAtLocation(parameter);
				const id = getTypeUid(state, type, parameter);
				parameters.push(id);
			}
		}

		if (metadata.isRequested(MetadataKeys.parameterNames)) {
			if (f.is.identifier(parameter.name)) {
				parameterNames.push(parameter.name.text);
			} else {
				parameterNames.push("_binding_");
			}
		}

		if (metadata.isRequested(MetadataKeys.parameterGuards)) {
			const type = state.typeChecker.getTypeAtLocation(parameter);
			const guard = buildGuardFromType(state, parameter, type);
			parameterGuards.push(guard);
		}
	}

	if (parameters.length > 0) {
		fields.push([MetadataKeys.parameters, parameters]);
	}

	if (parameterNames.length > 0) {
		fields.push([MetadataKeys.parameterNames, parameterNames]);
	}

	if (parameterGuards.length > 0) {
		fields.push([MetadataKeys.parameterGuards, parameterGuards]);
	}

	return fields;
}

function generateClassMetadata(
	state: TransformState,
	hasDecorators: boolean,
	metadata: NodeMetadata,
	node: ts.ClassDeclaration,
) {
	const fields: [string, f.ConvertableExpression][] = [];

	// Decorated classes always get an identifier, so dependency ids resolve through `Reflect.idToObj`.
	if (hasDecorators || metadata.isRequested(MetadataKeys.identifier)) {
		fields.push([MetadataKeys.identifier, getNodeUid(state, node)]);
	}

	const constructor = node.members.find((x): x is ts.ConstructorDeclaration => f.is.constructor(x));
	if (constructor) {
		fields.push(...generateMethodMetadata(state, metadata, constructor));
	}

	if (node.heritageClauses) {
		const implementClauses = new Array<ts.StringLiteral>();
		for (const clause of node.heritageClauses) {
			if (clause.token !== ts.SyntaxKind.ImplementsKeyword) continue;

			for (const type of clause.types) {
				implementClauses.push(f.string(getNodeUid(state, type)));
			}
		}

		if (implementClauses.length > 0 && metadata.isRequested(MetadataKeys.implements)) {
			fields.push([MetadataKeys.implements, f.array(implementClauses, false)]);
		}
	}

	return fields;
}

function getNodeReflection(
	state: TransformState,
	node: ts.ClassDeclaration | ts.ClassElement,
	metadata = new NodeMetadata(state, node),
) {
	if (f.is.methodDeclaration(node)) {
		return generateMethodMetadata(state, metadata, node);
	} else if (f.is.propertyDeclaration(node)) {
		return generateFieldMetadata(state, metadata, node);
	}
}

function getDecoratorStatements(
	state: TransformState,
	declaration: ts.ClassDeclaration,
	node: ts.ClassDeclaration | ts.ClassElement,
	metadata = new NodeMetadata(state, node),
): ts.Statement[] {
	if (!node.name) {
		return [];
	}

	const isClass = f.is.classDeclaration(node);
	const symbol = state.getSymbol(node.name);
	const propertyName = ts.getNameFromPropertyName(node.name);
	assert(propertyName);
	assert(symbol);
	const decoratorStatements = new Array<ts.Statement>();

	const decorators = ts.canHaveDecorators(node) ? ts.getDecorators(node) : undefined;
	if (decorators) {
		// Decorators apply last->first, so we iterate the decorators in reverse.
		for (let i = decorators.length - 1; i >= 0; i--) {
			const decorator = decorators[i];
			const expr = decorator.expression;
			if (isTypeTorchDecorator(state, decorator)) {
				const identifier = f.is.call(expr) ? expr.expression : expr;
				const symbol = state.getSymbol(identifier);
				assert(symbol);
				assert(symbol.valueDeclaration);

				const importIdentifier = state.getReflect(state.getSourceFile(node));
				const args = f.is.call(expr) ? expr.arguments.map((v) => state.transformNode(v)) : [];
				const propertyArgs = !f.is.classDeclaration(node)
					? [propertyName, (node.modifierFlagsCache & ts.ModifierFlags.Static) !== 0]
					: [];

				decoratorStatements.push(
					f.statement(
						f.call(f.field(importIdentifier, "decorate"), [
							declaration.name!,
							getSymbolUid(state, symbol, identifier),
							identifier,
							[...args],
							...propertyArgs,
						]),
					),
				);
			}
		}
	}

	const constraintTypes = metadata.getType("constraint");
	const nodeType = state.typeChecker.getTypeOfSymbolAtLocation(symbol, node);
	for (const constraintType of constraintTypes ?? []) {
		if (!state.typeChecker.isTypeAssignableTo(nodeType, constraintType)) {
			Diagnostics.addDiagnostic(
				getAssignabilityDiagnostics(
					node.name ?? node,
					nodeType,
					constraintType,
					metadata.getTrace(constraintType),
				),
			);
		}
	}

	addSectionComment(decoratorStatements[0], declaration, isClass ? undefined : propertyName, "decorators");
	return decoratorStatements;
}

function addSectionComment(
	node: ts.Node | undefined,
	declaration: ts.ClassDeclaration,
	property: string | undefined,
	label: string,
) {
	if (!node) {
		return;
	}

	const elementName = property === undefined ? `${declaration.name!.text}` : `${declaration.name!.text}.${property}`;
	ts.addSyntheticLeadingComment(node, ts.SyntaxKind.SingleLineCommentTrivia, ` (TypeTorch) ${elementName} ${label}`);
}

function formatType(type: ts.Type) {
	const typeNode = type.checker.typeToTypeNode(
		type,
		undefined,
		ts.NodeBuilderFlags.InTypeAlias | ts.NodeBuilderFlags.IgnoreErrors,
	)!;

	const printer = ts.createPrinter();
	return printer.printNode(ts.EmitHint.Unspecified, typeNode, undefined!);
}

function getAssignabilityDiagnostics(
	node: ts.Node,
	sourceType: ts.Type,
	constraintType: ts.Type,
	trace?: ts.Node,
): ts.DiagnosticWithLocation {
	const diagnostic = Diagnostics.createDiagnostic(
		node,
		ts.DiagnosticCategory.Error,
		`Type '${formatType(sourceType)}' does not satisfy constraint '${formatType(constraintType)}'`,
	);

	if (trace) {
		ts.addRelatedInfo(
			diagnostic,
			Diagnostics.createDiagnostic(trace, ts.DiagnosticCategory.Message, "The constraint is defined here."),
		);
	}

	return diagnostic;
}

function updateClass(state: TransformState, node: ts.ClassDeclaration, staticStatements?: ts.Statement[]) {
	const modifiers = getAllModifiers(node);
	const members = node.members
		.map((node) => state.transformNode(node))
		.map((member) => {
			// Strip TypeTorch decorators from members
			const modifiers = getAllModifiers(member);
			if (modifiers) {
				const filteredModifiers = transformModifiers(state, modifiers);
				if (f.is.propertyDeclaration(member)) {
					return f.update.propertyDeclaration(member, undefined, undefined, filteredModifiers);
				} else if (f.is.methodDeclaration(member)) {
					return f.update.methodDeclaration(
						member,
						undefined,
						undefined,
						undefined,
						undefined,
						filteredModifiers,
					);
				}
			}

			return member;
		});

	if (staticStatements && staticStatements.length > 0) {
		members.push(f.staticBlockDeclaration(staticStatements));
	}

	return f.update.classDeclaration(
		node,
		node.name ? state.transformNode(node.name) : undefined,
		members,
		node.heritageClauses,
		node.typeParameters,
		modifiers && transformModifiers(state, modifiers),
	);
}

function getAllModifiers(node: ts.Node) {
	return ts.canHaveDecorators(node) || ts.canHaveModifiers(node) ? node.modifiers : undefined;
}

function transformModifiers(state: TransformState, modifiers: readonly ts.ModifierLike[]) {
	return modifiers
		.filter((modifier) => !ts.isDecorator(modifier) || !isTypeTorchDecorator(state, modifier))
		.map((decorator) => state.transform(decorator));
}

function isTypeTorchDecorator(state: TransformState, decorator: ts.Decorator) {
	const decoratorType = state.typeChecker.getTypeAtLocation(decorator.expression);
	return decoratorType.getProperty(DECORATOR_MARKER) !== undefined;
}

function hasTypeTorchDecorators(state: TransformState, declaration: ts.ClassDeclaration) {
	const nodeDecorators = ts.canHaveDecorators(declaration) ? ts.getDecorators(declaration) : undefined;
	if (nodeDecorators && nodeDecorators.some((v) => isTypeTorchDecorator(state, v))) {
		return true;
	}

	for (const member of declaration.members) {
		const nodeDecorators = ts.canHaveDecorators(member) ? ts.getDecorators(member) : undefined;
		if (nodeDecorators && nodeDecorators.some((v) => isTypeTorchDecorator(state, v))) {
			return true;
		}
	}

	return false;
}
