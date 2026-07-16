package main

import "fmt"

// Version is the authoritative version of AssayManager (semantic versioning).
//
// Bump rules:
//   - PATCH: any agent that changes code bumps this.
//   - MINOR / MAJOR: humans only, on explicit request.
//
// Keep this in sync with the latest entry in CHANGELOG.md.
const Version = "0.1.0"

func main() {
	fmt.Printf("AssayManager %s\n", Version)
}
