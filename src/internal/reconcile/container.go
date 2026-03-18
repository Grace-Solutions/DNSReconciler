package reconcile

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/containerrt"
	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// ExpandContainerRecords discovers routable containers and generates a
// concrete RecordTemplate for each (containerRecord template × container) pair.
// The generated recordId is deterministic: SHA-256(templateKey + containerID)
// where templateKey is derived from providerId + type + name, ensuring stable
// IDs across restarts without requiring a user-supplied templateId.
func ExpandContainerRecords(
	ctx context.Context,
	logger *logging.Logger,
	detector *containerrt.Detector,
	cfg *config.Config,
) []config.RecordTemplate {
	if detector == nil || !detector.HasRuntime() {
		return nil
	}
	if len(cfg.ContainerRecords) == 0 {
		return nil
	}

	var generated []config.RecordTemplate

	for i, ct := range cfg.ContainerRecords {
		templateLabel := fmt.Sprintf("[%d] %s/%s", i, ct.Type, ct.Name)

		if !ct.IsEnabled() {
			logger.Debug(fmt.Sprintf("Container template %s is disabled, skipping", templateLabel))
			continue
		}

		prov := cfg.FindProvider(ct.ProviderID)
		merged := config.MergeContainerDefaults(ct, prov)

		// Derive a stable template key from the required fields.
		templateKey := merged.ProviderID + "|" + merged.Type + "|" + merged.Name

		// Discover containers matching this template's include/exclude filters.
		containers := detector.RoutableContainers(ctx, merged.Include, merged.Exclude, merged.MatchFields)
		if len(containers) == 0 {
			logger.Debug(fmt.Sprintf("Container template %s: no matching containers found", templateLabel))
			continue
		}

		logger.Information(fmt.Sprintf("Container template %s: found %d routable container(s)", templateLabel, len(containers)))

		for _, rc := range containers {
			recordID := deterministicID(templateKey, rc.ID)

			// Auto-inject ownership tags using variable references. These
			// are expanded alongside user tags in reconcileOne. If the
			// provider doesn't support structured tags, they're stripped by
			// the existing capability gating — no harm done. This allows
			// providers with tag support (Azure, Cloudflare) to match
			// ownership via tags instead of being limited by comment length.
			ownershipTags := []config.Tag{
				{Name: "nodeId", Value: "${NODE_ID}"},
				{Name: "containerName", Value: "${CONTAINER_NAME}"},
				{Name: "containerId", Value: "${CONTAINER_ID}"},
			}

			// Append user-defined tags first, then ownership tags.
			allTags := make([]config.Tag, 0, len(merged.Tags)+len(ownershipTags))
			allTags = append(allTags, merged.Tags...)
			allTags = append(allTags, ownershipTags...)

			// Build a concrete RecordTemplate from the container template.
			// Variable expansion happens later in reconcileOne — we only
			// inject the container-specific variable references here.
			rec := config.RecordTemplate{
				ProviderID: merged.ProviderID,
				RecordID:   recordID,
				Enabled:    merged.Enabled,
				Type:       merged.Type,
				Name:       merged.Name,
				Content:    merged.Content,
				Zone:       merged.Zone,
				TTL:        merged.TTL,
				Proxied:    merged.Proxied,
				Comment:    merged.Comment,
				Tags:       allTags,
				Ownership:  merged.Ownership,
				// Store container metadata so reconcileOne can build the
				// container-aware expansion context.
				ContainerMeta: &config.ContainerMeta{
					ContainerName:     rc.Name,
					ContainerHostname: rc.Hostname,
					ContainerID:       shortID(rc.ID),
					ContainerIP:       rc.RoutableIP,
					ContainerImage:    rc.Image,
					Labels:            rc.Labels,
				},
			}

			logger.Debug(fmt.Sprintf("Container template %s → record %s for container %q (%s)",
				templateLabel, recordID, rc.Name, rc.RoutableIP))

			generated = append(generated, rec)
		}
	}

	return generated
}

// deterministicID produces a stable UUID-formatted ID from a template key and
// container ID so the same container always maps to the same record.
func deterministicID(templateKey, containerID string) string {
	h := sha256.Sum256([]byte(templateKey + ":" + containerID))
	// Format as UUID v4-like: xxxxxxxx-xxxx-4xxx-8xxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-4%x-8%x-%x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// shortID returns the first 12 characters of a container ID, matching the
// Docker/Podman short ID convention.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

