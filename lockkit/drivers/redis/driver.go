package redis

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"lockman/lockkit/drivers"
)

const defaultKeyPrefix = "lockman:lease"

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
	if err := validateLeaseRecord(lease); err != nil {
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

	owner, err := d.client.Get(ctx, key).Result()
	switch {
	case err == nil:
	case err == goredis.Nil:
		return record, nil
	default:
		return drivers.PresenceRecord{}, err
	}

	ttl, err := d.client.PTTL(ctx, key).Result()
	if err != nil {
		return drivers.PresenceRecord{}, err
	}
	if ttl <= 0 {
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

	if err := d.client.Ping(ctx).Err(); err != nil {
		return err
	}
	if err := renewScript.Load(ctx, d.client).Err(); err != nil {
		return err
	}
	if err := releaseScript.Load(ctx, d.client).Err(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) buildLeaseKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func encodeSegment(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
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

func validatePresenceRequest(req drivers.PresenceRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return drivers.ErrInvalidRequest
	}
	if len(req.ResourceKeys) != 1 || strings.TrimSpace(req.ResourceKeys[0]) == "" {
		return drivers.ErrInvalidRequest
	}

	return nil
}
