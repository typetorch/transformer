// Header comment: stays at the top when the macro import below it is dropped.
import type { GitInfo } from "@typetorch/transformer/macros";
/** Every built-in macro once, so the emitted Luau can be checked (and, for prod, that they are stripped). */
export function debugMacros(value: number) {
    if (value > 100) { }
    const git: Pick<GitInfo, "Branch" | "Commit"> = { "Branch": "main", "Commit": "0123456" };
    const full = { "Branch": "main", "Commit": "0123456", "CommitHash": "0123456789abcdef0123456789abcdef01234567", "LatestTag": "v1.2.3", "ISODate": "2026-03-04T05:06:07.890Z", "Timestamp": 1767323045 };
    const built = 1772600767;
    const iso = "2026-03-04T05:06:07.890Z";
    const doubled = value * 2;
    assert(value > 0, "value must be positive");
    assert(doubled > 0);
    if (value < 0)
        error("negative", 2);
    const name = "GitInfo";
    const field = "Branch";
    const line = 20;
    const where = { "filePath": "src/shared/debug-macros.ts", "lineNumber": 21 };
    const version = "0.0.0";
    return { git, full, built, iso, doubled, name, field, line, where, version };
}
