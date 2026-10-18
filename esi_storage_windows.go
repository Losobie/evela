//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI binds saved refresh/access tokens to the current Windows user.
func sealESI(data []byte) ([]byte, error)   { return cryptESI(data, true) }
func unsealESI(data []byte) ([]byte, error) { return cryptESI(data, false) }
func cryptESI(data []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	input := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	} else {
		err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	return append([]byte(nil), unsafe.Slice(output.Data, int(output.Size))...), nil
}
