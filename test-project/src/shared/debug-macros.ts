// Header comment: stays at the top when the macro import below it is dropped.
import { $assert, $compileTime, $dbg, $error, $file, $git, $nameof, $package, $print, $warn, type GitInfo } from "@typetorch/transformer/macros";

/** Every built-in macro once, so the emitted Luau can be checked (and, for prod, that they are stripped). */
export function debugMacros(value: number) {
	$print("print", value);
	$warn("warn", value);
	if (value > 100) $print("never");
	const git: Pick<GitInfo, "Branch" | "Commit"> = $git("Branch", "Commit");
	const full = $git();
	const built = $compileTime();
	const iso = $compileTime("ISO-8601");
	const doubled = $dbg(value * 2);
	$dbg(doubled);
	$assert(value > 0, "value must be positive");
	$assert(doubled > 0);
	if (value < 0) $error("negative", 2);
	const name = $nameof<GitInfo>();
	const field = $nameof(git.Branch);
	const line = $file.lineNumber;
	const where = $file;
	const version = $package.version;
	return { git, full, built, iso, doubled, name, field, line, where, version };
}
