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

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// ParseProcFile parses file f storing the results in the matching fields in the
// interface iv.  iv must be a pointer to a structure.  The first word of the
// each line (terminated by whitespace or a :) is converted by GoName.  If the
// resulting name matches a field in iv then the string following is parsed and
// stored into the appropriate field.  The type of the field determines how the
// value is parsed.  Only numeric, string, and numeric slices are supported.
//
// For slices the values are separated by one or more white space or comma
// characters.  (See below for changing the comma to a different value)
//
// Strings have leading and trailing white space stripped.
//
// Numeric values can optionally have a scaling factor after the numeric
// portion.  The known scaling factors are:
//
//	kB: multiply the value by 1024
//	mB: multiply the value by 1024*1024
//
// There are several tags that are understood by ParseProcFile and can be used
// to alter the parsing of the value.
//
// For numbers the base of the number can be expressed as base:"16" Bases 2 - 36
// are supported.  For slices the delimiter can be set with delim:"/"
//
// Example fields:
//
//	Field int     // a base 10 number
//	Field uint    `base:"16"` // a base 16 unsigned number
//	Field []int   `base:"16"` // a slice of base 16 numbers
//	Field []int   `delim:"/"` // a slice of numbers deliminted by /
//	Field string  // a string (trimmed)
//	Field float32 // a 32 bit float
func ParseProcFile(f io.Reader, iv interface{}) error {
	r := bufio.NewReader(f)
	v := reflect.ValueOf(iv).Elem()
	t := v.Type()

	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			if line == "" {
				break
			}
			err = nil
		} else if err != nil {
			return err
		}
		if line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}

		// Get the name and find it in our struct
		// The name must not contain white space or a :
		var x int
		if x = strings.IndexAny(line, ": \t"); x < 1 {
			continue
		}
		tf, ok := t.FieldByName(GoName(line[:x]))
		if !ok {
			continue
		}
		f := v.FieldByIndex(tf.Index)

		line = strings.Trim(line[x+1:], " \t:")

		switch f.Kind() {
		case reflect.Uint:
			line, err = getUint(line, tf, f, strconv.IntSize)
		case reflect.Uint8:
			line, err = getUint(line, tf, f, 8)
		case reflect.Uint16:
			line, err = getUint(line, tf, f, 16)
		case reflect.Uint32:
			line, err = getUint(line, tf, f, 32)
		case reflect.Uint64:
			line, err = getUint(line, tf, f, 64)
		case reflect.Int:
			line, err = getInt(line, tf, f, strconv.IntSize)
		case reflect.Int8:
			line, err = getInt(line, tf, f, 8)
		case reflect.Int16:
			line, err = getInt(line, tf, f, 16)
		case reflect.Int32:
			line, err = getInt(line, tf, f, 32)
		case reflect.Int64:
			line, err = getInt(line, tf, f, 64)
		case reflect.Float32:
			line, err = getFloat(line, tf, f, 32)
		case reflect.Float64:
			line, err = getFloat(line, tf, f, 64)
		case reflect.Slice:
			var a reflect.Value
			switch f.Type().Elem().Kind() {
			case reflect.Uint:
				a, err = getSlice(line, tf, strconv.IntSize, getUint)
			case reflect.Uint8:
				a, err = getSlice(line, tf, 8, getUint)
			case reflect.Uint16:
				a, err = getSlice(line, tf, 16, getUint)
			case reflect.Uint32:
				a, err = getSlice(line, tf, 32, getUint)
			case reflect.Uint64:
				a, err = getSlice(line, tf, 64, getUint)
			case reflect.Int:
				a, err = getSlice(line, tf, strconv.IntSize, getInt)
			case reflect.Int8:
				a, err = getSlice(line, tf, 8, getInt)
			case reflect.Int16:
				a, err = getSlice(line, tf, 16, getInt)
			case reflect.Int32:
				a, err = getSlice(line, tf, 32, getInt)
			case reflect.Int64:
				a, err = getSlice(line, tf, 64, getInt)
			case reflect.Float32:
				a, err = getSlice(line, tf, 32, getFloat)
			case reflect.Float64:
				a, err = getSlice(line, tf, 64, getFloat)
			default:
				err = fmt.Errorf("unsupported slice type: %s", f.Type().Elem().Kind())
			}
			if err == nil {
				f.Set(a)
			}
		case reflect.String:
			f.SetString(line)
			line = ""
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// getInt parses one int from line placing the value in f.  bits specifies the
// number of bits allowed in the number.  t is the StructField that f came from.
// The tag in t is used to determine the base of the number.  the remaining part
// of the line is returned.
func getInt(line string, t reflect.StructField, f reflect.Value, bits int) (string, error) {
	var x int

	base, err := getBase(t)
	if err != nil {
		return line, err
	}
	if x = strings.IndexAny(line, " \t"); x < 0 {
		x = len(line)
	}
	value, err := strconv.ParseInt(line[:x], base, bits)
	if err != nil {
		return "", err
	}

	line = strings.TrimLeft(line[x:], " \t")

	scale, line := getScale(line)
	value *= int64(scale)
	f.SetInt(value)
	return line, nil
}

// getUint parses one unsigned int from line placing the value in f.  bits
// specifies the number of bits allowed in the number.  t is the StructField
// that f came from.  The tag in t is used to determine the base of the number.
// the remaining part of the line is returned.
func getUint(line string, t reflect.StructField, f reflect.Value, bits int) (string, error) {
	var x int

	base, err := getBase(t)
	if err != nil {
		return line, err
	}
	if x = strings.IndexAny(line, " \t"); x < 0 {
		x = len(line)
	}
	value, err := strconv.ParseUint(line[:x], base, bits)
	if err != nil {
		return "", err
	}

	line = strings.TrimLeft(line[x:], " \t")

	scale, line := getScale(line)
	value *= scale
	f.SetUint(value)
	return line, nil
}

// getFloat parses one float from line placing the value in f.  bits specifies
// the number of bits allowed in the number.  t is the StructField that f came
// from.
func getFloat(line string, t reflect.StructField, f reflect.Value, bits int) (string, error) {
	var x int

	if x = strings.IndexAny(line, " \t"); x < 0 {
		x = len(line)
	}

	value, err := strconv.ParseFloat(line[:x], bits)
	if err != nil {
		return "", err
	}

	f.SetFloat(value)
	return strings.TrimLeft(line[x:], " \t"), nil
}

// getSlice parses line as a list of numbers separated by white space or commas
// and returns the result as a reflect.Value slice.  bits specifies the number
// of bits allowed in the number.  Type type of elements in the slice are
// determined by the t, which must reference a field that is a slice.
func getSlice(line string, t reflect.StructField, bits int, get func(string, reflect.StructField, reflect.Value, int) (string, error)) (reflect.Value, error) {
	slice := reflect.New(t.Type).Elem()
	v := reflect.New(t.Type.Elem()).Elem()
	delim := t.Tag.Get("delim")
	if delim == "" {
		delim = " \t,"
	}

	for {
		var x int
		line = strings.TrimLeft(line, " \t"+delim)
		if line == "" {
			return slice, nil
		}
		if x = strings.IndexAny(line, delim); x < 0 {
			x = len(line)
		}
		_, err := get(line[:x], t, v, bits)
		if err != nil {
			return slice, err // slice is ignored
		}
		slice = reflect.Append(slice, v)
		line = line[x:]
	}
}

// getBase reads and returns the base field from the tag in f.  If f has no tag
// then 10 is returned.  Only bases 2-36 are valid.
func getBase(f reflect.StructField) (int, error) {
	tag := f.Tag.Get("base")
	if tag == "" {
		return 10, nil
	}
	ubase, err := strconv.ParseUint(tag, 10, 6)
	if ubase < 2 || ubase > 36 || err != nil {
		err = errors.New("invalid base: " + tag)
	}
	return int(ubase), err
}

// getScale parses line as a scaling suffix (i.e., "kB" or "mB") and returns its
// scale (i.e., 1024 or 1024*1024) and the remaining portion of line.  If line
// is empty or is unrecognized then 1 and line are returned.
func getScale(line string) (uint64, string) {
	switch line {
	case "kB":
		return 1024, ""
	case "mB":
		return 1024 * 1024, ""
	}
	return 1, line
}
