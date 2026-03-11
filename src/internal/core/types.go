package core

import "context"

type ProviderCapabilities struct {
	SupportsComments                bool
	SupportsStructuredTags          bool
	SupportsServerSideCommentFilter bool
	SupportsServerSideTagFilter     bool
	SupportsPerRecordUpdates        bool
	SupportsRRSetUpdates            bool
	SupportsWildcardRecords         bool
	SupportsProxiedFlag             bool
	SupportsBatchChanges            bool
}

type Tag struct {
	Name  string
	Value string
}

type Record struct {
	Provider           string
	Zone               string
	Type               string
	Name               string
	Content            string
	TTL                int
	Enabled            bool
	Proxied            bool
	Comment            string
	Tags               []Tag
	OwnershipMode      string
	RecordTemplateID   string
	ProviderRecordID   string
	DesiredFingerprint string
}

type RecordFilter struct {
	Zone      string
	Name      string
	Type      string
	Ownership map[string]string
	Tags      []Tag
}

type Provider interface {
	Name() string
	ValidateConfig(config map[string]any) error
	Capabilities() ProviderCapabilities
	ListRecords(ctx context.Context, filter RecordFilter) ([]Record, error)
	CreateRecord(ctx context.Context, record Record) (Record, error)
	UpdateRecord(ctx context.Context, record Record) (Record, error)
	DeleteRecord(ctx context.Context, record Record) error
}