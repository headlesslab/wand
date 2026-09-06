package cdp

import (
	"fmt"
)

// Error of the Response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// Error stdlib interface.
func (e *Error) Error() string {
	return fmt.Sprintf("%v", *e)
}

// Is stdlib interface. Two errors are the same when their code, data and
// message agree; the messages of a destroyed execution context count as one.
func (e Error) Is(target error) bool {
	err, ok := target.(*Error)
	if !ok || e.Code != err.Code || e.Data != err.Data {
		return false
	}
	return e.Message == err.Message ||
		(ctxDestroyedMessages[e.Message] && ctxDestroyedMessages[err.Message])
}

// ErrCtxNotFound type.
var ErrCtxNotFound = &Error{
	Code:    -32000,
	Message: "Cannot find context with specified id",
}

// ErrSessionNotFound type.
var ErrSessionNotFound = &Error{
	Code:    -32001,
	Message: "Session with given id not found.",
}

// ErrSearchSessionNotFound type.
var ErrSearchSessionNotFound = &Error{
	Code:    -32000,
	Message: "No search session with given id found",
}

// ErrCtxDestroyed type. It matches both messages Chrome has used for an
// evaluation interrupted by a navigation: Chromium 128 reports "Execution
// context was destroyed." from V8, Chrome 152 "Inspected target navigated or
// closed" from the DevTools session. ErrCtxNotFound, a stale context id, stays
// a separate error.
var ErrCtxDestroyed = &Error{
	Code:    -32000,
	Message: "Execution context was destroyed.",
}

// ctxDestroyedMessages are the messages ErrCtxDestroyed matches.
var ctxDestroyedMessages = map[string]bool{
	ErrCtxDestroyed.Message:                true,
	"Inspected target navigated or closed": true,
}

// ErrObjNotFound type.
var ErrObjNotFound = &Error{
	Code:    -32000,
	Message: "Could not find object with given id",
}

// ErrNodeNotFoundAtPos type.
var ErrNodeNotFoundAtPos = &Error{
	Code:    -32000,
	Message: "No node found at given location",
}

// ErrNotAttachedToActivePage type.
var ErrNotAttachedToActivePage = &Error{
	Code:    -32000,
	Message: "Not attached to an active page",
}
