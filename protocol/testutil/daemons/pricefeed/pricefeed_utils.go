package pricefeed

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

// ErrorMapsEqual is a testing method that takes any two maps of keys to errors and asserts that they have the same
// sets of keys, and that each associated error value has the same rendered message.
func ErrorMapsEqual[K comparable](t *testing.T, expected map[K]error, actual map[K]error) {
	require.Equal(t, len(expected), len(actual))
	for key, expectedError := range expected {
		error, ok := actual[key]
		require.True(t, ok)
		require.EqualError(t, expectedError, error.Error())
	}
}

// ErrorsEqual is a testing method that takes any two slices of errors and asserts that each actual error has
// the same rendered message as the expected error.
func ErrorsEqual(t *testing.T, expected []error, actual []error) {
	require.Equal(t, len(expected), len(actual))
	for i, expectedError := range expected {
		require.EqualError(t, expectedError, actual[i].Error())
	}
}

// ReadJsonTestFile takes a test file with human-readable, formatted JSON, load it, unmarshals and re-marshals it.
// The purpose is to remove the formatting (e.g. newlines, tabs, etc) and return a string that would match an
// unmarshaled object string generated by a Go program natively.
func ReadJsonTestFile(t *testing.T, fileName string) string {
	fileBytes, err := os.ReadFile(fmt.Sprintf("testdata/%v", fileName))
	require.NoError(t, err, "Error reading test file")

	val := map[string]interface{}{}
	err = json.Unmarshal(fileBytes, &val)
	require.NoError(t, err, "Error unmarshalling test file - is it valid JSON?")

	bytes, err := json.Marshal(val)
	require.NoError(t, err, "Error marshalling JSON test file")

	return string(bytes)
}
