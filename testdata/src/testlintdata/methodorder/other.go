package methodorder

type Elsewhere struct{}

func (h Helpers[T]) otherCrossFileCaller() { h.crossFileHelper() }
