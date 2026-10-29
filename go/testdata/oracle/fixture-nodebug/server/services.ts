import {} from "@typetorch/framework";
import { t as t } from "@rbxts/t";
import { FrameworkLogger, Reflect, Service, registered, type Lazy } from "@typetorch/framework";
import { argNames, describe, guardOf, here, idOf } from "../shared/macros";
import { network, Payload } from "../shared/network";
export class DataService {
    get(key: string) {
        return key;
    }
    static {
        // (TypeTorch) DataService metadata
        Reflect["defineMetadata"](DataService, "identifier", "server/services@DataService");
    }
}
// (TypeTorch) DataService decorators
Reflect["decorate"](DataService, "@typetorch/framework:decorators@Service", Service, []);
/** Two injected constructor dependencies: one from this game, one declared in the framework package. */
export class ShopService {
    constructor(private readonly data: DataService, private readonly logger: FrameworkLogger) { }
    buy(player: Player, itemId: string) {
        this.logger.log(`${player.Name} bought ${this.data.get(itemId)}`);
    }
    static {
        // (TypeTorch) ShopService metadata
        Reflect["defineMetadata"](ShopService, "identifier", "server/services@ShopService");
        Reflect["defineMetadata"](ShopService, "typetorch:parameters", [
            "server/services@DataService",
            "@typetorch/framework:fixture@FrameworkLogger"
        ]);
    }
}
// (TypeTorch) ShopService decorators
Reflect["decorate"](ShopService, "@typetorch/framework:decorators@Service", Service, [
    { loadOrder: 10 }
]);
/** Lazy<T> constructor parameters (0.2.1): `lazy:<id of T>`, so two modules can take each other. */
export class TeamService {
    constructor(private readonly items: Lazy<ItemService>, private readonly data: DataService) { }
    static {
        // (TypeTorch) TeamService metadata
        Reflect["defineMetadata"](TeamService, "identifier", "server/services@TeamService");
        Reflect["defineMetadata"](TeamService, "typetorch:parameters", [
            "lazy:server/services@ItemService",
            "server/services@DataService"
        ]);
    }
}
// (TypeTorch) TeamService decorators
Reflect["decorate"](TeamService, "@typetorch/framework:decorators@Service", Service, []);
export class ItemService {
    constructor(private readonly team: Lazy<TeamService>) { }
    static {
        // (TypeTorch) ItemService metadata
        Reflect["defineMetadata"](ItemService, "identifier", "server/services@ItemService");
        Reflect["defineMetadata"](ItemService, "typetorch:parameters", [
            "lazy:server/services@TeamService"
        ]);
    }
}
// (TypeTorch) ItemService decorators
Reflect["decorate"](ItemService, "@typetorch/framework:decorators@Service", Service, []);
export function runChecks() {
    const parameters = Reflect.getOwnMetadata<string[]>(ShopService, "typetorch:parameters") ?? [];
    const loggerId = idOf<FrameworkLogger>("@typetorch/framework:fixture@FrameworkLogger" as never);
    const dataId = idOf<DataService>("server/services@DataService" as never);
    const resolved = parameters.map((id) => Reflect.idToObj.get(id));
    const isPayload = guardOf<Payload>(t["interface"]({
        "id": t["string"],
        "meta": t["optional"](t["interface"]({
            "color": t["Color3"],
            "tags": t["array"](t["string"])
        }))
    }) as never);
    // Read by scripts/runtime.luau (Lune), which runs this code from fixture.rbxm.
    return {
        reflect: Reflect,
        parameters: parameters.join(","),
        teamParameters: (Reflect.getOwnMetadata<string[]>(TeamService, "typetorch:parameters") ?? []).join(","),
        resolved: resolved[0] === DataService && resolved[1] === FrameworkLogger,
        registered: registered.size(),
        payloadGood: isPayload({ id: "a", meta: { color: new Color3(1, 0, 0), tags: ["x"] } }),
        payloadBad: isPayload({ id: 1 }),
        guards: network.clientToServer.size() + network.serverToClient.size(),
    };
}
