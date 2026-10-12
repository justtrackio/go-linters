package inlineunsafe

import "testlintdata/iferrinline/widget"

func packageShadowError() error {
	widget, err := widget.New() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = widget
	return nil
}

func newService() (*widget.Service, error) { return nil, nil }

func shadowedTypeQualifier() error {
	widget := struct{}{}
	value, err := newService() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	_ = widget
	return nil
}

func privateConcreteType() error {
	value, err := widget.NewPrivate() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	return nil
}

var event string

func lookupEvent(string) (string, bool) { return "", true }

func packageShadowOk() string {
	event, ok := lookupEvent(event)
	if !ok {
		return ""
	}
	return event
}

func foreignAnonymousInterface() error {
	value, err := widget.PrivateInterface() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	return nil
}

func foreignAnonymousStruct() error {
	value, err := widget.PrivateStruct() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	return nil
}
