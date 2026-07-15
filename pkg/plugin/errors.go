package plugin

import "strings"

// buildCustomErrorMessage normalizes known dataservice error messages.
func buildCustomErrorMessage(errorMessage string) string {
	if strings.Contains(errorMessage, "Undefined or expired access token") {
		return "Undefined or expired access token"
	}
	return errorMessage
}
