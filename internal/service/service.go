package service

import (
	"context"

	"wallet-transfer-service/internal/repository"
)

type Service struct {
	store        *repository.Store
	beforeCommit func(context.Context) error
}

type Option func(*Service)

// WithBeforeCommit installs a test hook that runs after writes and before COMMIT.
// A non-nil error rolls the transaction back. Production does not set it.
func WithBeforeCommit(fn func(context.Context) error) Option {
	return func(s *Service) {
		s.beforeCommit = fn
	}
}

func New(store *repository.Store, opts ...Option) *Service {
	s := &Service{store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
