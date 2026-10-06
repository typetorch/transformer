// The runtime kit (copied from ../../runtime-kit by scripts/run.ts), exported from the package root exactly as
// @typetorch/framework does, plus stand-ins laid out like the real framework's modules.
export * from "./reflection";
export { Controller, Service } from "./decorators";
export { registered } from "./registry";
export type { ModuleConfig, RegisteredModule } from "./registry";
export { Module } from "./module";
export type { OnStart } from "./module";
export { createNetwork } from "./net";
export type { NetworkGuards } from "./net";
export type { GuardTree } from "./net/types";
export { FrameworkLogger } from "./fixture";
export type { Lazy } from "./lazy";
