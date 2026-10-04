import {} from "ts-expose-internals";
import ts from "typescript";
import path from "path";
import { transformFile } from "./transformations/transformFile";
import { TransformerConfig, TransformState } from "./classes/transformState";
import { Logger } from "./classes/logger";
import { f } from "./util/factory";
import { emitTypescriptMismatch } from "./util/functions/emitTypescriptMismatch";

export type { TransformerConfig };

export default function (program: ts.Program, config?: TransformerConfig) {
	return (context: ts.TransformationContext): ((file: ts.SourceFile) => ts.Node) => {
		if (Logger.verbose) Logger.write("\n");
		f.setFactory(context.factory);

		const state = new TransformState(program, context, config ?? {});

		return (file: ts.SourceFile) => {
			if (!ts.isSourceFile(file)) {
				emitTypescriptMismatch(state, "Failed to load! TS version mismatch detected");
			}

			if (state.config.noSemanticDiagnostics !== true) {
				const originalFile = ts.getParseTreeNode(file, ts.isSourceFile);
				if (originalFile) {
					const preEmitDiagnostics = ts.getPreEmitDiagnostics(program, originalFile);
					if (preEmitDiagnostics.some((x) => x.category === ts.DiagnosticCategory.Error)) {
						preEmitDiagnostics
							.filter(ts.isDiagnosticWithLocation)
							.forEach((diag) => context.addDiagnostic(diag));
						return file;
					}
				} else {
					const relativeName = path.relative(state.srcDir, file.fileName);
					Logger.warn(`Failed to validate '${relativeName}' due to lack of parse tree node.`);
				}
			}

			return transformFile(state, file);
		};
	};
}
