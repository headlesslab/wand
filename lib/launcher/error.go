package launcher

import "errors"

// ErrAlreadyLaunched is an error that indicates the launcher has already been launched.
var ErrAlreadyLaunched = errors.New("already launched")

// ErrNoBrowser is the error of a Browser resolution that found nothing: no
// explicit path, no System browser, no Managed browser in the cache and no
// download, either switched off or failed. The error wrapping it lists every
// step tried.
var ErrNoBrowser = errors.New("[launcher] no browser found")
