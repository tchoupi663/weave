//go:build crashy

package main

import "log/slog"

// This file is compiled only into the benchmark crash image. The normal
// catalog binary is unchanged.
func init() {
	slog.Error("injected startup failure: catalog crash-loop fixture")
	panic("injected startup failure: catalog crash-loop fixture")
}
