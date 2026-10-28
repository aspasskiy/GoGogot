//go:build !unix

package terminal

func termWidth() int { return 80 }
