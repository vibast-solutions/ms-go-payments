package provider

import "errors"

var ErrProviderNotSupported = errors.New("provider is not supported")

type Registry struct {
	providers map[int32]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	items := make(map[int32]Provider, len(providers))
	for _, p := range providers {
		items[p.Code()] = p
	}
	return &Registry{providers: items}
}

func (r *Registry) Get(code int32) (Provider, error) {
	provider, ok := r.providers[code]
	if !ok {
		return nil, ErrProviderNotSupported
	}
	return provider, nil
}
