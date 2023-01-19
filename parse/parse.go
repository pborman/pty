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

// Package parse reads lines and parses them into words.
package parse

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"unicode"
)

// A Reader reads lines and parses them into words.  A line is terminated by the
// newline character.  Words are delimited by whitespace as defined by the
// IsSpace function and delimiters as defined by the IsDelim function.  By
// default IsSpace is unicode.IsSpace and delimiters are ';' and '|'.
// Whitespace and newline characters may be include in words by enclosing them
// in quotes.  A quote is determined by the IsQuote function.  By default the '
// and " characters are considered quotes.  A \ and a following newline
// character are discarded (allowing a long line to be broken up).  A \ followed
// by any other character causes the following character to be included in a
// word.
//
// The PS1 element, if not "", is written to os.Stdout before reading the line.
//
// The PS2 element, if not "", is written to os.Stdout after reading a newnlie
// character that does not terminate the line (that is, a newline character
// preceeded by a \ or a newline character within a quoted string)
type Reader struct {
	IsQuote func(rune) bool // function to determine if a rune is a quote
	IsSpace func(rune) bool // function to determine if a rune is whitespace
	IsDelim func(rune) bool // function to determine if a rune is a delimiter
	PS1     string          // optional initial prompt string
	PS2     string          // optional secondary prompt string
	r       *bufio.Reader
}

var (
	EOL = errors.New("EOL")
)

func isQuote(r rune) bool {
	return r == '"' || r == '\''
}

func isDelim(r rune) bool {
	return r == '|' || r == ';'
}

// Line parses in into words and returns them, or an error.  If in contains
// a newline then parsing will stop on the newline.
func Line(in string) ([]string, error) {
	r := NewReader(bytes.NewBufferString(in))
	return r.Read()
}

// NewReader returns a new Reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		IsQuote: isQuote,
		IsSpace: unicode.IsSpace,
		IsDelim: isDelim,
		r:       bufio.NewReader(r),
	}
}

func (r *Reader) matchQuote(word *bytes.Buffer, quote rune) error {
	for {
		c, _, err := r.r.ReadRune()
		if err != nil {
			return err
		}
		switch c {
		case quote:
			return nil

		case '\\':
			c, _, err = r.r.ReadRune()
			if err != nil {
				return err
			}
			if c != '\n' {
				word.WriteRune(c)
			}
		default:
			word.WriteRune(c)
		}
		if c == '\n' && r.PS2 != "" {
			os.Stdout.Write([]byte(r.PS2))
		}
	}
	panic("unreachable")
}

func (r *Reader) readWord() (string, rune, error) {
	var c rune
	var err error
	// Skip leading white space
	for {
		c, _, err = r.r.ReadRune()
		switch err {
		case nil:
		case io.EOF:
			return "", 0, err
		default:
			return "", 0, err
		}
		// End of the line
		if c == '\n' {
			return "", c, EOL
		}
		if r.IsDelim(c) {
			return string(c), 0, nil
		}
		if !r.IsSpace(c) {
			break
		}
	}

	// We now have a word of some sort.

	word := &bytes.Buffer{}
	for {
		switch {
		case err != nil:
			return word.String(), 0, err
		case r.IsQuote(c):
			err := r.matchQuote(word, c)
			if err != nil {
				if word.Len() > 0 {
					return word.String(), 0, nil
				}
				return "", 0, err
			}
		case r.IsSpace(c):
			return word.String(), c, nil
		case r.IsDelim(c):
			return word.String(), c, nil
		case c == '\\':
			c, _, err = r.r.ReadRune()
			if err == io.EOF {
				word.WriteRune(c)
				return word.String(), 0, nil
			}
			if err != nil {
				return "", 0, err
			}
			if c == '\n' {
				os.Stdout.Write([]byte(r.PS2))
			} else {
				word.WriteRune(c)
			}
		default:
			word.WriteRune(c)
		}
		c, _, err = r.r.ReadRune()
	}
}

// Read reads one line of words from r.
//
// If some words were read and an error other than EOF is read then the
// words read are returned along with the error that terminated the line.
//
// On end of file both words and err are nil.
func (r *Reader) Read() (words []string, err error) {
	if r.PS1 != "" {
		os.Stdout.Write([]byte(r.PS1))
	}
	for {
		word, delim, err := r.readWord()
		if err == nil || word != "" {
			words = append(words, word)
		}
		if err == EOL || err == io.EOF {
			return words, nil
		}
		if err != nil {
			return words, err
		}
		if delim == '\n' {
			return words, nil
		}
		if r.IsDelim(delim) {
			words = append(words, string(delim))
		}
	}
	panic("unreachable")
}

/*
func main() {
	r := NewReader(os.Stdin)
	r.PS1 = "$ "
	r.PS2 = "> "
	for {
		words, err := r.Read()
		if err != nil {
			fmt.Println(err)
			return
		}
		if words == nil {
			return
		}
		fmt.Printf("%d:", len(words))
		for _, w := range words {
			fmt.Printf(" ``%s''", w)
		}
		fmt.Printf("\n")
	}
}
*/
