//go:build !unix

package main

// dropPrivileges is a no-op where there are no Unix users (local development on Windows).
func dropPrivileges(string) error { return nil }
