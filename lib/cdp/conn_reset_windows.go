//go:build windows

package cdp

import "syscall"

// errConnReset is the error a read returns when the peer has reset the
// connection, as Windows reports it.
var errConnReset error = syscall.WSAECONNRESET
