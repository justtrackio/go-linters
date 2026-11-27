package constlocal

const (
	metricFoo     = 1
	metricBar     = 2
	MetricBaz     = 3
	metricsValue  = 4 // want "constant metricsValue is used only by function PackageGroups"
	multiplyValue = 5 // want "constant multiplyValue is used only by function PackageGroups"
	metricTail    = 6 // want "constant metricTail is used only by function PackageGroups"
)

const (
	counterFoo     = 1
	CounterBar     = 2
	gaugeFoo       = 3
	gaugeBar   int = 4
)

func PackageGroups() int {
	return metricFoo + metricBar + MetricBaz + metricsValue + multiplyValue + metricTail +
		counterFoo + CounterBar + gaugeFoo + gaugeBar
}

func LocalGroups() int {
	const (
		httpGet, HTTPPost = 1, 2
		httpDelete        = 3
		bitmapStart       = 4 // want "local constant bitmapStart is used once"
		_                 = 5
		bitmapEnd         = 6 // want "local constant bitmapEnd is used once"
	)
	const bitmapSize = 7 // want "local constant bitmapSize is used once"
	const (
		_            = 8
		_BitmapSaved = 9 // want "local constant _BitmapSaved is used once"
	)
	return httpGet + HTTPPost + httpDelete + bitmapStart + bitmapEnd + bitmapSize + _BitmapSaved
}
