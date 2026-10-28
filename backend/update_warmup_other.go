//go:build !windows

package main

import "context"

func warmUpdateInstaller(context.Context, string) error { return nil }
