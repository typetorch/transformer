import { Service } from "@typetorch/framework";
import $equalsPrint from "./export-equals-macro";
import EqualsService from "./export-equals-service";

/**
 * Modules with `export =` next to exported types. TypeScript 7 flags their `export=` symbol as a namespace, and the
 * checker's SkipAlias stops at it: `$equalsPrint` wasn't a macro any more, and its import was still dropped.
 */
export function equalsMacro(value: number) {
	$equalsPrint("print", value);
}

/** The injected id is the class's, not the `export =` statement's. */
@Service()
export class EqualsConsumer {
	constructor(private readonly service: EqualsService) {}

	read(key: string) {
		return this.service.get(key);
	}
}
