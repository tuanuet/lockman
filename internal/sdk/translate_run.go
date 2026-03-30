package sdk

import "github.com/tuanuet/lockman/lockkit/definitions"

func translateRun(req runRequest) definitions.SyncLockRequest {
	return definitions.SyncLockRequest{
		DefinitionID: string(req.useCaseID),
		KeyInput: map[string]string{
			resourceKeyInputKey: req.resourceKey,
		},
		Ownership: definitions.OwnershipMeta{
			HandlerName: req.publicName,
			OwnerID:     req.ownerID,
		},
	}
}

// TranslateRun maps normalized run requests into low-level lockkit requests.
func TranslateRun(req RunRequest) definitions.SyncLockRequest {
	return translateRun(req.internal)
}
