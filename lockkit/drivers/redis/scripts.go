package redis

import goredis "github.com/redis/go-redis/v9"

var renewScript = goredis.NewScript(`
local current = redis.call("GET", KEYS[1])
if not current then
	return 0
end

if current ~= ARGV[1] then
	return -1
end

local ttl = tonumber(ARGV[2])
if not ttl or ttl <= 0 then
	return -3
end

if redis.call("PEXPIRE", KEYS[1], ttl) == 0 then
	return -2
end

return ttl
`)

var releaseScript = goredis.NewScript(`
local current = redis.call("GET", KEYS[1])
if not current then
	return 0
end

if current ~= ARGV[1] then
	return -1
end

redis.call("DEL", KEYS[1])
return 1
`)

var presenceScript = goredis.NewScript(`
local current = redis.call("GET", KEYS[1])
if not current then
	return {0}
end

local ttl = redis.call("PTTL", KEYS[1])
if not ttl or ttl <= 0 then
	return {0}
end

return {1, current, ttl}
`)
