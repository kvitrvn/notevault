//go:build !bindings

package main

func newApplication() (*App, error) {
	return NewApp()
}
