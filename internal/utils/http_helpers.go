package utils

import (
	"regexp"
	"strings"
)

// IsURL returns true if the given string appears to be a URL
func IsURL(str string) bool {
	str = strings.ToLower(str)
	if strings.HasPrefix(str, "http://") || strings.HasPrefix(str, "https://") {
		return true
	}

	// Additional check with regex for more complex cases
	urlPattern := regexp.MustCompile(`^(https?|ftp)://[^\s/$.?#].[^\s]*$`)
	return urlPattern.MatchString(str)
}
