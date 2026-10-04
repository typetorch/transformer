import { Modding } from "@typetorch/framework";
import { $assert } from "rbxts-transform-debug";

/**
 * Macros declared in the game itself (the other case is createNetwork, declared in a package).
 */

/** @metadata macro */
export function idOf<T>(id?: Modding.Generic<T, "id">): string {
	$assert(id, "id macro was not filled in by the transformer");
	return id;
}

/** @metadata macro */
export function guardOf<T>(guard?: Modding.Generic<T, "guard">): (value: unknown) => value is T {
	$assert(guard, "guard macro was not filled in by the transformer");
	return guard;
}

/** @metadata macro */
export function describe<T>(info?: Modding.GenericMany<T, "id" | "text">): { id: string; text: string } {
	$assert(info, "describe macro was not filled in by the transformer");
	return info;
}

/** @metadata macro */
export function here(line?: Modding.Caller<"line">, text?: Modding.Caller<"text">): string {
	return `${text} @ line ${line}`;
}

/** @metadata macro */
export function argNames<F extends (...args: never[]) => unknown>(names?: Modding.TupleLabels<Parameters<F>>) {
	return names ?? [];
}
