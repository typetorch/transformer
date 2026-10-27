import ts from "typescript";
import { TransformState } from "../classes/transformState";
import { getNodeList } from "../util/functions/getNodeList";
import { transformStatement } from "./transformStatement";

export function transformStatementList(state: TransformState, statements: ReadonlyArray<ts.Statement>) {
	const result = new Array<ts.Statement>();
	// Statements that compiled to nothing (a dropped macro import, a stripped `$print`): their comments, which may
	// be the file's header or a directive, move to the next statement that is emitted.
	let dropped = new Array<ts.Statement>();

	for (const statement of statements) {
		const [newStatements, prereqs] = state.capture(() => transformStatement(state, statement));
		const produced = [...prereqs, ...getNodeList(newStatements)];

		if (produced.length === 0) {
			dropped.push(statement);
			continue;
		}

		for (const node of dropped) {
			ts.copyComments(node, produced[0]);
		}
		dropped = [];
		result.push(...produced);
	}

	return result;
}
