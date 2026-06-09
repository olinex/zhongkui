package utils

import "sync"

type GenericPool[T any] struct {
	pool  sync.Pool
	reset func(*T)
}

func NewGenericPool[T any](reset func(*T)) *GenericPool[T] {
	return &GenericPool[T]{
		reset: reset,
		pool: sync.Pool{
			New: func() any {
				return new(T)
			},
		},
	}
}

func (p *GenericPool[T]) Get() *T {
	return p.pool.Get().(*T)
}

func (p *GenericPool[T]) Put(t *T) {
	if p.reset != nil {
		p.reset(t)
	}
	p.pool.Put(t)
}
