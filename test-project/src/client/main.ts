import { $print } from "rbxts-transform-debug";
import { network } from "../shared/network";

export function start() {
	$print(`client sees ${network.serverToClient.size()} server -> client guards`);
}
