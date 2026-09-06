// Package main is a fake browser that answers --dump-dom the way a headless
// browser does, for the launcher's validation test.
package main

import "fmt"

func main() {
	fmt.Println("<html><head></head><body></body></html>")
}
