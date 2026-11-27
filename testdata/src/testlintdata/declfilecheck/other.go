package declfilecheck

func Other() {
	_ = shared{}
	_ = otherUser{}
	_ = pairB
	_ = sharedSecond
	_ = pullConcurrency
	_ = explicitIotaSecond
	_ = implicitSecond
	_ = constPairB
}

func OtherConstants() int {
	return metricDuration
}
