package util

import "unicode/utf8"

func RuneCount(s string) int {
	return utf8.RuneCountInString(s)
}
