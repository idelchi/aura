package providers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/idelchi/aura/internal/config"
	"github.com/idelchi/aura/internal/debug"
	"github.com/idelchi/aura/pkg/cache"
	"github.com/idelchi/aura/pkg/llm/model"
)

// cachedProvider shares catalog and inspected metadata across provider instances.
// Only Model inspects a worker; Models never calls Model or loads workers.
type cachedProvider struct {
	Provider
	catalog  *cache.Domain
	metadata *cache.Domain
	key      string
}

func newCachedProvider(p Provider, cfg config.Provider) *cachedProvider {
	// Include the connection and credential scope, without storing credentials.
	identity, _ := json.Marshal([]any{cfg.Type, cfg.URL, cfg.Token, cfg.AuthDirs})
	return &cachedProvider{
		Provider: p,
		catalog:  cfg.Cache.Domain("models"),
		metadata: cfg.Cache.Domain("model-metadata"),
		key:      fmt.Sprintf("%x", sha256.Sum256(identity)),
	}
}

func (c *cachedProvider) Unwrap() Provider { return c.Provider }

func (c *cachedProvider) metadataKey(name string) string {
	return fmt.Sprintf("%s-%x.json", c.key, sha256.Sum256([]byte(name)))
}

func (c *cachedProvider) readModel(name string) (model.Model, bool) {
	data, ok := c.metadata.Read(c.metadataKey(name))
	var m model.Model
	if !ok || json.Unmarshal(data, &m) != nil || m.Name != name {
		return model.Model{}, false
	}
	return m, true
}

func (c *cachedProvider) writeModel(m model.Model) {
	c.write(c.metadata, c.metadataKey(m.Name), m)
}

func (c *cachedProvider) write(domain *cache.Domain, key string, value any) {
	data, err := json.Marshal(value)
	if err == nil {
		err = domain.Write(key, data)
	}
	if err != nil {
		debug.Log("[models] cache write: %v", err)
	}
}

// CachedModels is the CLI catalog path; interactive provider queries stay live.
func CachedModels(ctx context.Context, cfg config.Provider) (model.Models, error) {
	p, err := New(cfg)
	if err != nil {
		return nil, err
	}
	c, ok := p.(*cachedProvider)
	if !ok {
		return p.Models(ctx)
	}
	key := c.key + ".json"
	var models model.Models
	data, hit := c.catalog.Read(key)
	hit = hit && json.Unmarshal(data, &models) == nil
	if !hit {
		return c.Models(ctx)
	}
	for i, m := range models {
		if previous, ok := c.readModel(m.Name); ok {
			models[i] = m.FillMissing(previous)
		}
	}
	return models, nil
}

func (c *cachedProvider) Models(ctx context.Context) (model.Models, error) {
	models, err := c.Provider.Models(ctx)
	if err != nil {
		return nil, err
	}
	c.write(c.catalog, c.key+".json", models)
	for i, m := range models {
		if previous, ok := c.readModel(m.Name); ok {
			models[i] = m.FillMissing(previous)
			c.writeModel(models[i])
		} else if c.metadata.Exists(c.metadataKey(m.Name)) {
			// A no-cache refresh must replace saved observations as well, so
			// older capabilities cannot reappear on a later ordinary listing.
			c.writeModel(m)
		}
	}
	return models, nil
}

func (c *cachedProvider) Model(ctx context.Context, name string) (model.Model, error) {
	m, err := c.Provider.Model(ctx, name)
	if err != nil {
		return model.Model{}, err
	}
	if previous, ok := c.readModel(m.Name); ok {
		m = m.FillMissing(previous)
	}
	c.writeModel(m)
	// A subsequent listing must not override this inspection with an older
	// catalog. Refresh just this connection's catalog on its next CLI listing.
	if err := c.catalog.Delete(c.key + ".json"); err != nil {
		debug.Log("[models] invalidate catalog: %v", err)
	}
	return m, nil
}
