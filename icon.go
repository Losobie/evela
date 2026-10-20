package main

import _ "embed"

//go:generate go run ./tools/icon

//go:embed icon.svg
var applicationIcon []byte
