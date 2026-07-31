package util

func Ptr[T any](elem T) *T {
	return &elem
}

func PtrTrim[T any](elem *T) (res T) {
	if elem == nil {
		return res
	}

	return *elem
}
