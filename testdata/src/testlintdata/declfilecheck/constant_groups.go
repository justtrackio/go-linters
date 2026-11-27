package declfilecheck

const (
	metricCount                = 1
	metricSize, metricDuration = 2, 3
	thresholdValue             = 4 // want "const thresholdValue is used only by client.go"
	metricTail                 = 5 // want "const metricTail is used only by client.go"
)

const (
	optionMin = 6 // want "const optionMin is used only by client.go"
	optionMax = 7 // want "const optionMax is used only by client.go"
)

const (
	flagDefault  = 8
	FlagOverride = 9
)
