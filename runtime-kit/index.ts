/**
 * TypeTorch runtime kit: the runtime half of `@typetorch/transformer`.
 *
 * `@typetorch/framework` carries copies of `reflect.ts` and `modding.ts` in `src/reflection/` (this kit is the source
 * of truth; keep them identical) and exports them from its root. Generated code in a game imports `Reflect` (and, when
 * the game can't import `@rbxts/t` itself, `t`) from the package root.
 *
 * The framework compiles without the transformer. A runtime package that does compile with it gets generated code that
 * imports `Reflect` from `src/reflection/reflect` (the transformer's `runtimeReflectPath` default).
 */
export { Reflect } from "./reflect";
export { Modding } from "./modding";
export type { AbstractConstructor, ClassDescriptor, Constructor, MethodDescriptor, PropertyDescriptor } from "./modding";

// Generated guards fall back to this copy of `t` when the game resolves a different (or no) `@rbxts/t`.
export { t } from "@rbxts/t";
