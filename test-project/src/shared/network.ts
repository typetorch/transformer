import { createNetwork } from "@typetorch/framework";

export interface Payload {
	id: string;
	meta?: { color: Color3; tags: string[] };
}

/** Two levels of namespaces (shop.inventory.*), plus a flat one (chat). */
export interface ClientToServer {
	shop: {
		buy(itemId: string, amount: number): void;
		inventory: {
			equip(slot: 1 | 2 | 3, itemId?: string): void;
			drop(position: Vector3, part: BasePart): void;
		};
	};
	chat: {
		say(message: string, channel: "all" | "team" | "whisper"): void;
	};
}

export interface ServerToClient {
	shop: {
		stock: {
			changed(itemId: string, left: number, payload: Payload): void;
		};
		prices(prices: Map<string, number>, featured: readonly string[]): void;
	};
	notice(kind: Enum.Material, flag: boolean, extra: unknown): void;
}

export const network = createNetwork<ClientToServer, ServerToClient>();
