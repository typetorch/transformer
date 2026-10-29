import { t as t } from "@rbxts/t";
import { createNetwork } from "@typetorch/framework";
export interface Payload {
    id: string;
    meta?: {
        color: Color3;
        tags: string[];
    };
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
export const network = createNetwork<ClientToServer, ServerToClient>({ "shop": { "buy": t["strictArray"](t["string"], t["number"]) as never, "inventory": { "equip": t["strictArray"](t["literalList"]([
                2,
                1,
                3
            ]), t["optional"](t["string"])) as never, "drop": t["strictArray"](t["Vector3"], t["instanceIsA"]("BasePart")) as never } as never } as never, "chat": { "say": t["strictArray"](t["string"], t["literalList"]([
            "all",
            "team",
            "whisper"
        ])) as never } as never } as never, { "shop": { "stock": { "changed": t["strictArray"](t["string"], t["number"], t["interface"]({
                "id": t["string"],
                "meta": t["optional"](t["interface"]({
                    "color": t["Color3"],
                    "tags": t["array"](t["string"])
                }))
            })) as never } as never, "prices": t["strictArray"](t["map"](t["string"], t["number"]), t["array"](t["string"])) as never } as never, "notice": t["strictArray"](t["enum"](Enum["Material"]), t["boolean"], t["union"](t["any"], t["none"])) as never } as never);
