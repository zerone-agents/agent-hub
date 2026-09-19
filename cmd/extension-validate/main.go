package main

import (
	"fmt"
	"os"

	"control-panel/internal/extensionmanifest"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: extension-validate <capability-package-directory>")
		os.Exit(2)
	}
	report := extensionmanifest.ValidatePackage(os.Args[1])
	if !report.Valid {
		fmt.Fprintf(os.Stderr, "invalid capability package: %s\n", report.Manifest)
		for _, problem := range report.Errors {
			fmt.Fprintf(os.Stderr, "- %s\n", problem)
		}
		os.Exit(1)
	}
	fmt.Printf("valid capability package: %s\n", report.Manifest)
}
