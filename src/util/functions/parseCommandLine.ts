import ts from "typescript";
import path from "path";
import fs from "fs";

export interface CommandLine {
	tsconfigPath: string;
	project: string;
	/** The roblox-ts `--type` option, if it was passed. */
	type?: string;
}

function findTsConfigPath(projectPath: string) {
	let tsConfigPath: string | undefined = path.resolve(projectPath);
	if (!fs.existsSync(tsConfigPath) || !fs.statSync(tsConfigPath).isFile()) {
		tsConfigPath = ts.findConfigFile(tsConfigPath, ts.sys.fileExists);
		if (tsConfigPath === undefined) {
			throw new Error("Unable to find tsconfig.json!");
		}
	}
	return path.resolve(process.cwd(), tsConfigPath);
}

function getOption(names: string[]): string | undefined {
	for (let i = 0; i < process.argv.length; i++) {
		const arg = process.argv[i];
		for (const name of names) {
			if (arg === name) return process.argv[i + 1];
			if (arg.startsWith(`${name}=`)) return arg.slice(name.length + 1);
		}
	}
}

export function parseCommandLine(): CommandLine {
	const project = getOption(["-p", "--project"]);
	const tsconfigPath = findTsConfigPath(project ?? ".");

	return {
		tsconfigPath,
		project: path.dirname(tsconfigPath),
		type: getOption(["--type"]),
	};
}
