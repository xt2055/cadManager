//go:build !windows

package converter

import "context"

func watchCaxaFontDialogs(context.Context) func() { return func() {} }
