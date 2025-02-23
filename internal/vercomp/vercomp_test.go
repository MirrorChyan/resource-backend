package vercomp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// generateEqualTestCases generates test cases for equal version comparisons
func generateEqualTestCases(versions []string) []struct {
	Name               string
	Ver1               string
	Ver2               string
	ExpectedComparable bool
	ExpectedResult     int
} {
	var testCases []struct {
		Name               string
		Ver1               string
		Ver2               string
		ExpectedComparable bool
		ExpectedResult     int
	}
	for _, ver := range versions {
		testCases = append(testCases, struct {
			Name               string
			Ver1               string
			Ver2               string
			ExpectedComparable bool
			ExpectedResult     int
		}{
			Name:               "Compare_" + ver + "_Equals_" + ver,
			Ver1:               ver,
			Ver2:               ver,
			ExpectedComparable: true,
			ExpectedResult:     Equal,
		})
	}
	return testCases
}

// generateLessTestCases generates test cases for less-than version comparisons
func generateLessTestCases(lessPairs [][]string) []struct {
	Name               string
	Ver1               string
	Ver2               string
	ExpectedComparable bool
	ExpectedResult     int
} {
	var testCases []struct {
		Name               string
		Ver1               string
		Ver2               string
		ExpectedComparable bool
		ExpectedResult     int
	}
	for _, pair := range lessPairs {
		ver1, ver2 := pair[0], pair[1]
		testCases = append(testCases, struct {
			Name               string
			Ver1               string
			Ver2               string
			ExpectedComparable bool
			ExpectedResult     int
		}{
			Name:               "Compare_" + ver1 + "_Less_Than_" + ver2,
			Ver1:               ver1,
			Ver2:               ver2,
			ExpectedComparable: true,
			ExpectedResult:     Less,
		})
	}
	return testCases
}

// generateGreaterTestCases generates test cases for greater-than version comparisons
func generateGreaterTestCases(greaterPairs [][]string) []struct {
	Name               string
	Ver1               string
	Ver2               string
	ExpectedComparable bool
	ExpectedResult     int
} {
	var testCases []struct {
		Name               string
		Ver1               string
		Ver2               string
		ExpectedComparable bool
		ExpectedResult     int
	}
	for _, pair := range greaterPairs {
		ver1, ver2 := pair[0], pair[1]
		testCases = append(testCases, struct {
			Name               string
			Ver1               string
			Ver2               string
			ExpectedComparable bool
			ExpectedResult     int
		}{
			Name:               "Compare_" + ver1 + "_Greater_Than_" + ver2,
			Ver1:               ver1,
			Ver2:               ver2,
			ExpectedComparable: true,
			ExpectedResult:     Greater,
		})
	}
	return testCases
}

// generateInvalidTestCases generates test cases for invalid version comparisons
func generateInvalidTestCases(invalidPairs [][]string) []struct {
	Name               string
	Ver1               string
	Ver2               string
	ExpectedComparable bool
	ExpectedResult     int
} {
	var testCases []struct {
		Name               string
		Ver1               string
		Ver2               string
		ExpectedComparable bool
		ExpectedResult     int
	}
	for _, pair := range invalidPairs {
		ver1, ver2 := pair[0], pair[1]
		testCases = append(testCases, struct {
			Name               string
			Ver1               string
			Ver2               string
			ExpectedComparable bool
			ExpectedResult     int
		}{
			Name:               "Compare_" + ver1 + "_Invalid_" + ver2,
			Ver1:               ver1,
			Ver2:               ver2,
			ExpectedComparable: false,
			ExpectedResult:     Invalid,
		})
	}
	return testCases
}

func TestVersionComparatorCompare(t *testing.T) {
	equalVersions := []string{
		"1.0.0",
		"1.0.0-beta",
		"v1.0.0",
		"v1.0.0-beta",
		"20250204114023",
		"v20250204114023",
	}

	lessPairs := [][]string{
		{"1.0.0", "1.0.1"},
		{"1.0.0-beta", "1.0.0"},
		{"v1.0.0", "v1.0.1"},
		{"v1.0.0-beta", "v1.0.0"},
		{"20250204114020", "20250204114023"},
		{"v20250204114020", "v20250204114023"},
	}

	greaterPairs := [][]string{
		{"1.0.1", "1.0.0"},
		{"1.0.0", "1.0.0-beta"},
		{"v1.0.1", "v1.0.0"},
		{"v1.0.0", "v1.0.0-beta"},
		{"20250204114023", "20250204114020"},
		{"v20250204114023", "v20250204114020"},
	}

	invalidPairs := [][]string{
		{"1.0.0", "20250204114023"},
		{"v1.0.0", "v20250204114023"},
	}

	var testCases []struct {
		Name               string
		Ver1               string
		Ver2               string
		ExpectedComparable bool
		ExpectedResult     int
	}
	testCases = append(testCases, generateEqualTestCases(equalVersions)...)
	testCases = append(testCases, generateLessTestCases(lessPairs)...)
	testCases = append(testCases, generateGreaterTestCases(greaterPairs)...)
	testCases = append(testCases, generateInvalidTestCases(invalidPairs)...)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			comparator := NewComparator()
			ret := comparator.Compare(tc.Ver1, tc.Ver2)
			require.Equal(t, tc.ExpectedComparable, ret.Comparable)
			require.Equal(t, tc.ExpectedResult, ret.Result)
		})
	}
}
