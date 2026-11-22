//go:build !windows

package main

func restoreTrayInstance() bool { return false }

func listenForTrayRestore(func()) func() { return nil }
