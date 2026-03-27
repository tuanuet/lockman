package redis

import goredis "github.com/redis/go-redis/v9"

var lineageAcquireScript = goredis.NewScript(`
local ttl = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local kind = ARGV[4]
local member = ARGV[5]
local ancestor_count = tonumber(ARGV[6]) or 0

if not ttl or ttl <= 0 or not now or member == "" then
	return {-3, 0}
end

local function prune_and_get_remaining(key)
	redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
	local latest = redis.call("ZREVRANGE", key, 0, 0, "WITHSCORES")
	if latest and #latest >= 2 then
		local expiry = tonumber(latest[2])
		if expiry and expiry > now then
			return expiry - now
		end
	end
	return 0
end

redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now)
if redis.call("ZCARD", KEYS[2]) > 0 then
	return {-2, 0}
end

if redis.call("EXISTS", KEYS[1]) == 1 then
	return {-1, 0}
end

if kind == "child" then
	for i = 1, ancestor_count do
		if redis.call("EXISTS", KEYS[2 + i]) == 1 then
			return {-2, 0}
		end
	end
end

local set_result = redis.call("SET", KEYS[1], ARGV[1], "PX", ttl, "NX")
if not set_result then
	return {-1, 0}
end

local expiry = now + ttl
for i = 1, ancestor_count do
	local lineage_key = KEYS[2 + ancestor_count + i]
	redis.call("ZADD", lineage_key, expiry, member)
	local remaining = prune_and_get_remaining(lineage_key)
	if remaining <= 0 then
		redis.call("DEL", lineage_key)
	else
		local existing_ttl = redis.call("PTTL", lineage_key)
		if not existing_ttl or existing_ttl < remaining then
			redis.call("PEXPIRE", lineage_key, remaining)
		end
	end
end

return {1, ttl}
`)

var lineageRenewScript = goredis.NewScript(`
local ttl = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local member = ARGV[4]
local ancestor_count = tonumber(ARGV[5]) or 0

if not ttl or ttl <= 0 or not now or member == "" then
	return {-3, 0}
end

local current = redis.call("GET", KEYS[1])
if not current then
	return {0, 0}
end
if current ~= ARGV[1] then
	return {-1, 0}
end

local function prune_and_get_remaining(key)
	redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
	local latest = redis.call("ZREVRANGE", key, 0, 0, "WITHSCORES")
	if latest and #latest >= 2 then
		local expiry = tonumber(latest[2])
		if expiry and expiry > now then
			return expiry - now
		end
	end
	return 0
end

local expiry = now + ttl
for i = 1, ancestor_count do
	local lineage_key = KEYS[1 + i]
	if not redis.call("ZSCORE", lineage_key, member) then
		return {-4, 0}
	end
end

if redis.call("PEXPIRE", KEYS[1], ttl) == 0 then
	return {-2, 0}
end

for i = 1, ancestor_count do
	local lineage_key = KEYS[1 + i]
	redis.call("ZADD", lineage_key, "XX", expiry, member)
	local remaining = prune_and_get_remaining(lineage_key)
	if remaining <= 0 then
		redis.call("DEL", lineage_key)
	else
		local existing_ttl = redis.call("PTTL", lineage_key)
		if not existing_ttl or existing_ttl < remaining then
			redis.call("PEXPIRE", lineage_key, remaining)
		end
	end
end

return {1, ttl}
`)

var lineageReleaseScript = goredis.NewScript(`
local now = tonumber(ARGV[2])
local member = ARGV[3]
local ancestor_count = tonumber(ARGV[4]) or 0

if not now or member == "" then
	return -2
end

local current = redis.call("GET", KEYS[1])
if not current then
	return 0
end
if current ~= ARGV[1] then
	return -1
end

redis.call("DEL", KEYS[1])

for i = 1, ancestor_count do
	local lineage_key = KEYS[1 + i]
	redis.call("ZREM", lineage_key, member)
	redis.call("ZREMRANGEBYSCORE", lineage_key, "-inf", now)
	if redis.call("ZCARD", lineage_key) == 0 then
		redis.call("DEL", lineage_key)
	else
		local latest = redis.call("ZREVRANGE", lineage_key, 0, 0, "WITHSCORES")
		if latest and #latest >= 2 then
			local expiry = tonumber(latest[2])
			if expiry and expiry > now then
				redis.call("PEXPIRE", lineage_key, expiry - now)
			else
				redis.call("DEL", lineage_key)
			end
		else
			redis.call("DEL", lineage_key)
		end
	end
end

return 1
`)

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
