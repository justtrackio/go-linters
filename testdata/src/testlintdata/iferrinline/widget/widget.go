package widget

type Service struct{}

func New() (*Service, error) {
	return &Service{}, nil
}

func With(*Service) (*Service, error) {
	return nil, nil
}
