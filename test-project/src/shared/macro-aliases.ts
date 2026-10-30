/**
 * Macros called through a name that isn't their declaration's. These calls have to find their macro although a callee
 * name usually says it can't be one.
 */
import { $nameof as nameOfValue, $print as log } from "@typetorch/transformer/macros";
import type { Modding } from "@typetorch/framework";
import defaultHere, { macroNamespace, renamedHere } from "./macro-reexports";
import * as macros from "./macros";
import { here as whereAmI, idOf as idOfAlias } from "./macros";

export const viaImportAlias = whereAmI();
export const viaImportAliasGeneric = idOfAlias<string>();
export const viaExportAlias = renamedHere();
export const viaDefaultImport = defaultHere();
export const viaNamespace = macros.here();
export const viaExportedNamespace = macroNamespace.here();
export const viaElementAccess = macros["here"]();
export const viaParens = (whereAmI)();
log("aliased debug macro");
export const builtinAlias = nameOfValue(viaParens);

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
export const viaImplements = new ImplementsMacro();

interface MethodMacro {
	/** @metadata macro */
	lineOf(line?: Modding.Caller<"line">): number | undefined;
}

export class ImplementsMethodMacro implements MethodMacro {
	public lineOf(line?: Modding.Caller<"line">) {
		return line;
	}
}
export const viaImplementedMethod = new ImplementsMethodMacro().lineOf();

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
export const viaDecorator = new DecoratedMacro();

// not macros: a variable holding a macro isn't one, and neither is any other call
const holdsMacro = whereAmI;
export const notViaVariable = holdsMacro();
export function plainFunction(line?: Modding.Caller<"line">) {
	return line;
}
export const notAMacro = plainFunction();
