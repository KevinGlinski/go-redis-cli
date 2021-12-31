-- local cursor = {}, {}, "0";
-- repeat
--     local t = redis.call("SCAN", cursor, "MATCH", "*", "COUNT", 1000000000);
--     for i = 1, #list do
--         local s = list[i];
--         redis.call('EXPIRE', s, 86400, "NX")
--     end;
--     cursor = t[1];
-- until cursor == "0";

local matches = redis.call('KEYS', '*')
local count = 0
for _,key in ipairs(matches) do
    redis.call('EXPIRE', key, 86400)
    count = count + 1
end

return count