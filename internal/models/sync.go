package models

import (
	"time"

	"github.com/google/uuid"
)

type OperationType string

const (
	OpInsert OperationType = "INSERT"
	OpUpdate OperationType = "UPDATE"
	OpDelete OperationType = "DELETE"
	OpUpsert OperationType = "UPSERT"
)

type Mutation struct {
	MutationID uuid.UUID              `json:"mutation_id"`
	EntityName string                 `json:"entity_name"`
	EntityID   uuid.UUID              `json:"entity_id"`
	Operation  OperationType          `json:"operation"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	UpdatedAt  *time.Time             `json:"updated_at,omitempty"`
}

type PushPullRequest struct {
	ClientID               string     `json:"client_id"`
	SinceRevision          int64      `json:"since_revision"`
	ClientLastSyncRevision int64      `json:"client_last_sync_revision,omitempty"`
	Mutations              []Mutation `json:"mutations"`
}

type PushPullResponse struct {
	LatestRevision          int64                    `json:"latest_revision"`
	ServerCurrentRevision   int64                    `json:"server_current_revision"`
	AppliedMutationIDs      []uuid.UUID              `json:"applied_mutation_ids"`
	AcknowledgedMutationIDs []uuid.UUID              `json:"acknowledged_mutation_ids"`
	RejectedMutations       []any                    `json:"rejected_mutations"`
	ServerChanges           []Mutation               `json:"server_changes"`
	Deltas                  map[string]interface{}   `json:"deltas"`
	Tombstones              []map[string]interface{} `json:"tombstones"`
}

type SyncChangelog struct {
	Revision   int64         `json:"revision"`
	UserID     uuid.UUID     `json:"user_id"`
	ClientID   string        `json:"client_id"`
	EntityName string        `json:"entity_name"`
	EntityID   uuid.UUID     `json:"entity_id"`
	Operation  OperationType `json:"operation"`
	MutationID uuid.UUID     `json:"mutation_id"`
	CreatedAt  time.Time     `json:"created_at"`
}
