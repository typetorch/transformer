import type { Modding } from "../reflection/modding";
import type { GuardTree } from "./types";

type Guard = (value: unknown) => boolean;

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
 * Stand-in for framework/src/net createNetwork: a macro declared in a package, in a different module than the
 * GuardTree type it uses (both reach the game only through `.d.ts` files). `network`: guard
 * problems name the leaf (generic signatures, values that never arrive, types with no guard).
 *
 * @metadata macro network
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
