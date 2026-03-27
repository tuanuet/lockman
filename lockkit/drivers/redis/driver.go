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

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
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

func (d *Driver) AcquireStrict(ctx context.Context, req drivers.StrictAcquireRequest) (drivers.FencedLeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.FencedLeaseRecord{}, err
	}
	if err := validateStrictAcquireRequest(req); err != nil {
		return drivers.FencedLeaseRecord{}, err
	}

	now := d.now()
	keys := []string{
		d.buildLeaseKey(req.DefinitionID, req.ResourceKey),
		d.buildStrictFenceCounterKey(req.DefinitionID, req.ResourceKey),
		d.buildStrictTokenKey(req.DefinitionID, req.ResourceKey),
	}
	raw, err := strictAcquireScript.Run(ctx, d.client, keys, req.OwnerID, req.LeaseTTL.Milliseconds()).Result()
	if err != nil {
		return drivers.FencedLeaseRecord{}, err
	}

	status, ttlMillis, token, err := parseStrictStatusResult(raw)
	if err != nil {
		return drivers.FencedLeaseRecord{}, err
	}
	switch status {
	case 1:
		if token <= 0 {
			return drivers.FencedLeaseRecord{}, errInvalidScriptResponse
		}
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return drivers.FencedLeaseRecord{
			Lease:        buildLeaseRecord(req.DefinitionID, req.ResourceKey, req.OwnerID, ttl, now),
			FencingToken: uint64(token),
		}, nil
	case -1:
		return drivers.FencedLeaseRecord{}, drivers.ErrLeaseAlreadyHeld
	case -3:
		return drivers.FencedLeaseRecord{}, drivers.ErrInvalidRequest
	default:
		return drivers.FencedLeaseRecord{}, errInvalidScriptResponse
	}
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

func (d *Driver) RenewStrict(
	ctx context.Context,
	lease drivers.LeaseRecord,
	fencingToken uint64,
) (drivers.FencedLeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.FencedLeaseRecord{}, err
	}
	if err := validateStrictRenewRequest(lease, fencingToken); err != nil {
		return drivers.FencedLeaseRecord{}, err
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
		return drivers.FencedLeaseRecord{}, err
	}

	status, ttlMillis, token, err := parseStrictStatusResult(raw)
	if err != nil {
		return drivers.FencedLeaseRecord{}, err
	}
	switch status {
	case 1:
		if token <= 0 {
			return drivers.FencedLeaseRecord{}, errInvalidScriptResponse
		}
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return drivers.FencedLeaseRecord{
			Lease:        buildLeaseRecord(lease.DefinitionID, lease.ResourceKeys[0], lease.OwnerID, ttl, now),
			FencingToken: uint64(token),
		}, nil
	case 0:
		return drivers.FencedLeaseRecord{}, drivers.ErrLeaseNotFound
	case -1:
		return drivers.FencedLeaseRecord{}, drivers.ErrLeaseOwnerMismatch
	case -2:
		return drivers.FencedLeaseRecord{}, drivers.ErrLeaseExpired
	case -3:
		return drivers.FencedLeaseRecord{}, drivers.ErrInvalidRequest
	default:
		return drivers.FencedLeaseRecord{}, errInvalidScriptResponse
	}
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

func (d *Driver) ReleaseStrict(ctx context.Context, lease drivers.LeaseRecord, fencingToken uint64) error {
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
		return drivers.ErrLeaseNotFound
	case -1:
		return drivers.ErrLeaseOwnerMismatch
	case -2:
		return drivers.ErrInvalidRequest
	default:
		return errInvalidScriptResponse
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

func (d *Driver) AcquireWithLineage(ctx context.Context, req drivers.LineageAcquireRequest) (drivers.LeaseRecord, error) {
	if err := d.validateClient(); err != nil {
		return drivers.LeaseRecord{}, err
	}
	if err := validateLineageAcquireRequest(req); err != nil {
		return drivers.LeaseRecord{}, err
	}

	now := d.now()
	keys := d.lineageAcquireKeys(req)
	raw, err := lineageAcquireScript.Run(ctx, d.client, keys, d.lineageAcquireArgs(req, now)...).Result()
	if err != nil {
		return drivers.LeaseRecord{}, err
	}

	status, ttlMillis, err := parseLineageStatusResult(raw)
	if err != nil {
		return drivers.LeaseRecord{}, err
	}
	switch status {
	case 1:
		return buildLeaseRecord(req.DefinitionID, req.ResourceKey, req.OwnerID, time.Duration(ttlMillis)*time.Millisecond, now), nil
	case -1:
		return drivers.LeaseRecord{}, drivers.ErrLeaseAlreadyHeld
	case -2:
		return drivers.LeaseRecord{}, lockerrors.ErrOverlapRejected
	case -3:
		return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
	default:
		return drivers.LeaseRecord{}, errInvalidScriptResponse
	}
}

func (d *Driver) RenewWithLineage(
	ctx context.Context,
	lease drivers.LeaseRecord,
	lineage drivers.LineageLeaseMeta,
) (drivers.LeaseRecord, drivers.LineageLeaseMeta, error) {
	if err := d.validateClient(); err != nil {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, err
	}
	if err := validateLineageRenewRequest(lease, lineage); err != nil {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, err
	}

	now := d.now()
	keys := d.lineageRenewKeys(lease, lineage)
	raw, err := lineageRenewScript.Run(ctx, d.client, keys, d.lineageRenewArgs(lease, lineage, now)...).Result()
	if err != nil {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, err
	}

	status, ttlMillis, err := parseLineageStatusResult(raw)
	if err != nil {
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, err
	}
	switch status {
	case 1:
		ttl := time.Duration(ttlMillis) * time.Millisecond
		return buildLeaseRecord(lease.DefinitionID, lease.ResourceKeys[0], lease.OwnerID, ttl, now), cloneLineageLeaseMeta(lineage), nil
	case 0:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseNotFound
	case -1:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseOwnerMismatch
	case -2:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseExpired
	case -3:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrInvalidRequest
	case -4:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, drivers.ErrLeaseExpired
	default:
		return drivers.LeaseRecord{}, drivers.LineageLeaseMeta{}, errInvalidScriptResponse
	}
}

func (d *Driver) ReleaseWithLineage(ctx context.Context, lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) error {
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
		return drivers.ErrLeaseNotFound
	case -1:
		return drivers.ErrLeaseOwnerMismatch
	case -2:
		return drivers.ErrInvalidRequest
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

func (d *Driver) buildLeaseKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func (d *Driver) buildStrictFenceCounterKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:fence:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func (d *Driver) buildStrictTokenKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:strict-token:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func (d *Driver) buildLineageKey(definitionID, resourceKey string) string {
	return fmt.Sprintf("%s:lineage:%s:%s", d.keyPrefix, encodeSegment(definitionID), encodeSegment(resourceKey))
}

func (d *Driver) lineageAcquireKeys(req drivers.LineageAcquireRequest) []string {
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

func (d *Driver) lineageAcquireArgs(req drivers.LineageAcquireRequest, now time.Time) []interface{} {
	return []interface{}{
		req.OwnerID,
		req.LeaseTTL.Milliseconds(),
		now.UnixMilli(),
		string(req.Lineage.Kind),
		lineageMember(req.Lineage.LeaseID, req.DefinitionID, req.ResourceKey),
		len(req.Lineage.AncestorKeys),
	}
}

func (d *Driver) lineageRenewKeys(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) []string {
	keys := make([]string, 0, 1+len(lineage.AncestorKeys))
	keys = append(keys, d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]))
	for _, ancestor := range lineage.AncestorKeys {
		keys = append(keys, d.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	return keys
}

func (d *Driver) lineageRenewArgs(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta, now time.Time) []interface{} {
	return []interface{}{
		lease.OwnerID,
		lease.LeaseTTL.Milliseconds(),
		now.UnixMilli(),
		lineageMember(lineage.LeaseID, lease.DefinitionID, lease.ResourceKeys[0]),
		len(lineage.AncestorKeys),
	}
}

func (d *Driver) lineageReleaseKeys(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) []string {
	keys := make([]string, 0, 1+len(lineage.AncestorKeys))
	keys = append(keys, d.buildLeaseKey(lease.DefinitionID, lease.ResourceKeys[0]))
	for _, ancestor := range lineage.AncestorKeys {
		keys = append(keys, d.buildLineageKey(ancestor.DefinitionID, ancestor.ResourceKey))
	}
	return keys
}

func (d *Driver) lineageReleaseArgs(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta, now time.Time) []interface{} {
	return []interface{}{
		lease.OwnerID,
		now.UnixMilli(),
		lineageMember(lineage.LeaseID, lease.DefinitionID, lease.ResourceKeys[0]),
		len(lineage.AncestorKeys),
	}
}

func buildLeaseRecord(definitionID, resourceKey, ownerID string, ttl time.Duration, now time.Time) drivers.LeaseRecord {
	return drivers.LeaseRecord{
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

func cloneLineageLeaseMeta(meta drivers.LineageLeaseMeta) drivers.LineageLeaseMeta {
	out := meta
	if len(meta.AncestorKeys) > 0 {
		out.AncestorKeys = append([]drivers.AncestorKey(nil), meta.AncestorKeys...)
	}
	return out
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

func validateLineageAcquireRequest(req drivers.LineageAcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.ResourceKey) == "" {
		return drivers.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return drivers.ErrInvalidRequest
	}
	return validateLineageLeaseMeta(req.Lineage)
}

func validateStrictAcquireRequest(req drivers.StrictAcquireRequest) error {
	if strings.TrimSpace(req.DefinitionID) == "" || strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.ResourceKey) == "" {
		return drivers.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validateStrictRenewRequest(lease drivers.LeaseRecord, fencingToken uint64) error {
	if err := validateRenewLeaseRecord(lease); err != nil {
		return err
	}
	if fencingToken == 0 {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validateStrictReleaseRequest(lease drivers.LeaseRecord, fencingToken uint64) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	if fencingToken == 0 {
		return drivers.ErrInvalidRequest
	}

	return nil
}

func validateLineageRenewRequest(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) error {
	if err := validateRenewLeaseRecord(lease); err != nil {
		return err
	}
	return validateLineageLeaseMeta(lineage)
}

func validateLineageReleaseRequest(lease drivers.LeaseRecord, lineage drivers.LineageLeaseMeta) error {
	if err := validateLeaseRecord(lease); err != nil {
		return err
	}
	return validateLineageLeaseMeta(lineage)
}

func validateLineageLeaseMeta(meta drivers.LineageLeaseMeta) error {
	if strings.TrimSpace(meta.LeaseID) == "" {
		return drivers.ErrInvalidRequest
	}
	if meta.Kind != definitions.KindParent && meta.Kind != definitions.KindChild {
		return drivers.ErrInvalidRequest
	}
	for _, ancestor := range meta.AncestorKeys {
		if strings.TrimSpace(ancestor.DefinitionID) == "" || strings.TrimSpace(ancestor.ResourceKey) == "" {
			return drivers.ErrInvalidRequest
		}
	}
	return nil
}
