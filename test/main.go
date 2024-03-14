package main

import (
	"fmt"
	"path/filepath"
)

func main() {
	oldPath := "files/test1.txt"

	// Extract filename from the old path
	oldFilename := filepath.Base(oldPath)

	// New path in the current directory with the same filename
	fmt.Print(oldFilename)
}
