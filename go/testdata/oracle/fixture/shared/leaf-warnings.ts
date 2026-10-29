import { t as t } from "@rbxts/t";
import { createNetwork } from "@typetorch/framework";
/**
 * Leaves whose guards compile but can't do their job: the transformer warns and names each leaf (scripts/run.ts checks
 * the warnings). The network itself still works.
 */
interface ClientToServer {
    carry: {
        /** Generic and conditional: the guard checks T as Model | undefined and each conditional as either branch. */
        notify: <T extends Model | undefined>(box: T, group: T extends Model ? string : undefined) => void;
    };
    patches: {
        /** AnimationTracks don't replicate: they arrive as nil. */
        looped: (track: AnimationTrack, looped: boolean) => void;
        /** Functions, threads and connections can't be sent; nested ones too. */
        later: (done: () => void, options: {
            co: thread;
            conn: RBXScriptConnection;
        }) => void;
    };
    /** Fine: no warning. */
    fine: (part: BasePart, at: Vector3, tags: string[]) => void;
}
export const leafWarnings = createNetwork<ClientToServer, {}>({ "carry": { "notify": t["strictArray"](t["optional"](t["instanceIsA"]("Model")), t["optional"](t["string"])) as never } as never, "patches": { "looped": t["strictArray"](t["instanceIsA"]("AnimationTrack"), t["boolean"]) as never, "later": t["strictArray"](t["callback"], t["interface"]({
            "co": t["interface"]({}),
            "conn": t["RBXScriptConnection"]
        })) as never } as never, "fine": t["strictArray"](t["instanceIsA"]("BasePart"), t["Vector3"], t["array"](t["string"])) as never } as never, {} as never);
