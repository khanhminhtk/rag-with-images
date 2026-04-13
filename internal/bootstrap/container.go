package bootstrap

import (
	"errors"
	"fmt"
	"sync"
)

type Lifetime int

const (
	LifetimeSingleton Lifetime = iota
	LifetimeScoped
	LifetimeTransient
)

type Resolver interface {
	Resolve(name string) (any, error)
	MustResolve(name string) any
}

type Constructor func(Resolver) (any, error)

type Scope struct {
	container *DIContainer
	parent    *Scope

	mu        sync.RWMutex
	instances map[string]any
	resolving map[string]bool
}

type registration struct {
	name        string
	lifetime    Lifetime
	constructor Constructor
}

type DIContainer struct {
	mu            sync.RWMutex
	registrations map[string]*registration
	root          *Scope
}

func NewContainer() *DIContainer {
	c := &DIContainer{
		registrations: map[string]*registration{},
	}
	c.root = &Scope{
		container: c,
		parent:    nil,
		instances: map[string]any{},
		resolving: map[string]bool{},
	}
	return c
}

func (c *DIContainer) Root() *Scope {
	return c.root
}

func (c *DIContainer) NewScope() *Scope {
	return &Scope{
		container: c,
		parent:    c.root,
		instances: map[string]any{},
		resolving: map[string]bool{},
	}
}

func (c *DIContainer) Register(name string, lifetime Lifetime, constructor Constructor) error {
	if c == nil {
		return errors.New("internal.bootstrap.Register container is nil")
	}
	if name == "" {
		return errors.New("internal.bootstrap.Register name is empty")
	}
	if constructor == nil {
		return fmt.Errorf("internal.bootstrap.Register constructor is nil: %s", name)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.registrations[name]; exists {
		return fmt.Errorf("internal.bootstrap.Register dependency already registered: %s", name)
	}
	c.registrations[name] = &registration{
		name:        name,
		lifetime:    lifetime,
		constructor: constructor,
	}
	return nil
}

func (c *DIContainer) RegisterSingleton(name string, constructor Constructor) error {
	return c.Register(name, LifetimeSingleton, constructor)
}

func (c *DIContainer) RegisterScoped(name string, constructor Constructor) error {
	return c.Register(name, LifetimeScoped, constructor)
}

func (c *DIContainer) RegisterTransient(name string, constructor Constructor) error {
	return c.Register(name, LifetimeTransient, constructor)
}

func (c *DIContainer) Resolve(name string) (any, error) {
	if c == nil || c.root == nil {
		return nil, errors.New("internal.bootstrap.Resolve container is not initialized")
	}
	return c.root.Resolve(name)
}

func (c *DIContainer) MustResolve(name string) any {
	v, err := c.Resolve(name)
	if err != nil {
		panic(err)
	}
	return v
}

func (s *Scope) Resolve(name string) (any, error) {
	if s == nil || s.container == nil {
		return nil, errors.New("internal.bootstrap.Scope.Resolve scope is not initialized")
	}
	if name == "" {
		return nil, errors.New("internal.bootstrap.Scope.Resolve name is empty")
	}

	reg, err := s.container.findRegistration(name)
	if err != nil {
		return nil, err
	}

	cacheScope := s.cacheScopeForLifetime(reg.lifetime)
	if reg.lifetime != LifetimeTransient {
		if cached, ok := cacheScope.getInstance(name); ok {
			return cached, nil
		}
	}

	if reg.lifetime != LifetimeTransient {
		if err := cacheScope.beginResolving(name); err != nil {
			return nil, err
		}
		defer cacheScope.endResolving(name)
	}

	instance, err := reg.constructor(s)
	if err != nil {
		return nil, fmt.Errorf("internal.bootstrap.Scope.Resolve construct %s failed: %w", name, err)
	}
	if instance == nil {
		return nil, fmt.Errorf("internal.bootstrap.Scope.Resolve construct %s returned nil", name)
	}

	if reg.lifetime != LifetimeTransient {
		cacheScope.setInstance(name, instance)
	}
	return instance, nil
}

func (s *Scope) MustResolve(name string) any {
	v, err := s.Resolve(name)
	if err != nil {
		panic(err)
	}
	return v
}

func (s *Scope) cacheScopeForLifetime(l Lifetime) *Scope {
	switch l {
	case LifetimeSingleton:
		if s.container != nil && s.container.root != nil {
			return s.container.root
		}
		return s
	case LifetimeScoped:
		return s
	default:
		return s
	}
}

func (s *Scope) getInstance(name string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.instances[name]
	return v, ok
}

func (s *Scope) setInstance(name string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[name] = value
}

func (s *Scope) beginResolving(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolving[name] {
		return fmt.Errorf("internal.bootstrap.Scope.Resolve circular dependency detected: %s", name)
	}
	s.resolving[name] = true
	return nil
}

func (s *Scope) endResolving(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resolving, name)
}

func (c *DIContainer) findRegistration(name string) (*registration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	reg, ok := c.registrations[name]
	if !ok {
		return nil, fmt.Errorf("internal.bootstrap.Resolve dependency is not registered: %s", name)
	}
	return reg, nil
}
