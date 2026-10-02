package main

type Service struct{}

func main()          { shared() }
func (Service) Run() { shared(); exclusive() }

func shared()     { sharedLeaf() }
func sharedLeaf() {}
func exclusive()  {} // want "function exclusive is used only by Service"
