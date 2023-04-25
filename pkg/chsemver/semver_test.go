package chsemver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	cases := []struct {
		Input    string
		Expected []string
	}{
		{
			Input:    "1.2.312",
			Expected: []string{"1", "2", "312"},
		},
		{
			Input:    "10.0",
			Expected: []string{"10", "0"},
		},
		{
			Input:    "20",
			Expected: []string{"20"},
		},
		{
			Input:    "1.2.3-alpine",
			Expected: []string{"1", "2", "3", "alpine"},
		},
		{
			Input:    "head",
			Expected: []string{"head"},
		},
		{
			Input:    "latest",
			Expected: []string{"latest"},
		},
	}

	for _, test := range cases {
		t.Run(test.Input, func(t *testing.T) {
			assert.EqualValues(t, Parse(test.Input), test.Expected)
		})
	}
}

func TestIsAtLeastMajor(t *testing.T) {
	cases := []struct {
		Version  string
		Major    string
		Expected bool
	}{
		{
			Version:  "1",
			Major:    "21",
			Expected: false,
		},
		{
			Version:  "1",
			Major:    "21.1",
			Expected: false,
		},
		{
			Version:  "1.28",
			Major:    "21",
			Expected: false,
		},
		{
			Version:  "9.11",
			Major:    "10",
			Expected: false,
		},
		{
			Version:  "21.32",
			Major:    "20",
			Expected: true,
		},
		{
			Version:  "21",
			Major:    "21",
			Expected: true,
		},
	}

	for _, test := range cases {
		t.Run(fmt.Sprintf("%s >= %s?", test.Version, test.Major), func(t *testing.T) {
			assert.EqualValues(t, IsAtLeastMajor(test.Version, test.Major), test.Expected)
		})
	}
}
