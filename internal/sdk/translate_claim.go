package sdk

import "lockman/lockkit/definitions"

func translateClaim(req claimRequest) definitions.MessageClaimRequest {
	idempotencyKey := ""
	if req.idempotent {
		idempotencyKey = req.delivery.messageID
	}

	return definitions.MessageClaimRequest{
		DefinitionID: string(req.useCaseID),
		KeyInput: map[string]string{
			resourceKeyInputKey: req.resourceKey,
		},
		Ownership: definitions.OwnershipMeta{
			HandlerName:   req.publicName,
			OwnerID:       req.ownerID,
			MessageID:     req.delivery.messageID,
			Attempt:       req.delivery.attempt,
			ConsumerGroup: req.delivery.consumerGroup,
		},
		IdempotencyKey: idempotencyKey,
	}
}

// TranslateClaim maps normalized claim requests into low-level lockkit requests.
func TranslateClaim(req ClaimRequest) definitions.MessageClaimRequest {
	return translateClaim(req.internal)
}
