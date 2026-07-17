package store

import (
	"fmt"
	"strconv"
	"strings"
)

// Assay versions use the scheme "vMAJOR.MINOR" (e.g. v0.1, v1.0). New lineages
// start at v0.1. Versions are system-generated, never entered by hand.

// Bump kinds accepted by SaveNewVersion.
const (
	BumpMinor = "minor"
	BumpMajor = "major"
)

const initialVersion = "v0.1"

func parseVersion(s string) (major, minor int, err error) {
	if !strings.HasPrefix(s, "v") {
		return 0, 0, fmt.Errorf("version %q missing 'v' prefix", s)
	}
	parts := strings.SplitN(s[1:], ".", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("version %q is not vMAJOR.MINOR", s)
	}
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, fmt.Errorf("version %q: bad major: %w", s, err)
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, fmt.Errorf("version %q: bad minor: %w", s, err)
	}
	return major, minor, nil
}

func formatVersion(major, minor int) string {
	return fmt.Sprintf("v%d.%d", major, minor)
}

// bumpVersion returns the next version after latest for the given bump kind.
// A minor bump increments the minor component; a major bump increments the
// major component and resets minor to 0.
func bumpVersion(latest, bump string) (string, error) {
	major, minor, err := parseVersion(latest)
	if err != nil {
		return "", err
	}
	switch bump {
	case BumpMajor:
		return formatVersion(major+1, 0), nil
	case BumpMinor, "":
		return formatVersion(major, minor+1), nil
	default:
		return "", fmt.Errorf("unknown version bump %q", bump)
	}
}

// versionLess reports whether version a sorts before version b. Unparseable
// versions sort first.
func versionLess(a, b string) bool {
	aMaj, aMin, aErr := parseVersion(a)
	bMaj, bMin, bErr := parseVersion(b)
	if aErr != nil || bErr != nil {
		return aErr != nil && bErr == nil
	}
	if aMaj != bMaj {
		return aMaj < bMaj
	}
	return aMin < bMin
}
