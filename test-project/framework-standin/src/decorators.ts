import { Modding } from "./reflection/modding";
import { ModuleConfig, registered } from "./registry";

/**
 * Stand-in for framework/src/decorators.ts, in its own module like the real one: games see the decorator id
 * `@typetorch/framework:decorators@Service`. The JSDoc tag asks for the constructor's dependency ids.
 *
 * @metadata typetorch:parameters injectable
 */
export const Service = Modding.createDecorator<[config?: ModuleConfig]>("Class", (descriptor, [config]) => {
	registered.push({ ctor: descriptor.object, realm: "server", config: config ?? {} });
});

/**
 * A client module. Same contract as @Service.
 *
 * @metadata typetorch:parameters injectable
 */
export const Controller = Modding.createDecorator<[config?: ModuleConfig]>("Class", (descriptor, [config]) => {
	registered.push({ ctor: descriptor.object, realm: "client", config: config ?? {} });
});
