--!nocheck
-- Runtime check of the emitted Luau under Lune (run by scripts/run.ts after the Rojo build, from test-project/):
--   lune run scripts/runtime.luau fixture.rbxm
--
-- Loads the fixture model twice ("generations", like TypeTorch mounts a fresh payload per swap) into one emulated VM:
-- one require cache keyed by ModuleScript and one shared _G, as on a Roblox server. Each generation requires its own
-- copy of the runtime kit, so its Reflect registry must start empty: a registry shared across copies would fail on
-- the second generation's `identifier` ("id ... is already registered"). Then it runs the decorators, DI metadata,
-- generated guards and macros of that generation.
--
-- This is not Roblox: a small Instance emulator (names, children, Source) plus a few service stubs.

local fs = require("@lune/fs")
local luau = require("@lune/luau")
local process = require("@lune/process")
local roblox = require("@lune/roblox")
local task = require("@lune/task")

local passed, failed = 0, 0
local function check(label: string, condition: any, detail: any?)
	if condition then
		passed += 1
		print(`PASS  {label}`)
	else
		failed += 1
		print(`FAIL  {label}{if detail ~= nil then "\n      " .. tostring(detail) else ""}`)
	end
end

-- Instance emulator --------------------------------------------------------------------------------------------------

local Node = {}
local nodeMeta = {}
local inner = setmetatable({}, { __mode = "k" })

local function newNode(className: string, name: string, source: string?)
	local node = setmetatable({}, nodeMeta)
	inner[node] = { ClassName = className, Name = name, Source = source, children = {} }
	return node
end

nodeMeta.__index = function(self, key)
	local data = inner[self]
	if Node[key] then
		return Node[key]
	end
	if key == "ClassName" or key == "Name" or key == "Parent" or key == "Source" then
		return data[key]
	end
	for _, child in data.children do
		if inner[child].Name == key then
			return child
		end
	end
	error(`{key} is not a valid member of {data.ClassName} "{Node.GetFullName(self)}"`, 2)
end
nodeMeta.__tostring = function(self)
	return inner[self].Name
end

function Node.FindFirstChild(self, name: string)
	for _, child in inner[self].children do
		if inner[child].Name == name then
			return child
		end
	end
	return nil
end
function Node.WaitForChild(self, name: string)
	local child = Node.FindFirstChild(self, name)
	if child == nil then
		error(`Infinite yield on {Node.GetFullName(self)}:WaitForChild("{name}")`, 2)
	end
	return child
end
function Node.GetChildren(self)
	return table.clone(inner[self].children)
end
function Node.GetDescendants(self)
	local list = {}
	local function walk(node)
		for _, child in inner[node].children do
			table.insert(list, child)
			walk(child)
		end
	end
	walk(self)
	return list
end
function Node.IsA(self, className: string)
	return className == "Instance" or inner[self].ClassName == className
end
function Node.GetFullName(self)
	local parts = {}
	local node = self
	while node do
		table.insert(parts, 1, inner[node].Name)
		node = inner[node].Parent
	end
	return table.concat(parts, ".")
end
function Node.FindFirstAncestorWhichIsA(self, className: string)
	local node = inner[self].Parent
	while node do
		if Node.IsA(node, className) then
			return node
		end
		node = inner[node].Parent
	end
	return nil
end

local function convert(instance, parent)
	local node = newNode(instance.ClassName, instance.Name, if instance:IsA("LuaSourceContainer") then instance.Source else nil)
	inner[node].Parent = parent
	if parent then
		table.insert(inner[parent].children, node)
	end
	for _, child in instance:GetChildren() do
		convert(child, node)
	end
	return node
end

-- Services and globals ------------------------------------------------------------------------------------------------

local function stubSignal()
	return {
		Connect = function()
			return { Disconnect = function() end, Connected = true }
		end,
		Wait = function() end,
	}
end

local services = {
	RunService = {
		IsRunning = function() return false end,
		IsStudio = function() return false end,
		IsServer = function() return true end,
		IsClient = function() return false end,
		Heartbeat = stubSignal(),
	},
}
local game = {
	GetService = function(_, name: string)
		return services[name] or error(`service {name} is not emulated`)
	end,
	IsLoaded = function() return true end,
}

local G = {} -- the VM's shared _G (roblox-ts keeps per-module runtime pointers in it)
local cache = {} -- ModuleScript -> { value }: require caches per instance for the VM's lifetime, as in Roblox
local loading = {}
local globals = {
	game = game,
	_G = G,
	task = task,
	Enum = roblox.Enum,
	Color3 = roblox.Color3,
	Vector3 = roblox.Vector3,
	CFrame = roblox.CFrame,
	Instance = roblox.Instance,
}

local function requireModule(module)
	assert(type(module) == "table" and inner[module] and module.ClassName == "ModuleScript", `require expects a ModuleScript, got {module}`)
	local hit = cache[module]
	if hit then
		return hit.value
	end
	assert(not loading[module], `cyclic require of {module:GetFullName()}`)
	loading[module] = true
	local environment = table.clone(globals)
	environment.script = module
	environment.require = requireModule
	local chunk = luau.load(module.Source, { debugName = module:GetFullName(), environment = environment })
	local value = chunk()
	loading[module] = nil
	cache[module] = { value = value }
	return value
end

-- Generations ---------------------------------------------------------------------------------------------------------

local path = process.args[1] or "fixture.rbxm"
local bytes = fs.readFile(path)

local function mount(index: number)
	local model = convert(roblox.deserializeModel(bytes)[1], nil)
	inner[model].Name = `Fixture#{index}`
	return model
end

local generations = {}
for index = 1, 2 do
	local model = mount(index)
	local ok, result = pcall(function()
		local services = requireModule(model.Server.services)
		local client = requireModule(model.Client.main)
		local network = requireModule(model.Shared.network).network
		return { checks = services.runChecks(), menuParameters = client.start(), network = network }
	end)
	check(`generation {index}: the server and client modules load`, ok, result)
	if not ok then
		break
	end
	generations[index] = result
	local results = result.checks
	check(`generation {index}: ShopService's dependency ids resolve through Reflect.idToObj`, results.resolved, results.parameters)
	check(
		`generation {index}: 7 classes registered by decorators (fresh registry)`,
		results.registered == 7,
		`registered = {results.registered}`
	)
	check(
		`generation {index}: a Lazy<T> constructor parameter records lazy:<id of T>, a plain one its id`,
		results.teamParameters == "lazy:server/services@ItemService,server/services@DataService",
		results.teamParameters
	)
	check(`generation {index}: guardOf<Payload>() accepts a payload and rejects a bad one`, results.payloadGood and not results.payloadBad)
	check(`generation {index}: createNetwork built 7 leaf guards`, results.guards == 7, `guards = {results.guards}`)
	local buy = result.network.clientToServer["shop.buy"]
	check(
		`generation {index}: the shop.buy guard checks the argument tuple`,
		buy ~= nil and buy({ "sword", 2 }) and not buy({ "sword", "2" }) and not buy({ "sword" })
	)
	check(
		`generation {index}: MenuController's parameters name HudController`,
		result.menuParameters ~= nil and result.menuParameters[1] == "client/main@HudController"
	)
end

if generations[1] and generations[2] then
	local first, second = generations[1].checks.reflect, generations[2].checks.reflect
	check("each generation has its own Reflect", first ~= second and first.idToObj ~= second.idToObj)
	local count = 0
	for _ in first.idToObj do
		count += 1
	end
	check("the first generation's registry is untouched by the second", count == 7, `ids = {count}`)
end

print(if failed == 0 then `\nRuntime: all {passed} checks passed.` else `\nRuntime: {failed} check(s) failed.`)
process.exit(if failed == 0 then 0 else 1)
