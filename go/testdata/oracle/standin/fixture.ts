import { Reflect as Reflect } from "./reflection/reflect";
import { Service } from "./decorators";
/**
 * A module declared inside the package. Its `identifier` metadata is written by the package's own compile; games
 * compute the same id from the `.d.ts` when they inject it.
 */
export class FrameworkLogger {
    log(message: string) {
        print(`[framework] ${message}`);
    }
    static {
        // (TypeTorch) FrameworkLogger metadata
        Reflect["defineMetadata"](FrameworkLogger, "identifier", "@typetorch/framework:fixture@FrameworkLogger");
    }
}
// (TypeTorch) FrameworkLogger decorators
Reflect["decorate"](FrameworkLogger, "@typetorch/framework:decorators@Service", Service, [
    { loadOrder: -1 }
]);
