// Package registry provides model capability enrichment using Catwalk metadata.
//
// Two-tier initialization:
//   - Package init() loads compiled-in embedded data (always available, instant, zero network).
//   - Refresh() optionally fetches live data from the Catwalk API for freshest metadata.
//
// Enrich() fills capability gaps on a model without overwriting API-provided values.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/cache"
	"github.com/idelchi/aura/pkg/llm/model"
	"github.com/idelchi/aura/pkg/providers/capabilities"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/catwalk/pkg/embedded"
)

const (
	catwalkURL   = "https://catwalk.charm.sh"
	fetchTimeout = 3 * time.Second
)

// providerMapping maps Aura provider types to Catwalk provider IDs.
var providerMapping = map[string]catwalk.InferenceProvider{
	"google": catwalk.InferenceProviderGemini,
	"codex":  catwalk.InferenceProviderOpenAI,
}

var (
	mu    sync.RWMutex
	index map[catwalk.InferenceProvider]map[string]catwalk.Model
)

func init() {
	index = buildIndex(embedded.GetAll())
}

// Refresh fetches live Catwalk data and updates the in-memory index.
// Falls back to disk cache, then to the already-loaded embedded data.
// Safe to skip — Enrich() always works using embedded data from init().
func Refresh(ctx context.Context, domain *cache.Domain) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	client := catwalk.NewWithURL(catwalkURL)

	// Read cached etag for conditional fetch.
	etag := readCached(domain, "etag")

	providers, err := client.GetProviders(ctx, etag)
	if err != nil {
		if errors.Is(err, catwalk.ErrNotModified) {
			debug.Log("registry: catwalk data unchanged (etag match)")

			return
		}

		debug.Log("registry: live fetch failed: %v, trying disk cache", err)

		providers = loadDiskCache(domain)
		if providers == nil {
			debug.Log("registry: no disk cache, using embedded data")

			return
		}
	} else {
		writeDiskCache(domain, providers)
	}

	mu.Lock()
	index = buildIndex(providers)
	mu.Unlock()

	debug.Log("registry: refreshed with %d providers", len(providers))
}

// Enrich fills capability gaps for providers whose APIs don't expose full model metadata.
// Anthropic and OpenAI listing endpoints return only name/ID — no context length, vision,
// thinking, or other capabilities. Google exposes context length, thinking, and tools but
// not vision. Enrich looks up the missing fields from Catwalk data for these three providers.
// Providers with rich APIs (Ollama, OpenRouter, LlamaCPP, Copilot, Codex) set all capabilities
// inline from their API responses and don't need enrichment.
//
// Non-destructive: only fills gaps, never overwrites API-provided values.
//
// For ALL models: always adds Tools (safe default for chat models).
// For matched models: sets ContextLength (if zero), Vision, Thinking, ThinkingLevels per Catwalk data.
// For unmatched models: Tools only (no false positives).
func Enrich(providerType string, m *model.Model) {
	m.Capabilities.Add(capabilities.Tools)

	cm := lookup(providerType, m.Name)
	if cm == nil {
		return
	}

	if m.ContextLength == 0 && cm.ContextWindow > 0 {
		m.ContextLength = model.ContextLength(cm.ContextWindow)
	}

	if cm.SupportsImages {
		m.Capabilities.Add(capabilities.Vision)
	}

	if cm.CanReason {
		m.Capabilities.Add(capabilities.Thinking)
	}

	if len(cm.ReasoningLevels) > 0 {
		m.Capabilities.Add(capabilities.ThinkingLevels)
	}
}

func mapProviderID(providerType string) catwalk.InferenceProvider {
	if mapped, ok := providerMapping[providerType]; ok {
		return mapped
	}

	return catwalk.InferenceProvider(providerType)
}

func lookup(providerType, modelID string) *catwalk.Model {
	mu.RLock()
	defer mu.RUnlock()

	catwalkID := mapProviderID(providerType)

	models, ok := index[catwalkID]
	if !ok {
		return nil
	}

	m, ok := models[modelID]
	if !ok {
		return nil
	}

	return &m
}

func buildIndex(providers []catwalk.Provider) map[catwalk.InferenceProvider]map[string]catwalk.Model {
	idx := make(map[catwalk.InferenceProvider]map[string]catwalk.Model, len(providers))

	for _, p := range providers {
		models := make(map[string]catwalk.Model, len(p.Models))
		for _, m := range p.Models {
			models[m.ID] = m
		}

		idx[p.ID] = models
	}

	return idx
}

func readCached(domain *cache.Domain, key string) string {
	data, ok := domain.Read(key)
	if !ok {
		return ""
	}

	return string(data)
}

func loadDiskCache(domain *cache.Domain) []catwalk.Provider {
	data, ok := domain.Read("data.json")
	if !ok {
		return nil
	}

	var providers []catwalk.Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil
	}

	return providers
}

func writeDiskCache(domain *cache.Domain, providers []catwalk.Provider) {
	data, err := json.Marshal(providers)
	if err != nil {
		debug.Log("[catwalk] cache marshal: %v", err)

		return
	}

	if err := domain.Write("data.json", data); err != nil {
		debug.Log("[catwalk] cache write: %v", err)
	}

	if err := domain.Write("etag", []byte(catwalk.Etag(data))); err != nil {
		debug.Log("[catwalk] etag write: %v", err)
	}
}
