package embed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rolivape/hiverod-mcp-go/embeddings"
)

const healthCacheDuration = 30 * time.Second

type Manager struct {
	client    embeddings.Client
	mu        sync.RWMutex
	available bool
	failedAt  time.Time
}

func NewManager(client embeddings.Client) *Manager {
	return &Manager{
		client:    client,
		available: client != nil,
	}
}

func (m *Manager) IsAvailable() bool {
	if m.client == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.available && time.Since(m.failedAt) < healthCacheDuration {
		return false
	}
	return true
}

func (m *Manager) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.client == nil {
		return nil, fmt.Errorf("embedder not configured")
	}
	vec, err := m.client.Embed(ctx, text)
	if err != nil {
		m.markUnavailable()
		return nil, err
	}
	m.markAvailable()
	return vec, nil
}

func (m *Manager) ModelVersion() string {
	if m.client == nil {
		return ""
	}
	return m.client.ModelVersion()
}

func (m *Manager) Dimensions() int {
	if m.client == nil {
		return 0
	}
	return m.client.Dimensions()
}

func (m *Manager) IsHealthy(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("embedder not configured")
	}
	return m.client.IsHealthy(ctx)
}

func (m *Manager) CooldownRemaining() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.available {
		return 0
	}
	remaining := healthCacheDuration - time.Since(m.failedAt)
	if remaining <= 0 {
		return 0
	}
	return int(remaining.Seconds()) + 1
}

func (m *Manager) markUnavailable() {
	m.mu.Lock()
	m.available = false
	m.failedAt = time.Now()
	m.mu.Unlock()
}

func (m *Manager) markAvailable() {
	m.mu.Lock()
	m.available = true
	m.mu.Unlock()
}
