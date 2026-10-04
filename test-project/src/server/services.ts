import { FrameworkLogger, Reflect, Service, registered } from "@typetorch/framework";
import { $print } from "rbxts-transform-debug";
import { argNames, describe, guardOf, here, idOf } from "../shared/macros";
import { network, Payload } from "../shared/network";

@Service()
export class DataService {
	get(key: string) {
		return key;
	}
}

/** Two injected constructor dependencies: one from this game, one declared in the framework package. */
@Service({ loadOrder: 10 })
export class ShopService {
	constructor(
		private readonly data: DataService,
		private readonly logger: FrameworkLogger,
	) {}

	buy(player: Player, itemId: string) {
		this.logger.log(`${player.Name} bought ${this.data.get(itemId)}`);
	}
}

export function runChecks() {
	const parameters = Reflect.getOwnMetadata<string[]>(ShopService, "typetorch:parameters") ?? [];
	const loggerId = idOf<FrameworkLogger>();
	const dataId = idOf<DataService>();
	const resolved = parameters.map((id) => Reflect.idToObj.get(id));
	const isPayload = guardOf<Payload>();

	$print(`parameters=[${parameters.join(",")}] loggerId=${loggerId} dataId=${dataId}`);
	$print(`resolved=${resolved[0] === DataService && resolved[1] === FrameworkLogger} registered=${registered.size()}`);
	$print(`payload good=${isPayload({ id: "a" })} bad=${isPayload({ id: 1 })}`);
	$print(`guards=${network.clientToServer.size()}+${network.serverToClient.size()}`);
	$print(describe<Payload>().text, here(), argNames<ShopService["buy"]>().join(","));
}
