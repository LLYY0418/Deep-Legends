//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("This standalone probe must run on Windows. Unit tests can run on this platform.")
}
