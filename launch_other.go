//go:build !windows

package main

// На Mac и Linux программу запускают из терминала или как .app — окно консоли само не закрывается.
func startedByDoubleClick() bool { return false }
