package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type File struct {
	NodeID           string                 `json:"nodeId"`
	Hostname         string                 `json:"hostname"`
	ConfigChecksum   string                 `json:"configChecksum"`
	PublicIPv4Last   string                 `json:"publicIPv4LastSeen,omitempty"`
	PublicIPv6Last   string                 `json:"publicIPv6LastSeen,omitempty"`
	LastSuccessUTC   string                 `json:"lastSuccessUtc,omitempty"`
	Records          map[string]RecordState `json:"records"`
	PendingCleanup   []CleanupItem          `json:"pendingCleanup,omitempty"`
	SelectedSnapshot map[string]string      `json:"selectedAddressSnapshots,omitempty"`
}

type RecordState struct {
	ProviderRecordID   string `json:"providerRecordId,omitempty"`
	DesiredFingerprint string `json:"desiredStateFingerprint,omitempty"`
	SelectedAddress    string `json:"selectedAddress,omitempty"`
	LastReconciledUTC  string `json:"lastReconciledUtc,omitempty"`
}

type CleanupItem struct {
	RecordTemplateID string `json:"recordTemplateId"`
	Provider         string `json:"provider"`
	Zone             string `json:"zone"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	ProviderRecordID string `json:"providerRecordId,omitempty"`
}

// PruneOrphans removes state entries whose record template ID no longer
// exists in the current config. Returns the number of entries removed.
func (f *File) PruneOrphans(activeIDs map[string]struct{}) int {
	pruned := 0
	for id := range f.Records {
		if _, ok := activeIDs[id]; !ok {
			delete(f.Records, id)
			pruned++
		}
	}
	return pruned
}

type Store interface {
	Load(ctx context.Context) (File, error)
	Save(ctx context.Context, state File) error
}

type JSONStore struct {
	Path string
}

func (s JSONStore) Load(_ context.Context) (File, error) {
	content, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{Records: map[string]RecordState{}}, nil
		}
		return File{}, fmt.Errorf("read state %q: %w", s.Path, err)
	}
	var state File
	if err := json.Unmarshal(content, &state); err != nil {
		return File{}, fmt.Errorf("decode state %q: %w", s.Path, err)
	}
	if state.Records == nil {
		state.Records = map[string]RecordState{}
	}
	return state, nil
}

func (s JSONStore) Save(_ context.Context, state File) error {
	if state.Records == nil {
		state.Records = map[string]RecordState{}
	}
	if state.LastSuccessUTC == "" {
		state.LastSuccessUTC = time.Now().UTC().Format(time.RFC3339)
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state %q: %w", s.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("ensure state directory for %q: %w", s.Path, err)
	}
	tempPath := s.Path + ".tmp"
	if err := os.WriteFile(tempPath, content, 0o600); err != nil {
		return fmt.Errorf("write temp state %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		return fmt.Errorf("replace state %q: %w", s.Path, err)
	}
	return nil
}