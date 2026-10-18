//go:build !windows

package main

import "fmt"

func sealESI([]byte) ([]byte, error) {
	return nil, fmt.Errorf("secure ESI connection storage currently requires Windows")
}
func unsealESI([]byte) ([]byte, error) {
	return nil, fmt.Errorf("secure ESI connection storage currently requires Windows")
}
