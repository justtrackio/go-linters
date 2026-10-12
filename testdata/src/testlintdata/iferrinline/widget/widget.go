package widget

import "time"

type Service struct{}

func New() (*Service, error) {
	return &Service{}, nil
}

func With(*Service) (*Service, error) {
	return nil, nil
}

func Duration() (time.Duration, error) { return 0, nil }

// Client exposes the full public contract of the private implementation.
type Client interface {
	Exec() error
	Close() error
}
type Querier interface{ Exec() error }
type client struct{}

func (*client) Exec() error           { return nil }
func (*client) Close() error          { return nil }
func ProvideClient() (*client, error) { return &client{}, nil }

type private struct{}

func (*private) Exec() error  { return nil }
func (*private) Extra() error { return nil }

func NewPrivate() (*private, error) { return &private{}, nil }
