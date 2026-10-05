import { Service } from "./decorators";

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
