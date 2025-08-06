//go:build darwin && amd64 && !ios
// +build darwin,amd64,!ios

package gozstd

/*
#cgo LDFLAGS: ${SRCDIR}/cgo/lib/darwin_amd64.a
*/
import "C"
