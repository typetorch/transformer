import { randomUUID } from "crypto";
import ts from "typescript";
import { Diagnostics } from "../classes/diagnostics";
import { TransformState } from "../classes/transformState";
import { f } from "../util/factory";
import { buildGuardFromTypeWithDedup } from "../util/functions/buildGuardFromType";
import { getTypeUid } from "../util/uid";
import { isTupleType } from "../util/functions/isTupleType";

/**
 * Property names that mark TypeTorch's macro types (see `Modding` in the runtime kit). They only exist in the type
 * system: a parameter typed `Modding.Generic<T, "guard">` has a `_typetorch_macro_generic: [T, "guard"]` property.
 */
export const MACRO_MARKERS = {
	generic: "_typetorch_macro_generic",
	many: "_typetorch_macro_many",
	caller: "_typetorch_macro_caller",
	tupleLabels: "_typetorch_macro_tuple_labels",
} as const;

/**
 * Fills in the omitted macro parameters of a call to a function declared with `@metadata macro`.
 */
export function transformUserMacro(
	state: TransformState,
	node: ts.NewExpression | ts.CallExpression,
	signature: ts.Signature,
): ts.Expression | undefined {
	const args = node.arguments ? [...node.arguments] : [];
	const parameters = new Map<number, UserMacro>();

	let highestParameterIndex = -1;
	for (let i = 0; i < getParameterCount(state, signature); i++) {
		// This parameter is passed explicitly, so we don't need to evaluate it.
		if (!isUndefinedArgument(args[i])) {
			continue;
		}

		const targetParameter = state.typeChecker.getParameterType(signature, i).getNonNullableType();
		const userMacro = getUserMacroOfUnion(state, node, targetParameter);
		if (userMacro) {
			parameters.set(i, userMacro);
			highestParameterIndex = Math.max(highestParameterIndex, i);
		}
	}

	for (let i = 0; i <= highestParameterIndex; i++) {
		const userMacro = parameters.get(i);
		if (userMacro) {
			args[i] = buildUserMacro(state, node, userMacro);
		} else {
			args[i] = args[i] ? state.transform(args[i]) : f.nil();
		}
	}

	const name = state.transformNode(node.expression);

	if (ts.isNewExpression(node)) {
		return ts.factory.updateNewExpression(node, name, node.typeArguments, args);
	} else if (ts.isCallExpression(node)) {
		return ts.factory.updateCallExpression(node, name, node.typeArguments, args);
	} else {
		Diagnostics.error(node, `Macro could not be transformed.`);
	}
}

function isUndefinedArgument(argument: ts.Node | undefined) {
	return argument ? f.is.identifier(argument) && argument.text === "undefined" : true;
}

function getLabels(state: TransformState, type: ts.Type): UserMacro {
	if (!isTupleType(state, type)) {
		return {
			kind: "literal",
			value: undefined,
		};
	}

	const names = new Array<UserMacro>();
	const declarations = type.target.labeledElementDeclarations;

	if (!declarations) {
		return {
			kind: "literal",
			value: undefined,
		};
	}

	for (const namedMember of declarations) {
		// TypeScript 5.0+ allows nameless tuple elements, so we'll default to an empty string in that case.
		names.push({
			kind: "literal",
			value: namedMember ? (namedMember.name as ts.Identifier).text : "",
		});
	}

	return {
		kind: "many",
		members: names,
	};
}

function buildUserMacro(state: TransformState, node: ts.Expression, macro: UserMacro): ts.AsExpression {
	if (macro.kind === "generic") {
		const metadata = getGenericMetadata(macro);
		if (metadata) {
			return f.asNever(metadata);
		}
	} else if (macro.kind === "caller") {
		const metadata = getCallerMetadata(macro);
		if (metadata) {
			return f.asNever(metadata);
		}
	} else if (macro.kind === "many") {
		if (Array.isArray(macro.members)) {
			return f.asNever(f.array(macro.members.map((userMacro) => buildUserMacro(state, node, userMacro))));
		} else {
			const elements = new Array<ts.ObjectLiteralElementLike>();

			for (const [name, userMacro] of macro.members) {
				const expression = buildUserMacro(state, node, userMacro);
				if (f.is.nil(expression.expression)) {
					continue;
				}

				elements.push(f.propertyAssignmentDeclaration(f.string(name), expression));
			}

			return f.asNever(f.object(elements, false));
		}
	} else if (macro.kind === "literal") {
		const value = macro.value;
		return f.asNever(
			typeof value === "string"
				? f.string(value)
				: typeof value === "number"
				? f.number(value)
				: typeof value === "boolean"
				? f.bool(value)
				: f.nil(),
		);
	}

	return f.asNever(f.nil());

	function getGenericMetadata(macro: UserMacro & { kind: "generic" }) {
		if (macro.metadata === "id") {
			return f.string(getTypeUid(state, macro.target, node));
		}

		if (macro.metadata === "guard") {
			const result = buildGuardFromTypeWithDedup(state, node, macro.target);
			state.prereqList(result.statements);

			return result.guard;
		}

		if (macro.metadata === "text") {
			return f.string(state.typeChecker.typeToString(macro.target));
		}
	}

	function getCallerMetadata(macro: UserMacro & { kind: "caller" }) {
		const lineAndCharacter = ts.getLineAndCharacterOfPosition(node.getSourceFile(), node.getStart());

		if (macro.metadata === "line") {
			return f.number(lineAndCharacter.line + 1);
		}

		if (macro.metadata === "character") {
			return f.number(lineAndCharacter.character + 1);
		}

		if (macro.metadata === "width") {
			return f.number(node.getWidth());
		}

		if (macro.metadata === "uuid") {
			return f.string(randomUUID());
		}

		if (macro.metadata === "text") {
			return f.string(node.getText());
		}
	}
}

function getMetadataFromType(metadataType: ts.Type) {
	if (metadataType.isStringLiteral()) {
		return metadataType.value;
	}
}

function getUserMacroOfMany(state: TransformState, node: ts.Expression, target: ts.Type): UserMacro | undefined {
	const basicUserMacro = getBasicUserMacro(state, node, target);
	if (basicUserMacro) {
		return basicUserMacro;
	}

	const manyMetadata = state.typeChecker.getTypeOfPropertyOfType(target, MACRO_MARKERS.many);
	if (manyMetadata) {
		return getUserMacroOfMany(state, node, manyMetadata);
	}

	if (isTupleType(state, target)) {
		const userMacros = new Array<UserMacro>();

		for (const member of state.typeChecker.getTypeArguments(target)) {
			const userMacro = getUserMacroOfMany(state, node, member);
			if (!userMacro) return;

			userMacros.push(userMacro);
		}

		return {
			kind: "many",
			members: userMacros,
		};
	} else if (state.typeChecker.isArrayType(target)) {
		const targetType = state.typeChecker.getTypeArguments(target as ts.TypeReference)[0];
		const constituents = targetType.isUnion() ? targetType.types : [targetType];
		const userMacros = new Array<UserMacro>();

		for (const member of constituents) {
			// `never` may be encountered when a union has no constituents, so we should just return an empty array.
			if (member.flags & ts.TypeFlags.Never) {
				break;
			}

			const userMacro = getUserMacroOfMany(state, node, member);
			if (!userMacro) return;

			userMacros.push(userMacro);
		}

		return {
			kind: "many",
			members: userMacros,
		};
	} else if (isObjectType(target)) {
		const userMacros = new Map<string, UserMacro>();

		for (const member of target.getProperties()) {
			const memberType = state.typeChecker.getTypeOfPropertyOfType(target, member.name);
			if (!memberType) return;

			const userMacro = getUserMacroOfMany(state, node, memberType);
			if (!userMacro) return;

			userMacros.set(member.name, userMacro);
		}

		return {
			kind: "many",
			members: userMacros,
		};
	} else if (target.isStringLiteral() || target.isNumberLiteral()) {
		return {
			kind: "literal",
			value: target.value,
		};
	} else if (target.flags & ts.TypeFlags.Undefined) {
		return {
			kind: "literal",
			value: undefined,
		};
	} else if (target.flags & ts.TypeFlags.BooleanLiteral) {
		return {
			kind: "literal",
			value: (target as ts.FreshableType).regularType === state.typeChecker.getTrueType() ? true : false,
		};
	}

	Diagnostics.error(node, `Unknown type '${target.checker.typeToString(target)}' encountered`);
}

function getBasicUserMacro(state: TransformState, node: ts.Expression, target: ts.Type): UserMacro | undefined {
	const genericMetadata = state.typeChecker.getTypeOfPropertyOfType(target, MACRO_MARKERS.generic);
	if (genericMetadata) {
		const targetType = state.typeChecker.getTypeOfPropertyOfType(genericMetadata, "0");
		const metadataType = state.typeChecker.getTypeOfPropertyOfType(genericMetadata, "1");
		if (!targetType) return;
		if (!metadataType) return;

		const metadata = getMetadataFromType(metadataType);
		if (!metadata) {
			Diagnostics.error(
				node,
				`@typetorch/transformer encountered invalid metadata: '${state.typeChecker.typeToString(metadataType)}'`,
			);
		}

		return {
			kind: "generic",
			target: targetType,
			metadata,
		};
	}

	const callerMetadata = state.typeChecker.getTypeOfPropertyOfType(target, MACRO_MARKERS.caller);
	if (callerMetadata) {
		const metadata = getMetadataFromType(callerMetadata);
		if (!metadata) return;

		return {
			kind: "caller",
			metadata,
		};
	}

	const nonNullableTarget = target.getNonNullableType();
	const labelMetadata = state.typeChecker.getTypeOfPropertyOfType(nonNullableTarget, MACRO_MARKERS.tupleLabels);
	if (labelMetadata) {
		return getLabels(state, labelMetadata);
	}
}

function getUserMacroOfType(state: TransformState, node: ts.Expression, target: ts.Type): UserMacro | undefined {
	const manyMetadata = state.typeChecker.getTypeOfPropertyOfType(target, MACRO_MARKERS.many);
	if (manyMetadata) {
		return getUserMacroOfMany(state, node, manyMetadata);
	} else {
		return getBasicUserMacro(state, node, target);
	}
}

/**
 * This allows user macros to specify signatures that can accept non-metadata.
 * Multiple modding types in a single parameter aren't supported, and the first one is chosen.
 *
 * For example, `string | Modding.Generic<T, "id">`, will generate the ID for `T`, but also allow users to pass in one manually.
 */
function getUserMacroOfUnion(state: TransformState, node: ts.Expression, target: ts.Type) {
	if (!target.isUnion()) {
		return getUserMacroOfType(state, node, target);
	}

	for (const constituent of target.types) {
		const macro = getUserMacroOfType(state, node, constituent);
		if (macro) {
			return macro;
		}
	}
}

function isObjectType(type: ts.Type): boolean {
	return type.isIntersection() ? type.types.every(isObjectType) : (type.flags & ts.TypeFlags.Object) !== 0;
}

function getParameterCount(state: TransformState, signature: ts.Signature) {
	const length = signature.parameters.length;
	if (ts.signatureHasRestParameter(signature)) {
		const restType = state.typeChecker.getTypeOfSymbol(signature.parameters[length - 1]);
		if (isTupleType(state, restType)) {
			return length + restType.target.fixedLength - (restType.target.hasRestElement ? 0 : 1);
		}
	}
	return length;
}

export type UserMacro =
	| {
			kind: "generic";
			target: ts.Type;
			metadata: string;
	  }
	| {
			kind: "caller";
			metadata: string;
	  }
	| {
			kind: "many";
			members: Map<string, UserMacro> | Array<UserMacro>;
	  }
	| {
			kind: "literal";
			value: string | number | boolean | undefined;
	  };
