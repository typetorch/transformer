/**
 * Stand-in for @typetorch/framework's `Lazy<T>` (framework/src/runtime/dependency.ts): the transformer reads the
 * type-only marker `_typetorch_lazy` and records `lazy:<id of T>` for a constructor parameter of this type.
 */
export interface Lazy<T> {
	get(): T;
	/** @hidden */
	readonly _typetorch_lazy: T;
}
