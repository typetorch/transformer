import {} from "@typetorch/framework";
import { Controller, Module, Reflect, type OnStart } from "@typetorch/framework";
import { network } from "../shared/network";
/** Extends the package's abstract base and declares no constructor: an identifier, no dependency ids. */
export class HudController extends Module implements OnStart {
    onStart() {
        print("[src/client/main.ts:9]", "hud started");
    }
    static {
        // (TypeTorch) HudController metadata
        Reflect["defineMetadata"](HudController, "identifier", "client/main@HudController");
    }
}
// (TypeTorch) HudController decorators
Reflect["decorate"](HudController, "@typetorch/framework:decorators@Controller", Controller, []);
/** A derived class whose constructor (with super()) injects another controller. */
export class MenuController extends Module {
    constructor(private readonly hud: HudController) {
        super();
    }
    open() {
        this.hud.onStart();
    }
    static {
        // (TypeTorch) MenuController metadata
        Reflect["defineMetadata"](MenuController, "identifier", "client/main@MenuController");
        Reflect["defineMetadata"](MenuController, "typetorch:parameters", [
            "client/main@HudController"
        ]);
    }
}
// (TypeTorch) MenuController decorators
Reflect["decorate"](MenuController, "@typetorch/framework:decorators@Controller", Controller, [
    { loadOrder: 1 }
]);
export function start() {
    print("[src/client/main.ts:26]", `client sees ${network.serverToClient.size()} server -> client guards`);
    return Reflect.getOwnMetadata<string[]>(MenuController, "typetorch:parameters");
}
