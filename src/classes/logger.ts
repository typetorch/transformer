const COLORS = {
	red: 31,
	yellow: 33,
	blue: 34,
	gray: 90,
} as const;

type Color = keyof typeof COLORS;

const useColor = process.stdout.isTTY === true && process.env.NO_COLOR === undefined;

function paint(color: Color, text: string) {
	return useColor ? `\x1b[${COLORS[color]}m${text}\x1b[39m` : text;
}

export class Logger {
	public static verbose = process.argv.includes("--verbose");

	static write(message: string) {
		process.stdout.write(message);
	}

	static writeLine(...messages: Array<unknown>) {
		for (const message of messages) {
			const text = typeof message === "string" ? `${message}` : `${JSON.stringify(message, undefined, "\t")}`;

			const prefix = `[${paint("gray", "TypeTorch")}]: `;
			this.write(`${prefix}${text.replace(/\n/g, `\n${prefix}`)}\n`);
		}
	}

	static info(...messages: Array<unknown>) {
		this.writeLine(...messages.map((x) => paint("blue", `${x}`)));
	}

	static infoIfVerbose(...messages: Array<unknown>) {
		if (this.verbose) return this.info(...messages);
	}

	static warn(...messages: Array<unknown>) {
		this.writeLine(...messages.map((x) => paint("yellow", `${x}`)));
	}

	static error(...messages: Array<unknown>) {
		this.writeLine(...messages.map((x) => paint("red", `${x}`)));
	}
}

export function green(text: string) {
	return useColor ? `\x1b[32m${text}\x1b[39m` : text;
}
