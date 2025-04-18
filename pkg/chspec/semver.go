package chspec

import (
	"strconv"
	"strings"
	"unicode"
)

type Semver []string

// Parse takes clickhouse version and splits it into semver-like representation.
//
// Example:
// Parse("21.1.8") = ["21", "1", "8""]
func Parse(version string) Semver {
	return strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '-' || unicode.IsSpace(r)
	})
}

// IsGreater takes two semver representations and returns true
// if the first operand is greater than the second one.
//
// It can be used as a comparator in a sort-function.
func IsGreater(a, b Semver) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		numA, errA := strconv.ParseUint(a[i], 10, 64)
		numB, errB := strconv.ParseUint(b[i], 10, 64)

		if errA == nil && errB == nil {
			if numA != numB {
				return numA > numB
			}
		} else {
			return a[i] < b[i]
		}
	}

	return len(a) > len(b)
}

// IsAtLeastMajor checks if a database version if at least the given major version.
func IsAtLeastMajor(version, major string) bool {
	if strings.HasPrefix(version, "head") || strings.HasPrefix(version, "latest") {
		return true
	}

	return !IsGreater(Parse(major), Parse(version))
}
