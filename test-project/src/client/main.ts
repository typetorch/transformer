import { Controller, Module, Reflect, type OnStart } from "@typetorch/framework";
import { $print } from "rbxts-transform-debug";
import { network } from "../shared/network";

/** Extends the package's abstract base and declares no constructor: an identifier, no dependency ids. */
@Controller()
export class HudController extends Module implements OnStart {
	onStart() {
		$print("hud started");
	}
}

/** A derived class whose constructor (with super()) injects another controller. */
@Controller({ loadOrder: 1 })
export class MenuController extends Module {
	constructor(private readonly hud: HudController) {
		super();
	}

	open() {
		this.hud.onStart();
	}
}

export function start() {
	$print(`client sees ${network.serverToClient.size()} server -> client guards`);
	return Reflect.getOwnMetadata<string[]>(MenuController, "typetorch:parameters");
}
