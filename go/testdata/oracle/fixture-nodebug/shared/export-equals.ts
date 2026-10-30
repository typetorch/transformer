import { Reflect as Reflect } from "@typetorch/framework";
import { Service } from "@typetorch/framework";
import EqualsService from "./export-equals-service";
/**
 * Modules with `export =` next to exported types. TypeScript 7 flags their `export=` symbol as a namespace, and the
 * checker's SkipAlias stops at it: `$equalsPrint` wasn't a macro any more, and its import was still dropped.
 */
export function equalsMacro(value: number) {
}
/** The injected id is the class's, not the `export =` statement's. */
export class EqualsConsumer {
    constructor(private readonly service: EqualsService) { }
    read(key: string) {
        return this.service.get(key);
    }
    static {
        // (TypeTorch) EqualsConsumer metadata
        Reflect["defineMetadata"](EqualsConsumer, "identifier", "shared/export-equals@EqualsConsumer");
        Reflect["defineMetadata"](EqualsConsumer, "typetorch:parameters", [
            "shared/export-equals-service@EqualsService"
        ]);
    }
}
// (TypeTorch) EqualsConsumer decorators
Reflect["decorate"](EqualsConsumer, "@typetorch/framework:decorators@Service", Service, []);
