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
