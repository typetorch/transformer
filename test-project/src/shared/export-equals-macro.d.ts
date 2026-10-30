export interface EqualsPrintOptions {
	verbose: boolean;
}

/** @typetorchMacro print */
declare const $equalsPrint: (...args: unknown[]) => void;

// @ts-ignore TS2309: packages such as @rbxts/quickzone do this, with skipLibCheck on.
export = $equalsPrint;
