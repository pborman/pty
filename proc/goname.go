// Copyright 2011 Google Inc. All rights reserved.
// Author: borman@google.com (Paul Borman)

package proc

import "unicode"

// GoName converts the string s into a public Go identifier.
//
// Leading non-Letter/Digits are stripped.
// If the following rune is an ASCII letter convert it to uppercase
// else prepend an X.
// For all remaining characters, squeeze out non-Letter/Digits and
// make any letter following a non-letter be upper case (if possible).
// (i.e, "$foo_bar" -> "FooBar").
func GoName(s string) string {
	rs := []rune(s) // work in unicode code points
	var g string
	toup := false

Loop:
	// Make sure the result will be a public name
	for x, c := range rs {
		switch {
		case c >= 'A' && c <= 'Z':
			g = string([]rune{c})
			rs = rs[x+1:]
			break Loop
		case c >= 'a' && c <= 'z':
			g = string([]rune{c ^ 040})
			rs = rs[x+1:]
			break Loop
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			g = string([]rune{'X'})
			rs = rs[x:]
			break Loop
		}
	}

	// Squeze out any non letter or digit converting a following
	// lowercase letter to uppercase
	for _, c := range rs {
		switch {
		case toup && unicode.IsLower(c):
			toup = false
			g += string([]rune{unicode.ToUpper(c)})
		case unicode.IsLetter(c):
			g += string([]rune{c})
			toup = false
		case unicode.IsDigit(c):
			g += string([]rune{c})
			toup = true
		default:
			toup = true
		}
	}
	if g == "" {
		return ""
	}
	return g
}
