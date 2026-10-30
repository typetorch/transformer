export interface EqualsServiceOptions {
	verbose: boolean;
}

declare class EqualsService {
	get(key: string): string;
}

// @ts-ignore TS2309: packages such as @rbxts/quickzone do this, with skipLibCheck on.
export = EqualsService;
