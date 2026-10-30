/**
 * Compile-time macros of @typetorch/transformer, the successor of rbxts-transform-debug's. Import them from
 * `@typetorch/transformer/macros`; the transformer expands every call and drops the import, so nothing from this
 * package reaches the Luau output.
 *
 * `$print`, `$warn`, `$error`, `$assert` and `$dbg` are the debug macros: they carry the source position
 * (`[src/file.ts:line]`). With `debugMacros: false` in the plugin options, or TYPETORCH_DEBUG_MACROS=0 in the
 * environment (every prod-channel `typetorch build` sets it), they compile to nothing (`$print`, `$warn`, `$dbg`) or to
 * the plain `assert` / `error` call without the position.
 */

/** @typetorchMacro print */
export declare function $print(...params: unknown[]): void;

/** @typetorchMacro warn */
export declare function $warn(...params: unknown[]): void;

/**
 * `error("[src/file.ts:line] " .. message, level)`. Without debug macros: `error(message, level)`.
 * @typetorchMacro error
 */
export declare function $error(message: string, level?: number): never;

/**
 * `assert(condition, "[src/file.ts:line] <condition text>: <message>")` (the message defaults to "Assertion failed!").
 * Without debug macros: `assert(condition, message)`.
 * @typetorchMacro assert
 */
export declare function $assert<T>(condition: T, message?: string): asserts condition;

/**
 * Prints the expression's text and value and evaluates to the value. As a statement, `$dbg(x)` prints and discards.
 * Without debug macros: the expression itself.
 * @typetorchMacro dbg
 */
export declare function $dbg<T>(expression: T): T;

/**
 * The name of a type (`$nameof<Foo>()`) or of an identifier, property or `this` class (`$nameof(value)`).
 * @typetorchMacro nameof
 */
export declare function $nameof<T>(): string;
export declare function $nameof(value: unknown): string;

export interface FileInfo {
	readonly filePath: string;
	readonly lineNumber: number;
}
/**
 * The current file: `$file.filePath` (relative to the project, forward slashes) and `$file.lineNumber`.
 * @typetorchMacro file
 */
export declare const $file: FileInfo;

export interface PackageJsonInfo {
	readonly name: string;
	readonly version: string;
	readonly [field: string]: unknown;
}
/**
 * The project's package.json (`$package.version`, `$package.name`, ...).
 * @typetorchMacro package
 */
export declare const $package: PackageJsonInfo;

export interface GitInfo {
	/** The branch name ("" when detached or not a git repository). */
	readonly Branch: string;
	/** The first 7 characters of the commit hash. */
	readonly Commit: string;
	/** The full commit hash. */
	readonly CommitHash: string;
	/** The latest tag reachable from the commit ("" without one). */
	readonly LatestTag: string;
	/**
	 * The commit's author date (ISO 8601) while the commit's object is loose. Once it is packed (a fresh clone, or after
	 * `git gc`) this is the compile time, as with rbxts-transform-debug. `Timestamp` is always the author date.
	 */
	readonly ISODate: string;
	/** The commit's author date as a unix timestamp (seconds). */
	readonly Timestamp: number;
}
/** @deprecated The rbxts-transform-debug name; use `GitInfo`. */
export type $git = GitInfo;

/**
 * Git information of the project at compile time, as an object literal. With keys, only those fields.
 * @typetorchMacro git
 */
export declare function $git(): GitInfo;
export declare function $git<K extends keyof GitInfo>(...keys: K[]): Pick<GitInfo, K>;

export interface CompileTimeKind {
	DateTime: DateTime;
	UnixTimestamp: number;
	UnixTimestampMillis: number;
	["ISO-8601"]: string;
}

/**
 * The time of the compile: a unix timestamp in seconds by default, or in the given form.
 * @typetorchMacro compileTime
 */
export declare function $compileTime(): number;
export declare function $compileTime<K extends keyof CompileTimeKind>(outputAs: K): CompileTimeKind[K];
