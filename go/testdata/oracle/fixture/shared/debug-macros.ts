// Header comment: stays at the top when the macro import below it is dropped.
import type { GitInfo } from "@typetorch/transformer/macros";
/** Every built-in macro once, so the emitted Luau can be checked (and, for prod, that they are stripped). */
export function debugMacros(value: number) {
    print("[src/shared/debug-macros.ts:6]", "print", value);
    warn("[src/shared/debug-macros.ts:7]", "warn", value);
    if (value > 100)
        print("[src/shared/debug-macros.ts:8]", "never");
    const git: Pick<GitInfo, "Branch" | "Commit"> = { "Branch": "main", "Commit": "0123456" };
    const full = { "Branch": "main", "Commit": "0123456", "CommitHash": "0123456789abcdef0123456789abcdef01234567", "LatestTag": "v1.2.3", "ISODate": "2026-01-02T03:04:05.000Z", "Timestamp": 1767323045 };
    const built = 1772600767;
    const iso = "2026-03-04T05:06:07.890Z";
    const doubled = (() => {
        const value_1 = value * 2;
        print("[src/shared/debug-macros.ts:13] value * 2 =", value_1);
        return value_1;
    })();
    print("[src/shared/debug-macros.ts:14] doubled =", doubled);
    assert(value > 0, "[src/shared/debug-macros.ts:15] value > 0: value must be positive");
    assert(doubled > 0, "[src/shared/debug-macros.ts:16] doubled > 0: Assertion failed!");
    if (value < 0)
        error("[src/shared/debug-macros.ts:17] negative", 2);
    const name = "GitInfo";
    const field = "Branch";
    const line = 20;
    const where = { "filePath": "src/shared/debug-macros.ts", "lineNumber": 21 };
    const version = "0.0.0";
    return { git, full, built, iso, doubled, name, field, line, where, version };
}
