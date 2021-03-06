package candiedyaml

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var byteSliceType = reflect.TypeOf([]byte(nil))

var bool_values map[string]bool
var null_values map[string]bool

var signs = []byte{'-', '+'}
var nulls = []byte{'~', 'n', 'N'}
var bools = []byte{'t', 'T', 'f', 'F', 'y', 'Y', 'n', 'N', 'o', 'O'}

var timestamp_regexp *regexp.Regexp
var ymd_regexp *regexp.Regexp

func init() {
	bool_values = make(map[string]bool)
	bool_values["y"] = true
	bool_values["yes"] = true
	bool_values["n"] = false
	bool_values["no"] = false
	bool_values["true"] = true
	bool_values["false"] = false
	bool_values["on"] = true
	bool_values["off"] = false

	null_values = make(map[string]bool)
	null_values["~"] = true
	null_values["null"] = true
	null_values["Null"] = true
	null_values["NULL"] = true
	null_values[""] = true

	timestamp_regexp = regexp.MustCompile("^([0-9][0-9][0-9][0-9])-([0-9][0-9]?)-([0-9][0-9]?)(?:(?:[Tt]|[ \t]+)([0-9][0-9]?):([0-9][0-9]):([0-9][0-9])(?:\\.([0-9]*))?(?:[ \t]*(?:Z|([-+][0-9][0-9]?)(?::([0-9][0-9])?)?))?)?$")
	ymd_regexp = regexp.MustCompile("^([0-9][0-9][0-9][0-9])-([0-9][0-9]?)-([0-9][0-9]?)$")
}

func resolve(event yaml_event_t, v reflect.Value) error {
	val := string(event.value)

	if null_values[val] {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(val)
	case reflect.Bool:
		return resolve_bool(val, v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return resolve_int(val, v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return resolve_uint(val, v)
	case reflect.Float32, reflect.Float64:
		return resolve_float(val, v)
	case reflect.Interface:
		v.Set(reflect.ValueOf(resolveInterface(event)))
	case reflect.Struct:
		return resolve_time(val, v)
	case reflect.Slice:
		if v.Type() != byteSliceType {
			return errors.New("Cannot resolve into " + v.Type().String())
		}
		b := make([]byte, base64.StdEncoding.DecodedLen(len(event.value)))
		n, err := base64.StdEncoding.Decode(b, event.value)
		if err != nil {
			return err
		}

		v.Set(reflect.ValueOf(b[0:n]))
	default:
		return errors.New("Resolve failed for " + v.Kind().String())
	}

	return nil
}

func resolve_bool(val string, v reflect.Value) error {
	b, found := bool_values[strings.ToLower(val)]
	if !found {
		return errors.New("Invalid boolean: " + val)
	}

	v.SetBool(b)
	return nil
}

func resolve_int(val string, v reflect.Value) error {
	val = strings.Replace(val, "_", "", -1)
	var value int64

	sign := int64(1)
	if val[0] == '-' {
		sign = -1
		val = val[1:]
	} else if val[0] == '+' {
		val = val[1:]
	}

	base := 10
	if val == "0" {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	if strings.HasPrefix(val, "0b") {
		base = 2
		val = val[2:]
	} else if strings.HasPrefix(val, "0x") {
		base = 16
		val = val[2:]
	} else if val[0] == '0' {
		base = 8
		val = val[1:]
	} else if strings.Contains(val, ":") {
		digits := strings.Split(val, ":")
		bes := int64(1)
		for j := len(digits) - 1; j >= 0; j-- {
			n, err := strconv.ParseInt(digits[j], 10, 64)
			n *= bes
			if err != nil || v.OverflowInt(n) {
				return errors.New("Integer: " + val)
			}
			value += n
			bes *= 60
		}

		value *= sign
		v.SetInt(value)
		return nil
	}

	value, err := strconv.ParseInt(val, base, 64)
	value *= sign
	if err != nil || v.OverflowInt(value) {
		return errors.New("Integer: " + val)
	}

	v.SetInt(value)
	return nil
}

func resolve_uint(val string, v reflect.Value) error {
	val = strings.Replace(val, "_", "", -1)
	var value uint64

	if val[0] == '-' {
		return errors.New("Unsigned int with negative value: " + val)
	}

	if val[0] == '+' {
		val = val[1:]
	}

	base := 10
	if val == "0" {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	if strings.HasPrefix(val, "0b") {
		base = 2
		val = val[2:]
	} else if strings.HasPrefix(val, "0x") {
		base = 16
		val = val[2:]
	} else if val[0] == '0' {
		base = 8
		val = val[1:]
	} else if strings.Contains(val, ":") {
		digits := strings.Split(val, ":")
		bes := uint64(1)
		for j := len(digits) - 1; j >= 0; j-- {
			n, err := strconv.ParseUint(digits[j], 10, 64)
			n *= bes
			if err != nil || v.OverflowUint(n) {
				return errors.New("Unsigned Integer: " + val)
			}
			value += n
			bes *= 60
		}

		v.SetUint(value)
		return nil
	}

	value, err := strconv.ParseUint(val, base, 64)
	if err != nil || v.OverflowUint(value) {
		return errors.New("Unsigned Integer: " + val)
	}

	v.SetUint(value)
	return nil
}

func resolve_float(val string, v reflect.Value) error {
	val = strings.Replace(val, "_", "", -1)
	var value float64

	sign := 1
	if val[0] == '-' {
		sign = -1
		val = val[1:]
	} else if val[0] == '+' {
		val = val[1:]
	}

	valLower := strings.ToLower(val)
	if valLower == ".inf" {
		value = math.Inf(sign)
	} else if valLower == ".nan" {
		value = math.NaN()
	} else if strings.Contains(val, ":") {
		digits := strings.Split(val, ":")
		bes := float64(1)
		for j := len(digits) - 1; j >= 0; j-- {
			n, err := strconv.ParseFloat(digits[j], v.Type().Bits())
			n *= bes
			if err != nil || v.OverflowFloat(n) {
				return errors.New("Float: " + val)
			}
			value += n
			bes *= 60
		}
		value *= float64(sign)
	} else {
		var err error
		value, err = strconv.ParseFloat(val, v.Type().Bits())
		value *= float64(sign)
		if err != nil || v.OverflowFloat(value) {
			return errors.New("Float: " + val)
		}
	}

	v.SetFloat(value)
	return nil
}

func resolve_time(val string, v reflect.Value) error {
	var parsedTime time.Time
	matches := ymd_regexp.FindStringSubmatch(val)
	if len(matches) > 0 {
		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		parsedTime = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	} else {
		matches = timestamp_regexp.FindStringSubmatch(val)
		if len(matches) == 0 {
			return errors.New("Unexpected timestamp: " + val)
		}

		year, _ := strconv.Atoi(matches[1])
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		hour, _ := strconv.Atoi(matches[4])
		min, _ := strconv.Atoi(matches[5])
		sec, _ := strconv.Atoi(matches[6])

		nsec := 0
		if matches[7] != "" {
			millis, _ := strconv.Atoi(matches[7])
			nsec = int(time.Duration(millis) * time.Millisecond)
		}

		loc := time.UTC
		if matches[8] != "" {
			sign := matches[8][0]
			hr, _ := strconv.Atoi(matches[8][1:])
			min := 0
			if matches[9] != "" {
				min, _ = strconv.Atoi(matches[9])
			}

			zoneOffset := (hr*60 + min) * 60
			if sign == '-' {
				zoneOffset = -zoneOffset
			}

			loc = time.FixedZone("", zoneOffset)
		}
		parsedTime = time.Date(year, time.Month(month), day, hour, min, sec, nsec, loc)
	}

	v.Set(reflect.ValueOf(parsedTime))
	return nil
}

func resolveInterface(event yaml_event_t) interface{} {
	if len(event.value) == 0 {
		return nil
	}

	val := string(event.value)
	if len(event.tag) == 0 && !event.implicit {
		return val
	}

	sign := false
	c := val[0]
	switch {
	case bytes.IndexByte(signs, c) != -1:
		sign = true
		fallthrough
	case c >= '0' && c <= '9':
		i := int64(0)
		if resolve_int(val, reflect.ValueOf(&i).Elem()) == nil {
			return i
		}
		f := float64(0)
		if resolve_float(val, reflect.ValueOf(&f).Elem()) == nil {
			return f
		}

		if !sign {
			t := time.Time{}
			if resolve_time(val, reflect.ValueOf(&t).Elem()) == nil {
				return t
			}
		}
	case bytes.IndexByte(nulls, c) != -1:
		if null_values[val] {
			return nil
		}
		b := false
		if resolve_bool(val, reflect.ValueOf(&b).Elem()) == nil {
			return b
		}
	case c == '.':
		f := float64(0)
		if resolve_float(val, reflect.ValueOf(&f).Elem()) == nil {
			return f
		}
	case bytes.IndexByte(bools, c) != -1:
		b := false
		if resolve_bool(val, reflect.ValueOf(&b).Elem()) == nil {
			return b
		}
	}

	return string(event.value)
}
