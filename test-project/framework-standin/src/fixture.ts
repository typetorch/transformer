import { Modding } from "./reflection";

/**
 * Framework-like declarations, compiled as part of the package. The game sees them only through the published
 * `.d.ts` files, which is the case that matters: JSDoc tags and macro marker types must survive declaration emit.
 */

export interface ModuleConfig {
	loadOrder?: number;
}

export interface RegisteredModule {
	readonly ctor: object;
	readonly config: ModuleConfig;
}

/** Filled by @Service as modules load. */
export const registered = new Array<RegisteredModule>();

/**
 * Stand-in for framework/src/decorators.ts. The JSDoc tag asks for the constructor's dependency ids.
 *
 * @metadata typetorch:parameters injectable
 */
export const Service = Modding.createDecorator<[config?: ModuleConfig]>("Class", (descriptor, [config]) => {
	registered.push({ ctor: descriptor.object, config: config ?? {} });
});

/**
 * A module declared inside the package. Its `identifier` metadata is written by the package's own compile; games
 * compute the same id from the `.d.ts` when they inject it.
 */
@Service({ loadOrder: -1 })
export class FrameworkLogger {
	log(message: string) {
		print(`[framework] ${message}`);
	}
}

type Guard = (value: unknown) => boolean;

/** Same shape as framework/src/net/types.ts: one guard per leaf, nested namespaces recurse. */
export type GuardTree<T> = {
	[K in keyof T]: T[K] extends (...args: infer A) => unknown ? Modding.Generic<A, "guard"> : GuardTree<T[K]>;
};

function flatten(tree: unknown, prefix: string, into: Map<string, Guard>) {
	for (const [key, value] of pairs(tree as Record<string, unknown>)) {
		const path = prefix === "" ? (key as string) : `${prefix}.${key}`;
		if (typeIs(value, "function")) into.set(path, value as Guard);
		else if (typeIs(value, "table")) flatten(value, path, into);
	}
}

export interface NetworkGuards {
	/** Guards for client -> server leaves, by dotted path ("shop.inventory.equip"). */
	readonly clientToServer: ReadonlyMap<string, Guard>;
	readonly serverToClient: ReadonlyMap<string, Guard>;
}

/**
 * Stand-in for framework/src/net createNetwork: a macro declared in a package.
 *
 * @metadata macro
 */
export function createNetwork<ClientToServer extends object, ServerToClient extends object>(
	clientToServer?: Modding.Many<GuardTree<ClientToServer>>,
	serverToClient?: Modding.Many<GuardTree<ServerToClient>>,
): NetworkGuards {
	assert(clientToServer && serverToClient, "createNetwork: guards were not generated (is @typetorch/transformer enabled?)");
	const c2s = new Map<string, Guard>();
	const s2c = new Map<string, Guard>();
	flatten(clientToServer, "", c2s);
	flatten(serverToClient, "", s2c);
	return { clientToServer: c2s, serverToClient: s2c };
}
