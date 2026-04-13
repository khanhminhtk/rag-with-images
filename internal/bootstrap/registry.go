package bootstrap

import (
	"fmt"
	"strings"
)

type Registry struct {
	container *DIContainer
	namespace string
}

func NewRegistry(container *DIContainer, namespace string) *Registry {
	return &Registry{
		container: container,
		namespace: strings.TrimSpace(namespace),
	}
}

func (r *Registry) Key(parts ...string) string {
	chunks := make([]string, 0, len(parts)+1)
	if r != nil && strings.TrimSpace(r.namespace) != "" {
		chunks = append(chunks, strings.TrimSpace(r.namespace))
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		chunks = append(chunks, p)
	}
	return strings.Join(chunks, ".")
}

func (r *Registry) RegisterSingleton(key string, constructor Constructor) error {
	if r == nil || r.container == nil {
		return fmt.Errorf("internal.bootstrap.Registry.RegisterSingleton container is nil for key %s", key)
	}
	return r.container.RegisterSingleton(strings.TrimSpace(key), constructor)
}

func (r *Registry) RegisterScoped(key string, constructor Constructor) error {
	if r == nil || r.container == nil {
		return fmt.Errorf("internal.bootstrap.Registry.RegisterScoped container is nil for key %s", key)
	}
	return r.container.RegisterScoped(strings.TrimSpace(key), constructor)
}

func (r *Registry) RegisterTransient(key string, constructor Constructor) error {
	if r == nil || r.container == nil {
		return fmt.Errorf("internal.bootstrap.Registry.RegisterTransient container is nil for key %s", key)
	}
	return r.container.RegisterTransient(strings.TrimSpace(key), constructor)
}

func ResolveAs[T any](resolver Resolver, key string) (T, error) {
	var zero T
	if resolver == nil {
		return zero, fmt.Errorf("internal.bootstrap.ResolveAs resolver is nil for key %s", key)
	}
	raw, err := resolver.Resolve(strings.TrimSpace(key))
	if err != nil {
		return zero, err
	}
	typed, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("internal.bootstrap.ResolveAs invalid type for key %s", key)
	}
	return typed, nil
}

func MustResolveAs[T any](resolver Resolver, key string) T {
	v, err := ResolveAs[T](resolver, key)
	if err != nil {
		panic(err)
	}
	return v
}
