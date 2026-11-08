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
)

type Mutation struct {
	MutationID uuid.UUID              `json:"mutation_id"`
	EntityName string                 `json:"entity_name"`
	EntityID   uuid.UUID              `json:"entity_id"`
	Operation  OperationType          `json:"operation"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

type PushPullRequest struct {
	ClientID      string     `json:"client_id"`
	SinceRevision int64      `json:"since_revision"`
	Mutations     []Mutation `json:"mutations"`
}

type PushPullResponse struct {
	LatestRevision     int64       `json:"latest_revision"`
	AppliedMutationIDs []uuid.UUID `json:"applied_mutation_ids"`
	ServerChanges      []Mutation  `json:"server_changes"`
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
