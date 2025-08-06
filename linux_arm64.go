//go:build linux && arm64 && !musl
// +build linux,arm64,!musl

package gozstd

/*
#cgo LDFLAGS: ${SRCDIR}/cgo/lib/linux_arm64.a
*/
import "C"
