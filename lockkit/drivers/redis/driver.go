package redis

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/drivers"
)

const defaultKeyPrefix = "lockman:lease"

var errInvalidScriptResponse = errors.New("redis driver: invalid script response")

// Driver implements the lock driver contract with Redis-backed lease records.
type Driver struct {
	client    goredis.UniversalClient
	keyPrefix string
	now       func() time.Time
}

// NewDriver constructs a Redis-backed lock driver.
func NewDriver(client goredis.UniversalClient, keyPrefix string) *Driver {
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = defaultKeyPrefix
	}

	return &Driver{
		client:    client,
		keyPrefix: keyPrefix,
		now:       time.Now,
	}
}

func (d *Driver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.LeaseRecord{}, err
	}
	if err := validateAcquireRequest(req); err != nil {
		return drivers.LeaseRecord{}, err
	}

	now := d.now()
	resourceKey := req.ResourceKeys[0]
	key := d.buildLeaseKey(req.DefinitionID, resourceKey)

	acquired, err := d.client.SetNX(ctx, key, req.OwnerID, req.LeaseTTL).Result()
	if err != nil {
		return drivers.LeaseRecord{}, err
	}
	if !acquired {
		return drivers.LeaseRecord{}, drivers.ErrLeaseAlreadyHeld
	}

	return drivers.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}, nil
}

func (d *Driver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.LeaseRecord{}, err
	}
	if err := validateRenewLeaseRecord(lease); err != nil {
		return drivers.LeaseRecord{}, err
	}

	resourceKey := lease.ResourceKeys[0]
	key := d.buildLeaseKey(lease.DefinitionID, resourceKey)

	ttlMillis, err := renewScript.Run(ctx, d.client, []string{key}, lease.OwnerID, lease.LeaseTTL.Milliseconds()).Int64()
	if err != nil {
		return drivers.LeaseRecord{}, err
	}

	switch ttlMillis {
	case 0:
		return drivers.LeaseRecord{}, drivers.ErrLeaseNotFound
	case -1:
		return drivers.LeaseRecord{}, drivers.ErrLeaseOwnerMismatch
	case -2:
		return drivers.LeaseRecord{}, drivers.ErrLeaseExpired
	case -3:
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	}
	if ttlMillis < 0 {
		return drivers.LeaseRecord{}, drivers.ErrLeaseExpired
	}

	ttl := time.Duration(ttlMillis) * time.Millisecond
	now := d.now()

	return drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      lease.OwnerID,
		LeaseTTL:     ttl,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(ttl),
	}, nil
}

func (d *Driver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}

	resourceKey := lease.ResourceKeys[0]
	key := d.buildLeaseKey(lease.DefinitionID, resourceKey)

	result, err := releaseScript.Run(ctx, d.client, []string{key}, lease.OwnerID).Int64()
	if err != nil {
		return err
	}

	switch result {
	case 1:
		return nil
	case 0:
		return drivers.ErrLeaseNotFound
	case -1:
		return drivers.ErrLeaseOwnerMismatch
	default:
		return drivers.ErrLeaseNotFound
	}
}

func (d *Driver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.PresenceRecord{}, err
	}
	if err := validatePresenceRequest(req); err != nil {
		return drivers.PresenceRecord{}, err
	}

	resourceKey := req.ResourceKeys[0]
	record := drivers.PresenceRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
	}

	key := d.buildLeaseKey(req.DefinitionID, resourceKey)
	raw, err := presenceScript.Run(ctx, d.client, []string{key}).Result()
	if err != nil {
		return drivers.PresenceRecord{}, err
	}

	present, owner, ttl, err := parsePresenceResult(raw)
	if err != nil {
		return drivers.PresenceRecord{}, err
	}
	if !present {
		return record, nil
	}

	now := d.now()
	record.Present = true
	record.Lease = drivers.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      owner,
		LeaseTTL:     ttl,
		ExpiresAt:    now.Add(ttl),
	}

	return record, nil
}

func (d *Driver) Ping(ctx context.Context) error {
	if err := d.validateClient(); err != nil {
		return err
	}

	return d.client.Ping(ctx).Err()
}

func (d *Driver) buildLeaseKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func encodeSegment(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
}

func parsePresenceResult(raw interface{}) (bool, string, time.Duration, error) {
	values, ok := raw.([]interface{})
	if !ok || len(values) == 0 {
		return false, "", 0, errInvalidScriptResponse
	}

	status, err := toInt64(values[0])
	if err != nil {
		return false, "", 0, err
	}
	if status == 0 {
		return false, "", 0, nil
	}
	if len(values) != 3 {
		return false, "", 0, errInvalidScriptResponse
	}

	owner, err := toString(values[1])
	if err != nil {
		return false, "", 0, err
	}

	ttlMillis, err := toInt64(values[2])
	if err != nil {
		return false, "", 0, err
	}
	if ttlMillis <= 0 {
		return false, "", 0, nil
	}

	return true, owner, time.Duration(ttlMillis) * time.Millisecond, nil
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

func (d *Driver) validateClient() error {
	if d == nil || d.client == nil {
		return drivers.ErrInvalidRequest
	}
	return nil
}

func validateAcquireRequest(req drivers.AcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" {
		return drivers.ErrInvalidRequest
	}
	if len(req.ResourceKeys) != 1 || strings.TrimSpace(req.ResourceKeys[0]) == "" {
		return drivers.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validateLeaseRecord(lease drivers.LeaseRecord) error {
	if strings.TrimSpace(lease.DefinitionID) == "" || strings.TrimSpace(lease.OwnerID) == "" {
		return drivers.ErrInvalidRequest
	}
	if len(lease.ResourceKeys) != 1 || strings.TrimSpace(lease.ResourceKeys[0]) == "" {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validateRenewLeaseRecord(lease drivers.LeaseRecord) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	if lease.LeaseTTL <= 0 {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validatePresenceRequest(req drivers.PresenceRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return drivers.ErrInvalidRequest
	}
	if len(req.ResourceKeys) != 1 || strings.TrimSpace(req.ResourceKeys[0]) == "" {
		return drivers.ErrInvalidRequest
	}

	return nil
}
