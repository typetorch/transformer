/** Stand-in for framework/src/runtime/registry.ts: filled by @Service / @Controller as modules load. */
export interface ModuleConfig {
	loadOrder?: number;
}

export interface RegisteredModule {
	readonly ctor: object;
	readonly realm: "server" | "client";
	readonly config: ModuleConfig;
}

export const registered = new Array<RegisteredModule>();
