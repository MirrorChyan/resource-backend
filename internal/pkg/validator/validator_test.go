package validator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlug(t *testing.T) {

	testCases := []struct {
		Name     string
		Slug     string
		Expected bool
	}{
		{
			Name:     "valid",
			Slug:     "valid-slug_123",
			Expected: true,
		},
		{
			Name:     "invalid",
			Slug:     "invalid/!?",
			Expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			err := Validate.Var(tc.Slug, "slug")

			if tc.Expected {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
