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

	"github.com/tuanuet/lockman/backend"
)

const defaultKeyPrefix = "lockman:lease"

var errInvalidScriptResponse = errors.New("redis driver: invalid script response")

// Driver implements the lock driver contract with Redis-backed lease records.
type Driver struct {
	client                 goredis.UniversalClient
	keyPrefix              string
	encodedIDs             map[string]string // definitionID → base64-encoded segment
	leaseKeyPrefixes       map[string]string
	strictFenceKeyPrefixes map[string]string
	strictTokenKeyPrefixes map[string]string
	lineageKeyPrefixes     map[string]string
	now                    func() time.Time
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

func (d *Driver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.LeaseRecord{}, err
	}
	if err := validateAcquireRequest(req); err != nil {
		return backend.LeaseRecord{}, err
	}

	now := d.now()
	resourceKey := req.ResourceKeys[0]
	key := d.buildLeaseKey(req.DefinitionID, resourceKey)

	acquired, err := d.client.SetNX(ctx, key, req.OwnerID, req.LeaseTTL).Result()
	if err != nil {
		return backend.LeaseRecord{}, err
	}
	if !acquired {
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	}

	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}, nil
}

func (d *Driver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.FencedLeaseRecord{}, err
	}
	if err := validateStrictAcquireRequest(req); err != nil {
		return backend.FencedLeaseRecord{}, err
	}

	now := d.now()
	keys := []string{
		d.buildLeaseKey(req.DefinitionID, req.ResourceKey),
		d.buildStrictFenceCounterKey(req.DefinitionID, req.ResourceKey),
		d.buildStrictTokenKey(req.DefinitionID, req.ResourceKey),
	}
	raw, err := strictAcquireScript.Run(ctx, d.client, keys, req.OwnerID, req.LeaseTTL.Milliseconds()).Result()
	if err != nil {
		return backend.FencedLeaseRecord{}, err
	}

	status, ttlMillis, token, err := parseStrictStatusResult(raw)
	if err != nil {
		return backend.FencedLeaseRecord{}, err
	}
	switch status {
	case 1:
		if token <= 0 {
			return backend.FencedLeaseRecord{}, errInvalidScriptResponse
		}
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return backend.FencedLeaseRecord{
			Lease:        buildLeaseRecord(req.DefinitionID, req.ResourceKey, req.OwnerID, ttl, now),
			FencingToken: uint64(token),
		}, nil
	case -1:
		return backend.FencedLeaseRecord{}, backend.ErrLeaseAlreadyHeld
	case -3:
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	default:
		return backend.FencedLeaseRecord{}, errInvalidScriptResponse
	}
}

func (d *Driver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.LeaseRecord{}, err
	}
	if err := validateRenewLeaseRecord(lease); err != nil {
		return backend.LeaseRecord{}, err
	}

	resourceKey := lease.ResourceKeys[0]
	key := d.buildLeaseKey(lease.DefinitionID, resourceKey)

	ttlMillis, err := renewScript.Run(ctx, d.client, []string{key}, lease.OwnerID, lease.LeaseTTL.Milliseconds()).Int64()
	if err != nil {
		return backend.LeaseRecord{}, err
	}

	switch ttlMillis {
	case 0:
		return backend.LeaseRecord{}, backend.ErrLeaseNotFound
	case -1:
		return backend.LeaseRecord{}, backend.ErrLeaseOwnerMismatch
	case -2:
		return backend.LeaseRecord{}, backend.ErrLeaseExpired
	case -3:
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if ttlMillis < 0 {
		return backend.LeaseRecord{}, backend.ErrLeaseExpired
	}

	ttl := time.Duration(ttlMillis) * time.Millisecond
	now := d.now()

	return backend.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      lease.OwnerID,
		LeaseTTL:     ttl,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(ttl),
	}, nil
}

func (d *Driver) RenewStrict(
	ctx context.Context,
	lease backend.LeaseRecord,
	fencingToken uint64,
) (backend.FencedLeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.FencedLeaseRecord{}, err
	}
	if err := validateStrictRenewRequest(lease, fencingToken); err != nil {
		return backend.FencedLeaseRecord{}, err
	}

	now := d.now()
	keys := []string{
		d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]),
		d.buildStrictTokenKey(lease.DefinitionID, lease.ResourceKeys[0]),
	}
	raw, err := strictRenewScript.Run(
		ctx,
		d.client,
		keys,
		lease.OwnerID,
		fencingToken,
		lease.LeaseTTL.Milliseconds(),
	).Result()
	if err != nil {
		return backend.FencedLeaseRecord{}, err
	}

	status, ttlMillis, token, err := parseStrictStatusResult(raw)
	if err != nil {
		return backend.FencedLeaseRecord{}, err
	}
	switch status {
	case 1:
		if token <= 0 {
			return backend.FencedLeaseRecord{}, errInvalidScriptResponse
		}
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return backend.FencedLeaseRecord{
			Lease:        buildLeaseRecord(lease.DefinitionID, lease.ResourceKeys[0], lease.OwnerID, ttl, now),
			FencingToken: uint64(token),
		}, nil
	case 0:
		return backend.FencedLeaseRecord{}, backend.ErrLeaseNotFound
	case -1:
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	case -2:
		return backend.FencedLeaseRecord{}, backend.ErrLeaseExpired
	case -3:
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	default:
		return backend.FencedLeaseRecord{}, errInvalidScriptResponse
	}
}

func (d *Driver) Release(ctx context.Context, lease backend.LeaseRecord) error {
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
		return backend.ErrLeaseNotFound
	case -1:
		return backend.ErrLeaseOwnerMismatch
	default:
		return backend.ErrLeaseNotFound
	}
}

func (d *Driver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if err := validateStrictReleaseRequest(lease, fencingToken); err != nil {
		return err
	}

	keys := []string{
		d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]),
		d.buildStrictTokenKey(lease.DefinitionID, lease.ResourceKeys[0]),
	}
	result, err := strictReleaseScript.Run(ctx, d.client, keys, lease.OwnerID, fencingToken).Int64()
	if err != nil {
		return err
	}

	switch result {
	case 1:
		return nil
	case 0:
		return backend.ErrLeaseNotFound
	case -1:
		return backend.ErrLeaseOwnerMismatch
	case -2:
		return backend.ErrInvalidRequest
	default:
		return errInvalidScriptResponse
	}
}

func (d *Driver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.PresenceRecord{}, err
	}
	if err := validatePresenceRequest(req); err != nil {
		return backend.PresenceRecord{}, err
	}

	resourceKey := req.ResourceKeys[0]
	record := backend.PresenceRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
	}

	key := d.buildLeaseKey(req.DefinitionID, resourceKey)
	raw, err := presenceScript.Run(ctx, d.client, []string{key}).Result()
	if err != nil {
		return backend.PresenceRecord{}, err
	}

	present, owner, ttl, err := parsePresenceResult(raw)
	if err != nil {
		return backend.PresenceRecord{}, err
	}
	if !present {
		return record, nil
	}

	now := d.now()
	record.Present = true
	record.Lease = backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      owner,
		LeaseTTL:     ttl,
		ExpiresAt:    now.Add(ttl),
	}

	return record, nil
}

func (d *Driver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return backend.LeaseRecord{}, err
	}
	if err := validateLineageAcquireRequest(req); err != nil {
		return backend.LeaseRecord{}, err
	}

	now := d.now()
	keys := d.lineageAcquireKeys(req)
	raw, err := lineageAcquireScript.Run(ctx, d.client, keys, d.lineageAcquireArgs(req, now)...).Result()
	if err != nil {
		return backend.LeaseRecord{}, err
	}

	status, ttlMillis, err := parseLineageStatusResult(raw)
	if err != nil {
		return backend.LeaseRecord{}, err
	}
	switch status {
	case 1:
		return buildLeaseRecord(req.DefinitionID, req.ResourceKey, req.OwnerID, time.Duration(ttlMillis)*time.Millisecond, now), nil
	case -1:
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	case -2:
		return backend.LeaseRecord{}, backend.ErrOverlapRejected
	case -3:
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	default:
		return backend.LeaseRecord{}, errInvalidScriptResponse
	}
}

func (d *Driver) RenewWithLineage(
	ctx context.Context,
	lease backend.LeaseRecord,
	lineage backend.LineageLeaseMeta,
) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	if err := d.validateClient(); err != nil {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, err
	}
	if err := validateLineageRenewRequest(lease, lineage); err != nil {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, err
	}

	now := d.now()
	keys := d.lineageRenewKeys(lease, lineage)
	raw, err := lineageRenewScript.Run(ctx, d.client, keys, d.lineageRenewArgs(lease, lineage, now)...).Result()
	if err != nil {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, err
	}

	status, ttlMillis, err := parseLineageStatusResult(raw)
	if err != nil {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, err
	}
	switch status {
	case 1:
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return buildLeaseRecord(lease.DefinitionID, lease.ResourceKeys[0], lease.OwnerID, ttl, now), cloneLineageLeaseMeta(lineage), nil
	case 0:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseNotFound
	case -1:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseOwnerMismatch
	case -2:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	case -3:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrInvalidRequest
	case -4:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	default:
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, errInvalidScriptResponse
	}
}

func (d *Driver) ReleaseWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	if err := d.validateClient(); err != nil {
		return err
	}
	if err := validateLineageReleaseRequest(lease, lineage); err != nil {
		return err
	}

	result, err := lineageReleaseScript.Run(ctx, d.client, d.lineageReleaseKeys(lease, lineage), d.lineageReleaseArgs(lease, lineage, d.now())...).Int64()
	if err != nil {
		return err
	}

	switch result {
	case 1:
		return nil
	case 0:
		return backend.ErrLeaseNotFound
	case -1:
		return backend.ErrLeaseOwnerMismatch
	case -2:
		return backend.ErrInvalidRequest
	default:
		return errInvalidScriptResponse
	}
}

func (d *Driver) Ping(ctx context.Context) error {
	if err := d.validateClient(); err != nil {
		return err
	}

	return d.client.Ping(ctx).Err()
}

func (d *Driver) ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error {
	if err := d.validateClient(); err != nil {
		return err
	}

	keys := []string{
		d.buildLeaseKey(definitionID, resourceKey),
		d.buildStrictFenceCounterKey(definitionID, resourceKey),
		d.buildStrictTokenKey(definitionID, resourceKey),
	}

	_, err := d.client.Del(ctx, keys...).Result()
	return err
}

func (d *Driver) buildLeaseKey(definitionID, resourceKey string) string {
	return d.keyPrefixFor(d.cachedLeaseKeyPrefix, &d.leaseKeyPrefixes, definitionID) + encodeSegment(resourceKey)
}

func (d *Driver) buildStrictFenceCounterKey(definitionID, resourceKey string) string {
	return d.keyPrefixFor(d.cachedStrictFenceKeyPrefix, &d.strictFenceKeyPrefixes, definitionID) + encodeSegment(resourceKey)
}

func (d *Driver) buildStrictTokenKey(definitionID, resourceKey string) string {
	return d.keyPrefixFor(d.cachedStrictTokenKeyPrefix, &d.strictTokenKeyPrefixes, definitionID) + encodeSegment(resourceKey)
}

func (d *Driver) buildLineageKey(definitionID, resourceKey string) string {
	return d.keyPrefixFor(d.cachedLineageKeyPrefix, &d.lineageKeyPrefixes, definitionID) + encodeSegment(resourceKey)
}

func (d *Driver) lineageAcquireKeys(req backend.LineageAcquireRequest) []string {
	keys := make([]string, 0, 2+(len(req.Lineage.AncestorKeys)*2))
	keys = append(keys,
		d.buildLeaseKey(req.DefinitionID, req.ResourceKey),
		d.buildLineageKey(req.DefinitionID, req.ResourceKey),
	)
	for _, ancestor := range req.Lineage.AncestorKeys {
		keys = append(keys, d.buildLeaseKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	for _, ancestor := range req.Lineage.AncestorKeys {
		keys = append(keys, d.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	return keys
}

func (d *Driver) lineageAcquireArgs(req backend.LineageAcquireRequest, now time.Time) []interface{} {
	return []interface{}{
		req.OwnerID,
		req.LeaseTTL.Milliseconds(),
		now.UnixMilli(),
		string(req.Lineage.Kind),
		lineageMember(req.Lineage.LeaseID, req.DefinitionID, req.ResourceKey),
		len(req.Lineage.AncestorKeys),
	}
}

func (d *Driver) lineageRenewKeys(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) []string {
	keys := make([]string, 0, 1+len(lineage.AncestorKeys))
	keys = append(keys, d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]))
	for _, ancestor := range lineage.AncestorKeys {
		keys = append(keys, d.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	return keys
}

func (d *Driver) lineageRenewArgs(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta, now time.Time) []interface{} {
	return []interface{}{
		lease.OwnerID,
		lease.LeaseTTL.Milliseconds(),
		now.UnixMilli(),
		lineageMember(lineage.LeaseID, lease.DefinitionID, lease.ResourceKeys[0]),
		len(lineage.AncestorKeys),
	}
}

func (d *Driver) lineageReleaseKeys(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) []string {
	keys := make([]string, 0, 1+len(lineage.AncestorKeys))
	keys = append(keys, d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]))
	for _, ancestor := range lineage.AncestorKeys {
		keys = append(keys, d.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	return keys
}

func (d *Driver) lineageReleaseArgs(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta, now time.Time) []interface{} {
	return []interface{}{
		lease.OwnerID,
		now.UnixMilli(),
		lineageMember(lineage.LeaseID, lease.DefinitionID, lease.ResourceKeys[0]),
		len(lineage.AncestorKeys),
	}
}

func buildLeaseRecord(definitionID, resourceKey, ownerID string, ttl time.Duration, now time.Time) backend.LeaseRecord {
	return backend.LeaseRecord{
		DefinitionID: definitionID,
		ResourceKeys: []string{resourceKey},
		OwnerID:      ownerID,
		LeaseTTL:     ttl,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(ttl),
	}
}

func lineageMember(leaseID, definitionID, resourceKey string) string {
	return fmt.Sprintf("%s|%s|%s", leaseID, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func encodeSegment(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
}

// CacheDefinitionIDs pre-encodes definition IDs for faster key building.
func (d *Driver) CacheDefinitionIDs(ids []string) {
	if d.encodedIDs == nil {
		d.encodedIDs = make(map[string]string, len(ids))
	}
	if d.leaseKeyPrefixes == nil {
		d.leaseKeyPrefixes = make(map[string]string, len(ids))
	}
	if d.strictFenceKeyPrefixes == nil {
		d.strictFenceKeyPrefixes = make(map[string]string, len(ids))
	}
	if d.strictTokenKeyPrefixes == nil {
		d.strictTokenKeyPrefixes = make(map[string]string, len(ids))
	}
	if d.lineageKeyPrefixes == nil {
		d.lineageKeyPrefixes = make(map[string]string, len(ids))
	}
	for _, id := range ids {
		d.cacheDefinitionID(id)
	}
}

func (d *Driver) cacheDefinitionID(id string) {
	if d.encodedIDs == nil {
		d.encodedIDs = make(map[string]string)
	}
	encoded := encodeSegment(id)
	d.encodedIDs[id] = encoded
	d.cachedLeaseKeyPrefix(id, encoded)
	d.cachedStrictFenceKeyPrefix(id, encoded)
	d.cachedStrictTokenKeyPrefix(id, encoded)
	d.cachedLineageKeyPrefix(id, encoded)
}

func (d *Driver) encodeDefinitionID(id string) string {
	if d.encodedIDs != nil {
		if encoded, ok := d.encodedIDs[id]; ok {
			return encoded
		}
		encoded := encodeSegment(id)
		d.encodedIDs[id] = encoded
		return encoded
	}
	return encodeSegment(id)
}

func (d *Driver) keyPrefixFor(
	build func(string, string) string,
	cache *map[string]string,
	definitionID string,
) string {
	if *cache != nil {
		if prefix, ok := (*cache)[definitionID]; ok {
			return prefix
		}
	}
	encoded := d.encodeDefinitionID(definitionID)
	return build(definitionID, encoded)
}

func (d *Driver) cachedLeaseKeyPrefix(definitionID, encoded string) string {
	if d.leaseKeyPrefixes == nil {
		d.leaseKeyPrefixes = make(map[string]string)
	}
	prefix := d.keyPrefix + ":" + encoded + ":"
	d.leaseKeyPrefixes[definitionID] = prefix
	return prefix
}

func (d *Driver) cachedStrictFenceKeyPrefix(definitionID, encoded string) string {
	if d.strictFenceKeyPrefixes == nil {
		d.strictFenceKeyPrefixes = make(map[string]string)
	}
	prefix := d.keyPrefix + ":fence:" + encoded + ":"
	d.strictFenceKeyPrefixes[definitionID] = prefix
	return prefix
}

func (d *Driver) cachedStrictTokenKeyPrefix(definitionID, encoded string) string {
	if d.strictTokenKeyPrefixes == nil {
		d.strictTokenKeyPrefixes = make(map[string]string)
	}
	prefix := d.keyPrefix + ":strict-token:" + encoded + ":"
	d.strictTokenKeyPrefixes[definitionID] = prefix
	return prefix
}

func (d *Driver) cachedLineageKeyPrefix(definitionID, encoded string) string {
	if d.lineageKeyPrefixes == nil {
		d.lineageKeyPrefixes = make(map[string]string)
	}
	prefix := d.keyPrefix + ":lineage:" + encoded + ":"
	d.lineageKeyPrefixes[definitionID] = prefix
	return prefix
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

func parseLineageStatusResult(raw interface{}) (int64, int64, error) {
	values, ok := raw.([]interface{})
	if !ok || len(values) != 2 {
		return 0, 0, errInvalidScriptResponse
	}

	status, err := toInt64(values[0])
	if err != nil {
		return 0, 0, err
	}
	ttlMillis, err := toInt64(values[1])
	if err != nil {
		return 0, 0, err
	}
	return status, ttlMillis, nil
}

func parseStrictStatusResult(raw interface{}) (int64, int64, int64, error) {
	values, ok := raw.([]interface{})
	if !ok || len(values) != 3 {
		return 0, 0, 0, errInvalidScriptResponse
	}

	status, err := toInt64(values[0])
	if err != nil {
		return 0, 0, 0, err
	}
	ttlMillis, err := toInt64(values[1])
	if err != nil {
		return 0, 0, 0, err
	}
	token, err := toInt64(values[2])
	if err != nil {
		return 0, 0, 0, err
	}

	return status, ttlMillis, token, nil
}

func cloneLineageLeaseMeta(meta backend.LineageLeaseMeta) backend.LineageLeaseMeta {
	out := meta
	if len(meta.AncestorKeys) > 0 {
		out.AncestorKeys = append([]backend.AncestorKey(nil), meta.AncestorKeys...)
	}
	return out
}

func (d *Driver) validateClient() error {
	if d == nil || d.client == nil {
		return backend.ErrInvalidRequest
	}
	return nil
}

func validateAcquireRequest(req backend.AcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" {
		return backend.ErrInvalidRequest
	}
	if len(req.ResourceKeys) != 1 || strings.TrimSpace(req.ResourceKeys[0]) == "" {
		return backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateLeaseRecord(lease backend.LeaseRecord) error {
	if strings.TrimSpace(lease.DefinitionID) == "" || strings.TrimSpace(lease.OwnerID) == "" {
		return backend.ErrInvalidRequest
	}
	if len(lease.ResourceKeys) != 1 || strings.TrimSpace(lease.ResourceKeys[0]) == "" {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateRenewLeaseRecord(lease backend.LeaseRecord) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	if lease.LeaseTTL <= 0 {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validatePresenceRequest(req backend.PresenceRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return backend.ErrInvalidRequest
	}
	if len(req.ResourceKeys) != 1 || strings.TrimSpace(req.ResourceKeys[0]) == "" {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateLineageAcquireRequest(req backend.LineageAcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.ResourceKey) == "" {
		return backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.ErrInvalidRequest
	}
	return validateLineageLeaseMeta(req.Lineage)
}

func validateStrictAcquireRequest(req backend.StrictAcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.ResourceKey) == "" {
		return backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateStrictRenewRequest(lease backend.LeaseRecord, fencingToken uint64) error {
	if err := validateRenewLeaseRecord(lease); err != nil {
		return err
	}
	if fencingToken == 0 {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateStrictReleaseRequest(lease backend.LeaseRecord, fencingToken uint64) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	if fencingToken == 0 {
		return backend.ErrInvalidRequest
	}

	return nil
}

func validateLineageRenewRequest(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	if err := validateRenewLeaseRecord(lease); err != nil {
		return err
	}
	return validateLineageLeaseMeta(lineage)
}

func validateLineageReleaseRequest(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	return validateLineageLeaseMeta(lineage)
}

func validateLineageLeaseMeta(meta backend.LineageLeaseMeta) error {
	if strings.TrimSpace(meta.LeaseID) == "" {
		return backend.ErrInvalidRequest
	}
	if meta.Kind != backend.KindParent && meta.Kind != backend.KindChild {
		return backend.ErrInvalidRequest
	}
	for _, ancestor := range meta.AncestorKeys {
		if strings.TrimSpace(ancestor.DefinitionID) == "" || strings.TrimSpace(ancestor.ResourceKey) == "" {
			return backend.ErrInvalidRequest
		}
	}
	return nil
}
