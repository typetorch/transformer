/**
 * Macros called through a name that isn't their declaration's. These calls have to find their macro although a callee
 * name usually says it can't be one.
 */
import type { Modding } from "@typetorch/framework";
import defaultHere, { macroNamespace, renamedHere } from "./macro-reexports";
import * as macros from "./macros";
import { here as whereAmI, idOf as idOfAlias } from "./macros";
export const viaImportAlias = whereAmI(11 as never, "whereAmI()" as never);
export const viaImportAliasGeneric = idOfAlias<string>("$p:string" as never);
export const viaExportAlias = renamedHere(13 as never, "renamedHere()" as never);
export const viaDefaultImport = defaultHere(14 as never, "defaultHere()" as never);
export const viaNamespace = macros.here(15 as never, "macros.here()" as never);
export const viaExportedNamespace = macroNamespace.here(16 as never, "macroNamespace.here()" as never);
export const viaElementAccess = macros["here"]();
export const viaParens = (whereAmI)();
print("[src/shared/macro-aliases.ts:19]", "aliased debug macro");
export const builtinAlias = "viaParens";
/** @metadata macro */
interface LineMacro {
    line?: number;
}
/** A class is a macro through the interface it implements, under its own name. */
export class ImplementsMacro implements LineMacro {
    public line?: number;
    public constructor(line?: Modding.Caller<"line">) {
        this.line = line;
    }
}
export const viaImplements = new ImplementsMacro(35 as never);
interface MethodMacro {
    /** @metadata macro */
    lineOf(line?: Modding.Caller<"line">): number | undefined;
}
export class ImplementsMethodMacro implements MethodMacro {
    public lineOf(line?: Modding.Caller<"line">) {
        return line;
    }
}
export const viaImplementedMethod = new ImplementsMethodMacro().lineOf(47 as never);
/** @metadata macro */
function MacroDecorator<T>(value: T) {
    return value;
}
@MacroDecorator
export class DecoratedMacro {
    public line?: number;
    public constructor(line?: Modding.Caller<"line">) {
        this.line = line;
    }
}
export const viaDecorator = new DecoratedMacro(62 as never);
// not macros: a variable holding a macro isn't one, and neither is any other call
const holdsMacro = whereAmI;
export const notViaVariable = holdsMacro();
export function plainFunction(line?: Modding.Caller<"line">) {
    return line;
}
export const notAMacro = plainFunction();
