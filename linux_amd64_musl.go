//go:build linux && amd64 && musl
// +build linux,amd64,musl

package gozstd

/*
#cgo LDFLAGS: ${SRCDIR}/cgo/lib/linux_amd64_musl.a
*/
import "C"
