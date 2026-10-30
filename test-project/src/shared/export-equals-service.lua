local EqualsService = {}
EqualsService.__index = EqualsService
function EqualsService:get(key)
	return key
end
return EqualsService
