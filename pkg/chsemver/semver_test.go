package chsemver

import (
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
