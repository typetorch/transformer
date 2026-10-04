# @typetorch/transformer

The roblox-ts transformer behind [TypeTorch](https://github.com/typetorch). It does two things at compile time:

1. **User macros.** A function tagged `@metadata macro` gets its omitted trailing parameters filled in from types:
   runtime `t` guards, stable type ids, call-site info. Nested and mapped types work, so `createNetwork<C2S, S2C>()`
   gets one guard per method of a nested interface.
2. **Class metadata for dependency injection.** Classes decorated with a TypeTorch decorator get an `identifier`, their
   constructor's dependency ids (`typetorch:parameters`) and a `Reflect.decorate` call.

It is a stripped-down fork of [rbxts-transformer-flamework](https://github.com/rbxts-flamework/transformer) (MIT), with
the guard builder and macro engine kept as they are. TypeTorch projects don't depend on Flamework at all: the runtime
(`Reflect`, `Modding`) lives in `@typetorch/framework`, and its source is in [`runtime-kit/`](runtime-kit).

## Usage

```sh
bun add -d @typetorch/transformer typescript@5.5.3 roblox-ts
```

`tsconfig.json` of a game (TypeTorch payloads are roblox-ts Model projects):

```jsonc
{
	"compilerOptions": {
		"experimentalDecorators": true,
		// @typetorch must be a type root so roblox-ts accepts imports from @typetorch/framework.
		"typeRoots": ["node_modules/@rbxts", "node_modules/@typetorch"],
		"plugins": [
			// rbxts-transform-debug stays first in every TypeTorch repo.
			{ "transform": "rbxts-transform-debug", "environmentRequires": {} },
			{ "transform": "@typetorch/transformer" }
		]
	}
}
```

In the Rojo project, map only the runtime package, not the whole scope (the transformer is Node code and would add
empty folders to the model):

```json
"@typetorch": { "$className": "Folder", "framework": { "$path": "node_modules/@typetorch/framework" } }
```

The package ships a dependency-free `out/index.d.ts`, because TypeScript loads every package under a type root as a
type library.

### Options

Passed in the plugin entry: `{ "transform": "@typetorch/transformer", "runtimeModule": "..." }`.

| Option | Default | Meaning |
|---|---|---|
| `runtimeModule` | `"@typetorch/framework"` | Module generated code imports `Reflect` from (and `t`, see below). |
| `runtimeReflectPath` | `"reflection/reflect"` | Only while compiling the runtime package itself: the rootDir-relative file exporting `Reflect`. Generated code imports it relatively, since a package can't import itself by name. |
| `guardModule` | automatic | Module generated guards import `t` from. By default `@rbxts/t` when the file resolves the same copy as the runtime package, otherwise `t` re-exported by `runtimeModule`. |
| `noSemanticDiagnostics` | `false` | Skip TypeScript's semantic checks before transforming each file. Faster, riskier. |
| `optimizations.guardGenerationDedupLimit` | off | Types used this many times inside one guard are hoisted into a local and reused. |

When the transformer resolves a different `typescript` than roblox-ts (for example when it is linked), its
`require("typescript")` is pointed at roblox-ts' copy while it loads, the same trick Flamework uses. Set
`TYPETORCH_TRANSFORMER_HOOK=0` to disable that, or `=1` to force it outside a project folder. `rbxtsc --verbose` makes
the transformer log more.

## User macros

Declare a macro with the JSDoc tag `@metadata macro`. Optional trailing parameters whose types are macro types are
filled in when the caller leaves them out (or passes `undefined`).

```ts
import { Modding } from "@typetorch/framework";

/** @metadata macro */
export function guardOf<T>(guard?: Modding.Generic<T, "guard">) {
	assert(guard);
	return guard;
}

/** @metadata macro */
export function idOf<T>(id?: Modding.Generic<T, "id">) {
	assert(id);
	return id;
}

const isPoint = guardOf<{ x: number; y: number }>(); // guardOf(t.interface({ x = t.number, y = t.number }))
const id = idOf<ShopService>(); // idOf("server/services/shop@ShopService")
```

| Type | Filled with |
|---|---|
| `Modding.Generic<T, "guard">` | a `t` guard for `T` |
| `Modding.Generic<T, "id">` | the stable id of `T` (see [Ids](#ids)) |
| `Modding.Generic<T, "text">` | `T` printed as a string |
| `Modding.GenericMany<T, "id" \| "guard">` | an object with the requested keys |
| `Modding.Caller<"line" \| "character" \| "width" \| "text" \| "uuid">` | info about the call site (`uuid` is random per compile) |
| `Modding.CallerMany<...>` | an object with the requested keys |
| `Modding.TupleLabels<T>` | the labels of tuple `T`, e.g. parameter names via `Parameters<F>` |
| `Modding.Many<X>` | `X` built recursively: objects, tuples, arrays, string/number/boolean literals and `undefined`, with macro types as leaves |

`Modding.Many` follows mapped and recursive types, which is how TypeTorch's networking gets one guard per leaf:

```ts
type GuardTree<T> = {
	[K in keyof T]: T[K] extends (...args: infer A) => unknown ? Modding.Generic<A, "guard"> : GuardTree<T[K]>;
};

/** @metadata macro */
export function createNetwork<C2S extends object, S2C extends object>(
	clientToServer?: Modding.Many<GuardTree<C2S>>,
	serverToClient?: Modding.Many<GuardTree<S2C>>,
) { /* ... */ }

interface ClientToServer {
	shop: { buy(itemId: string, amount: number): void; inventory: { equip(slot: 1 | 2 | 3, itemId?: string): void } };
}
createNetwork<ClientToServer, {}>();
```

compiles to

```lua
local network = createNetwork({
	shop = {
		buy = t.strictArray(t.string, t.number),
		inventory = {
			equip = t.strictArray(t.literalList({ 2, 1, 3 }), t.optional(t.string)),
		},
	},
}, {})
```

A parameter can accept either a value or a macro: `id?: string | Modding.Generic<T, "id">` fills in the id only when
the caller passes nothing.

Macros work across packages: a macro declared in `@typetorch/framework` is recognised from its `.d.ts`, so packages
must keep JSDoc in their declaration output (don't set `removeComments`).

### Guards

The guard builder is Flamework's, unchanged. Highlights:

| Type | Guard |
|---|---|
| `string`, `number`, `boolean`, `undefined`/`void`, `any`, `unknown` | `t.string`, `t.number`, `t.boolean`, `t.none`, `t.any`, `t.union(t.any, t.none)` |
| literals, literal unions, TS `enum`s | `t.literal(...)` / `t.literalList({...})` |
| `T \| undefined`, optional properties | `t.optional(...)` |
| unions, intersections | `t.union` / `t.unionList`, `t.intersection` / `t.intersectionList` |
| tuples (`Parameters<F>`) | `t.strictArray(...)` |
| `T[]`, `Map<K, V>`, `Set<T>`, index signatures | `t.array`, `t.map`, `t.set`, `t.map(key, value)` |
| objects and interfaces | `t.interface({...})` |
| Roblox datatypes (`Vector3`, `CFrame`, `Color3`, ...), `Enum.X`, `buffer` | `t.Vector3`, ..., `t.enum(Enum.X)`, `t.typeof("buffer")` |
| `Instance` types | `t.instanceIsA("BasePart")`, plus `t.children` for declared children |
| functions | `t.callback` |

Not supported (a compile error points at the type): template literal types, classes, and more than one index
signature.

## Class metadata

Decorators made with `Modding.createDecorator` or `Modding.createMetaDecorator` are TypeTorch decorators. The
transformer removes them from the class and emits, for

```ts
/** @metadata typetorch:parameters injectable */
export const Service = Modding.createDecorator<[config?: ModuleConfig]>("Class", (descriptor, [config]) => {
	registered.push({ ctor: descriptor.object, config: config ?? {} });
});

@Service({ loadOrder: 10 })
export class ShopService {
	constructor(private readonly data: DataService, private readonly logger: FrameworkLogger) {}
}
```

this Luau:

```lua
-- inside the class definition
Reflect.defineMetadata(ShopService, "identifier", "server/services@ShopService")
Reflect.defineMetadata(ShopService, "typetorch:parameters", { "server/services@DataService", "@typetorch/framework:logger@FrameworkLogger" })
-- after it
Reflect.decorate(ShopService, "@typetorch/framework:decorators@Service", Service, { { loadOrder = 10 } })
```

`Reflect.decorate` runs the decorator's callback with `{ id, object, constructor, property?, isStatic? }` and the
arguments. A dependency id resolves to its class through `Reflect.idToObj.get(id)` (or `Reflect.getObjectFromId`).

Metadata is requested with JSDoc `@metadata <keys...>` on the class itself, on a decorator's declaration, or on an
interface the class implements. Prefix a key with `~` to opt out of it, `*` asks for everything.

| Key | Written on | Value |
|---|---|---|
| `identifier` | class | its id; always written for decorated classes (registers `Reflect.idToObj`) |
| `typetorch:parameters` | class (constructor), methods | dependency ids of the parameters |
| `typetorch:parameter_names` | class (constructor), methods | parameter names |
| `typetorch:parameter_guards` | class (constructor), methods | a guard per parameter |
| `typetorch:implements` | class | ids of the interfaces in its `implements` clause |
| `typetorch:type` / `typetorch:guard` | properties | the property type's id / guard |
| `typetorch:return_type` / `typetorch:return_guard` | methods | the return type's id / guard |
| `reflect` | class | emit metadata even without a TypeTorch decorator |
| `injectable` | decorator | no effect; documents that the decorator's classes are injected |

`@metadata macro` marks user macros (above). The runtime also writes `typetorch:decorators` (decorator ids per class
or property) and `typetorch:decorators.<id>` (`{ arguments }`). `@metadata {@link SomeType constraint}` on a
decorator makes every class (or member) it decorates fail to compile unless it is assignable to `SomeType`.

## Ids

Ids are deterministic and need no build-info file:

- A declaration in the project being compiled: `<module>@<Name>`, where `<module>` is the output module path relative
  to `outDir` without extension (`index` becomes `init`). `src/server/services.ts` → `server/services@ShopService`.
- When the project is a package (`rbxtsc --type package`, or a scoped package name, the same rule roblox-ts uses), it
  is prefixed with the package name: `@typetorch/framework:net/init@Network`.
- A declaration from another package's `.d.ts` files: `<package>:<module>@<Name>`, with `<module>` relative to the
  directory of the package's `types` entry (its outDir).

So a declaration in a package has the **same id whether the package compiles itself or a game sees its declaration
files**, as long as the package's `types` point into its outDir (the roblox-ts default). Nested declarations include
their namespaces (`Outer.Inner`). Primitives are `$p:string`, `$p:number`, ...; literals are `$ps:<text>` and
`$pn:<number>`.

## Runtime kit

[`runtime-kit/`](runtime-kit) holds the roblox-ts source of the runtime half, to be copied into `@typetorch/framework`
as `src/reflection/` and re-exported from its root:

- `Reflect`: `defineMetadata` (with the special `identifier` key), `getOwnMetadata`, `hasOwnMetadata`, `getMetadata`
  and `getMetadatas` (walk superclasses), `decorate`, the `idToObj` / `objToId` maps, `getObjectFromId`,
  `getIdFromObject` and the `decorators` registry.
- `Modding`: `createDecorator` and `createMetaDecorator` (Class, Method, Property), `getDecoratorArguments`, and the
  macro types `Generic`, `GenericMany`, `Many`, `Caller`, `CallerMany`, `TupleLabels`.
- `t`, re-exported from `@rbxts/t` as the fallback for generated guards.

The macro types are recognised by marker properties that only exist in the type system (`_typetorch_macro_generic`,
`_typetorch_macro_many`, `_typetorch_macro_caller`, `_typetorch_macro_tuple_labels`, `_typetorch_decorator`,
`_typetorch_parameters`). Flamework's `Modding` types are not recognised.

## Migrating from Flamework

| Flamework | TypeTorch |
|---|---|
| `rbxts-transformer-flamework` plugin | `@typetorch/transformer` plugin |
| `import { Modding, Reflect } from "@flamework/core"` | `import { Modding, Reflect } from "@typetorch/framework"` |
| `typeRoots: [..., "node_modules/@flamework"]` | `typeRoots: [..., "node_modules/@typetorch"]` |
| `@metadata flamework:implements flamework:parameters injectable` | `@metadata typetorch:parameters injectable` (add `typetorch:implements` if you read it) |
| `"flamework:parameters"`, `"flamework:implements"`, `"flamework:decorators"` keys | `"typetorch:parameters"`, `"typetorch:implements"`, `"typetorch:decorators"` |
| `Reflect.idToObj` (internal) | `Reflect.idToObj` (public `Map`) or `Reflect.getObjectFromId(id)` |
| `@metadata macro`, `Modding.Generic/Many/Caller/TupleLabels` | unchanged names, TypeTorch's `Modding` |
| `flamework.build`, `flamework.json`, `include/flamework/` | gone; delete them |

Not carried over: `Flamework.addPaths` / `ignite`, `Dependency<T>()` and the other dependency macros, components and
attribute guards, networking intrinsics and key obfuscation, `Modding.Hash` / `Obfuscate` / `Intrinsic`, id
obfuscation modes, `hashPrefix` / `salt`, build-info files, and the `flamework.json` config. Ids changed format
(`flamework.build` is gone), so data saved with Flamework ids does not carry over.

## Developing

```sh
bun install
bun run build    # tsc -> out/
bun run test     # builds, then runs test-project/scripts/run.ts
```

The test fixture compiles a stand-in `@typetorch/framework` (the runtime kit plus a decorated class and a
`createNetwork` macro, built as a package with this transformer) and a Model project that uses it, builds the model
with Rojo, and checks the emitted Luau: a guard per nested leaf, the id strings, `typetorch:parameters`, the `Reflect`
import, and that a package declaration has the same id in both compiles.

## Credits and license

Forked from [rbxts-transformer-flamework](https://github.com/rbxts-flamework/transformer) by Fireboltofdeath and the
Flamework contributors; the runtime kit is adapted from [@flamework/core](https://github.com/rbxts-flamework/core).
Both are MIT licensed. This package is MIT too; see [LICENSE](LICENSE) for both copyright notices.
