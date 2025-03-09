package validator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlug(t *testing.T) {

	type Slug struct {
		S string `validate:"slug"`
	}

	testCases := []struct {
		Name     string
		Value    string
		Expected bool
	}{
		{
			Name:     "valid",
			Value:    "valid-slug_123",
			Expected: true,
		},
		{
			Name:     "invalid",
			Value:    "invalid/!?",
			Expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			err := Validate.Struct(&Slug{
				S: tc.Value,
			})

			if tc.Expected {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
