import ts from "typescript";
import { Diagnostics } from "../../classes/diagnostics";
import { TransformState } from "../../classes/transformState";
import { isTupleType } from "./isTupleType";

/**
 * What a network leaf's parameters may be (shown with every network guard error): what the guard builder can check and
 * Roblox can send over a RemoteEvent.
 */
export const SUPPORTED_NETWORK_TYPES =
	"Supported parameter types: string, number, boolean, undefined/optional, literals and TS enums, Enum items, " +
	"Roblox datatypes (Vector3, CFrame, Color3, UDim2, ...), Instances by class (BasePart, Model, Player, ...), " +
	"arrays, tuples, Maps, Sets, plain objects/interfaces, unions, buffer, and unknown/any.";

/** Types that never arrive on the other side of a RemoteEvent (Roblox sends nil), with what to send instead. */
const UNSENDABLE: Record<string, string> = {
	thread: "a thread can't be sent over the network (it arrives as nil)",
	RBXScriptSignal: "an RBXScriptSignal can't be sent over the network (it arrives as nil): send the data instead",
	RBXScriptConnection: "an RBXScriptConnection can't be sent over the network (it arrives as nil)",
};

/** Instance classes that exist only where they were made, so a reference to one arrives as nil. */
const LOCAL_ONLY_INSTANCES: Record<string, string> = {
	AnimationTrack:
		"an AnimationTrack doesn't replicate (it arrives as nil): send the Animation or its AnimationId and load the track on the other side",
};

const MAX_DEPTH = 4;

function isInstanceType(type: ts.Type) {
	return type.getProperty("_nominal_Instance") !== undefined;
}

function typeName(type: ts.Type): string | undefined {
	return type.aliasSymbol?.name ?? type.getSymbol()?.name;
}

/** Why a value of `type` can't cross the network, if it can't. */
function unsendableReason(type: ts.Type): string | undefined {
	if (isInstanceType(type)) {
		for (const [className, reason] of Object.entries(LOCAL_ONLY_INSTANCES)) {
			if (type.getProperty(`_nominal_${className}`) !== undefined) return reason;
		}
		return undefined;
	}
	const name = typeName(type);
	if (name !== undefined && UNSENDABLE[name] !== undefined) return UNSENDABLE[name];
	if (type.getCallSignatures().length > 0) return "a function can't be sent over the network (it arrives as nil)";
	return undefined;
}

/** Plain object types whose properties are worth looking into (not Instances, datatypes, Maps or arrays). */
function isPlainObject(state: TransformState, type: ts.Type) {
	if ((type.flags & ts.TypeFlags.Object) === 0 || isInstanceType(type)) return false;
	if (state.typeChecker.isArrayType(type) || state.typeChecker.isTupleType(type)) return false;
	const declaration = type.getSymbol()?.declarations?.[0];
	// Roblox datatypes and engine types come from the @rbxts type packages: their fields aren't sent one by one.
	if (declaration && /[\\/]node_modules[\\/]@rbxts[\\/](types|compiler-types)[\\/]/.test(declaration.getSourceFile().fileName)) return false;
	return true;
}

/**
 * Warnings for a network leaf (a `Modding.Generic<A, "guard">` inside a `@metadata macro network` call's tree) whose
 * guard compiles but can't do its job: a generic or conditional signature (the guard checks each type parameter as its
 * constraint and each conditional type as either branch, so the arguments' relation isn't enforced), and parameters
 * that never arrive (functions, threads, signals, connections, AnimationTracks: Roblox sends nil, so a client -> server
 * guard rejects every message and the server -> client side gets nil).
 */
export function checkNetworkLeaf(
	state: TransformState,
	at: ts.Node,
	leaf: string,
	params: ts.Type,
	declaration: ts.Node | undefined,
) {
	const checker = state.typeChecker;
	const generic = new Array<string>();
	const unsendable = new Array<string>();
	const seen = new Set<ts.Type>();

	// `infer A` erases a generic signature (each type parameter becomes its constraint), so look at the declared leaf.
	if (declaration) {
		for (const signature of checker.getTypeAtLocation(declaration).getCallSignatures()) {
			for (const parameter of signature.typeParameters ?? []) {
				const constraint = checker.getBaseConstraintOfType(parameter);
				const text = `${checker.typeToString(parameter)} as ${constraint ? checker.typeToString(constraint) : "unknown"}`;
				if (!generic.includes(text)) generic.push(text);
			}
		}
	}

	const walk = (type: ts.Type, where: string, depth: number) => {
		if (depth > MAX_DEPTH || seen.has(type)) return;
		seen.add(type);
		if (type.flags & ts.TypeFlags.TypeParameter) {
			const constraint = checker.getBaseConstraintOfType(type);
			const text = `${checker.typeToString(type)} as ${constraint ? checker.typeToString(constraint) : "unknown"}`;
			if (!generic.includes(text)) generic.push(text);
			return;
		}
		if (type.flags & ts.TypeFlags.Conditional) {
			const text = `${checker.typeToString(type)} as either branch`;
			if (!generic.includes(text)) generic.push(text);
			return;
		}
		if (type.isUnionOrIntersection()) {
			for (const member of type.types) walk(member, where, depth);
			return;
		}
		const reason = unsendableReason(type);
		if (reason !== undefined) {
			unsendable.push(`${where}: ${reason}.`);
			return;
		}
		if (checker.isArrayType(type)) {
			const element = checker.getTypeArguments(type as ts.TypeReference)[0];
			if (element) walk(element, `${where}[]`, depth + 1);
			return;
		}
		if (isPlainObject(state, type)) {
			for (const property of type.getProperties()) {
				const propertyType = checker.getTypeOfPropertyOfType(type, property.name);
				if (propertyType) walk(propertyType, `${where}.${property.name}`, depth + 1);
			}
		}
	};

	if (isTupleType(state, params)) {
		const labels = params.target.labeledElementDeclarations;
		checker.getTypeArguments(params).forEach((element, index) => {
			const label = labels?.[index]?.name;
			const name = label && ts.isIdentifier(label) ? `"${label.text}"` : `${index + 1}`;
			walk(element, `parameter ${name}`, 0);
		});
	} else {
		walk(params, "its parameters", 0);
	}

	if (generic.length > 0) {
		Diagnostics.warning(
			at,
			`Network leaf "${leaf}" has a generic or conditional signature: its guard checks ${generic.join(", ")} ` +
				"(and a conditional type as either branch), so it can't enforce how the arguments go together. " +
				"Declare a union of tuples instead, " +
				"e.g. `(...args: [box: Model, group: string] | [box: undefined]) => void`, or use separate leaves.",
		);
	}
	for (const problem of unsendable) {
		Diagnostics.warning(at, `Network leaf "${leaf}", ${problem}`);
	}
}
