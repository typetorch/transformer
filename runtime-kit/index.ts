/**
 * TypeTorch runtime kit: the runtime half of `@typetorch/transformer`.
 *
 * Copy `reflect.ts`, `modding.ts` and this file into `@typetorch/framework` as `src/reflection/`, then add
 * `export * from "./reflection";` to the framework's `src/index.ts`. Generated code imports `Reflect` (and, when the
 * game can't import `@rbxts/t` itself, `t`) from the package root.
 *
 * While the framework compiles itself with the transformer, generated code imports `Reflect` from
 * `src/reflection/reflect` (the transformer's `runtimeReflectPath` default).
 */
export { Reflect } from "./reflect";
export { Modding } from "./modding";
export type { AbstractConstructor, ClassDescriptor, Constructor, MethodDescriptor, PropertyDescriptor } from "./modding";

// Generated guards fall back to this copy of `t` when the game resolves a different (or no) `@rbxts/t`.
export { t } from "@rbxts/t";
