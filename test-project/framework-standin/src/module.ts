/** Stand-in for framework/src/module.ts: the abstract base game modules extend, and a lifecycle interface. */
export interface OnStart {
	onStart(): void;
}

export abstract class Module {
	protected readonly label!: string;
}
