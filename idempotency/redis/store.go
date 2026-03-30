package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/idempotency"
)

// New creates a Redis idempotency store compatible with lockman.WithIdempotency(...).
// Passing an empty keyPrefix uses lockman's default Redis idempotency namespace.
func New(client goredis.UniversalClient, keyPrefix string) idempotency.Store {
	return NewStore(client, keyPrefix)
}

const defaultKeyPrefix = "lockman:idempotency"

var (
	errInvalidRequest        = errors.New("redis idempotency store: invalid request")
	errInvalidScriptResponse = errors.New("redis idempotency store: invalid script response")
)

var beginScript = goredis.NewScript(`
local existing = redis.call("GET", KEYS[1])
if existing then
	return {0, existing}
end

local record = cjson.encode({
	key = ARGV[1],
	status = ARGV[2],
	owner_id = ARGV[3],
	message_id = ARGV[4],
	consumer_group = ARGV[5],
	attempt = tonumber(ARGV[6]),
	updated_at_unix_ms = tonumber(ARGV[7]),
	expires_at_unix_ms = tonumber(ARGV[8])
})

redis.call("PSETEX", KEYS[1], tonumber(ARGV[9]), record)
return {1, record}
`)

var terminalScript = goredis.NewScript(`
local existingRaw = redis.call("GET", KEYS[1])
local owner = ARGV[3]
local message = ARGV[4]
local consumer_group = ""
local attempt = 0

if existingRaw then
	local existing = cjson.decode(existingRaw)
	owner = existing.owner_id or owner
	message = existing.message_id or message
	consumer_group = existing.consumer_group or consumer_group
	attempt = tonumber(existing.attempt) or attempt
end

local record = cjson.encode({
	key = ARGV[1],
	status = ARGV[2],
	owner_id = owner,
	message_id = message,
	consumer_group = consumer_group,
	attempt = attempt,
	updated_at_unix_ms = tonumber(ARGV[5]),
	expires_at_unix_ms = tonumber(ARGV[6])
})

redis.call("PSETEX", KEYS[1], tonumber(ARGV[7]), record)
return record
`)

type Store struct {
	client    goredis.UniversalClient
	keyPrefix string
	now       func() time.Time
}

func NewStore(client goredis.UniversalClient, keyPrefix string) *Store {
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = defaultKeyPrefix
	}

	return &Store{
		client:    client,
		keyPrefix: keyPrefix,
		now:       time.Now,
	}
}

func (s *Store) Get(ctx context.Context, key string) (idempotency.Record, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Record{}, err
	}
	if err := s.validateKeyedRequest(key); err != nil {
		return idempotency.Record{}, err
	}

	raw, err := s.client.Get(ctx, s.buildKey(key)).Result()
	if errors.Is(err, goredis.Nil) {
		return idempotency.Record{
			Key:    key,
			Status: idempotency.StatusMissing,
		}, nil
	}
	if err != nil {
		return idempotency.Record{}, err
	}

	record, err := decodeRecord(raw)
	if err != nil {
		return idempotency.Record{}, err
	}

	return record, nil
}

func (s *Store) Begin(ctx context.Context, key string, input idempotency.BeginInput) (idempotency.BeginResult, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.BeginResult{}, err
	}
	if err := s.validateKeyedRequest(key); err != nil {
		return idempotency.BeginResult{}, err
	}

	ttlMillis, err := durationToMillis(input.TTL)
	if err != nil {
		return idempotency.BeginResult{}, err
	}

	now := s.now()
	nowMillis := now.UnixMilli()
	expiresAtMillis := now.Add(input.TTL).UnixMilli()

	raw, err := beginScript.Run(
		ctx,
		s.client,
		[]string{s.buildKey(key)},
		key,
		string(idempotency.StatusInProgress),
		input.OwnerID,
		input.MessageID,
		input.ConsumerGroup,
		strconv.Itoa(input.Attempt),
		strconv.FormatInt(nowMillis, 10),
		strconv.FormatInt(expiresAtMillis, 10),
		strconv.FormatInt(ttlMillis, 10),
	).Result()
	if err != nil {
		return idempotency.BeginResult{}, err
	}

	acquired, record, err := parseBeginResponse(raw)
	if err != nil {
		return idempotency.BeginResult{}, err
	}

	return idempotency.BeginResult{
		Record:    record,
		Acquired:  acquired,
		Duplicate: !acquired,
	}, nil
}

func (s *Store) Complete(ctx context.Context, key string, input idempotency.CompleteInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, idempotency.StatusCompleted)
}

func (s *Store) Fail(ctx context.Context, key string, input idempotency.FailInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, idempotency.StatusFailed)
}

func (s *Store) setTerminalStatus(ctx context.Context, key, ownerID, messageID string, ttl time.Duration, status idempotency.Status) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateKeyedRequest(key); err != nil {
		return err
	}

	ttlMillis, err := durationToMillis(ttl)
	if err != nil {
		return err
	}

	now := s.now()
	nowMillis := now.UnixMilli()
	expiresAtMillis := now.Add(ttl).UnixMilli()

	_, err = terminalScript.Run(
		ctx,
		s.client,
		[]string{s.buildKey(key)},
		key,
		string(status),
		ownerID,
		messageID,
		strconv.FormatInt(nowMillis, 10),
		strconv.FormatInt(expiresAtMillis, 10),
		strconv.FormatInt(ttlMillis, 10),
	).Result()
	return err
}

func (s *Store) buildKey(key string) string {
	return fmt.Sprintf("%s:%s", s.keyPrefix, encodeSegment(key))
}

func encodeSegment(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
}

func (s *Store) validateKeyedRequest(key string) error {
	if s == nil || s.client == nil {
		return errInvalidRequest
	}
	if strings.TrimSpace(key) == "" {
		return errInvalidRequest
	}
	return nil
}

func durationToMillis(ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, errInvalidRequest
	}

	millis := ttl / time.Millisecond
	if ttl%time.Millisecond != 0 {
		millis++
	}
	if millis <= 0 {
		return 0, errInvalidRequest
	}
	return int64(millis), nil
}

func parseBeginResponse(raw interface{}) (bool, idempotency.Record, error) {
	values, ok := raw.([]interface{})
	if !ok || len(values) != 2 {
		return false, idempotency.Record{}, errInvalidScriptResponse
	}

	code, err := toInt64(values[0])
	if err != nil {
		return false, idempotency.Record{}, err
	}

	recordRaw, err := toString(values[1])
	if err != nil {
		return false, idempotency.Record{}, err
	}

	record, err := decodeRecord(recordRaw)
	if err != nil {
		return false, idempotency.Record{}, err
	}

	switch code {
	case 1:
		return true, record, nil
	case 0:
		return false, record, nil
	default:
		return false, idempotency.Record{}, errInvalidScriptResponse
	}
}

func toInt64(v interface{}) (int64, error) {
	switch value := v.(type) {
	case int64:
		return value, nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, errInvalidScriptResponse
		}
		return parsed, nil
	default:
		return 0, errInvalidScriptResponse
	}
}

func toString(v interface{}) (string, error) {
	switch value := v.(type) {
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	default:
		return "", errInvalidScriptResponse
	}
}

type recordPayload struct {
	Key             string `json:"key"`
	Status          string `json:"status"`
	OwnerID         string `json:"owner_id"`
	MessageID       string `json:"message_id"`
	ConsumerGroup   string `json:"consumer_group"`
	Attempt         int    `json:"attempt"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms"`
	ExpiresAtUnixMS int64  `json:"expires_at_unix_ms"`
}

func decodeRecord(raw string) (idempotency.Record, error) {
	var payload recordPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return idempotency.Record{}, errInvalidScriptResponse
	}

	return idempotency.Record{
		Key:           payload.Key,
		Status:        idempotency.Status(payload.Status),
		OwnerID:       payload.OwnerID,
		MessageID:     payload.MessageID,
		ConsumerGroup: payload.ConsumerGroup,
		Attempt:       payload.Attempt,
		UpdatedAt:     time.UnixMilli(payload.UpdatedAtUnixMS),
		ExpiresAt:     time.UnixMilli(payload.ExpiresAtUnixMS),
	}, nil
}

var _ idempotency.Store = (*Store)(nil)
