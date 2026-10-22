//go:build cgo && linux

package search

/*
#include <malloc.h>
*/
import "C"

func releaseMemory() {
	C.malloc_trim(0)
}
