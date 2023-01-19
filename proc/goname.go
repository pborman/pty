//   Copyright 2023 Paul Borman
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

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
