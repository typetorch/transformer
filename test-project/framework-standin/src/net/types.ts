import type { Modding } from "../reflection/modding";

/** Same as framework/src/net/types.ts: one guard per leaf, nested namespaces recurse. */
export type GuardTree<T> = {
	[K in keyof T]: T[K] extends (...args: infer A) => unknown ? Modding.Generic<A, "guard"> : GuardTree<T[K]>;
};
