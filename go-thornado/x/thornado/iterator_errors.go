package thornado

import "strings"

func shouldLogIteratorError(err error) bool {
	if err == nil {
		return false
	}
	return !strings.Contains(err.Error(), "invalid cacheMergeIterator")
}
