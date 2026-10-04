package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/* ExternalCapability is a host-registered extension, with trusted read-only classification. */
type ExternalCapability struct {
	Name     string
	ReadOnly bool
}

/* SessionServices owns external connections independently of individual runs. */
type SessionServices interface {
	Capabilities() []ExternalCapability
	Reconnect(context.Context, string) error
	Close(context.Context) error
}

/* WithExternalCapabilities derives a profile copy before caller restrictions apply. */
func (p Profile) WithExternalCapabilities(capabilities []ExternalCapability) Profile {
	p.toolCeiling = cloneToolNames(p.toolCeiling)
	p.readOnlyTools = cloneToolNames(p.readOnlyTools)
	for _, capability := range capabilities {
		p.toolCeiling = append(p.toolCeiling, capability.Name)
		if capability.ReadOnly {
			p.readOnlyTools = append(p.readOnlyTools, capability.Name)
		}
	}
	return p
}

func (s *AgentSession) initializeServices(ctx context.Context) error {
	if s.dependencies.NewSessionServices == nil {
		return nil
	}
	services, err := s.dependencies.NewSessionServices(ctx, s.Snapshot())
	if err != nil {
		if !isNilRuntimeDependency(services) {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = errors.Join(err, services.Close(cleanup))
			cancel()
		}
		s.stateMu.Lock()
		s.closed = true
		s.stateMu.Unlock()
		s.releaseLease()
		return fmt.Errorf("initialize session services: %w", err)
	}
	if !isNilRuntimeDependency(services) {
		s.services = services
	}
	return nil
}

/* ReconnectServices serializes explicit reconnect with session run admission. */
func (s *AgentSession) ReconnectServices(ctx context.Context, name string) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.services == nil {
		return fmt.Errorf("session has no external services")
	}
	return s.services.Reconnect(ctx, name)
}

/* Services exposes the session's external service port for composition and management. */
func (s *AgentSession) Services() SessionServices { return s.services }
