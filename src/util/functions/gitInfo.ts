import { execFileSync } from "child_process";
import { existsSync } from "fs";
import { resolve } from "path";

/** What `$git()` compiles in. Empty strings (and the current time) when the project is not a git repository. */
export interface GitInfo {
	branch: string;
	commit: string;
	latestTag: string;
	isoDate: string;
	timestamp: number;
}

function git(cwd: string, args: string[]): string | undefined {
	try {
		return execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], timeout: 10_000 }).trim();
	} catch {
		return undefined;
	}
}

/**
 * Whether the commit's object is a loose file, not in a pack. rbxts-transform-debug read git with git-repo-info, which
 * only reads loose objects: its ISODate is the author date for a loose commit, and the compile time for a packed one
 * (a fresh clone, or after `git gc`). Games moving off it keep the value they had.
 */
function isLooseCommit(cwd: string, commit: string): boolean {
	const objectPath = git(cwd, ["rev-parse", "--git-path", `objects/${commit.slice(0, 2)}/${commit.slice(2)}`]);
	return objectPath !== undefined && existsSync(resolve(cwd, objectPath));
}

/** Reads the repository `cwd` is in with the git CLI (four calls; the caller caches the result per compile). */
export function readGitInfo(cwd: string): GitInfo {
	const head = git(cwd, ["log", "-1", "--format=%H%n%aI%n%at"])?.split("\n") ?? [];
	const commit = /^[0-9a-f]{40}$/.test(head[0] ?? "") ? head[0] : "";
	const now = new Date();
	const timestamp = commit && /^\d+$/.test(head[2] ?? "") ? Number(head[2]) : Math.round(now.getTime() / 1000);
	const isoDate = commit && head[1] && isLooseCommit(cwd, commit) ? new Date(head[1]).toISOString() : now.toISOString();
	// A detached HEAD prints "HEAD"; rbxts-transform-debug compiled "" in for it.
	const branch = commit ? git(cwd, ["rev-parse", "--abbrev-ref", "HEAD"]) ?? "" : "";
	return {
		branch: branch === "HEAD" ? "" : branch,
		commit,
		latestTag: commit ? git(cwd, ["describe", "--tags", "--abbrev=0"]) ?? "" : "",
		isoDate,
		timestamp,
	};
}
