import { Modding } from "@typetorch/framework";
/**
 * Macros declared in the game itself (the other case is createNetwork, declared in a package).
 */
/** @metadata macro */
export function idOf<T>(id?: Modding.Generic<T, "id">): string {
    assert(id, "[src/shared/macros.ts:10] id: id macro was not filled in by the transformer");
    return id;
}
/** @metadata macro */
export function guardOf<T>(guard?: Modding.Generic<T, "guard">): (value: unknown) => value is T {
    assert(guard, "[src/shared/macros.ts:16] guard: guard macro was not filled in by the transformer");
    return guard;
}
/** @metadata macro */
export function describe<T>(info?: Modding.GenericMany<T, "id" | "text">): {
    id: string;
    text: string;
} {
    assert(info, "[src/shared/macros.ts:22] info: describe macro was not filled in by the transformer");
    return info;
}
/** @metadata macro */
export function here(line?: Modding.Caller<"line">, text?: Modding.Caller<"text">): string {
    return `${text} @ line ${line}`;
}
/** @metadata macro */
export function argNames<F extends (...args: never[]) => unknown>(names?: Modding.TupleLabels<Parameters<F>>) {
    return names ?? [];
}
