package main

func installerWarmup(args []string) bool {
	for _, arg := range args {
		if arg == "--warmup" {
			return true
		}
	}
	return false
}
