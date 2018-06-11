package http

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// DecodeParams decodes the http query parameters into a struct using reflection.
func DecodeParams(value interface{}, req *http.Request) error {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("value must be an underlying kind of struct")
	}

	// Ensure the form is parsed.
	if err := req.ParseForm(); err != nil {
		return err
	}

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}

		var name string
		typ := v.Type().Field(i)
		if value, ok := typ.Tag.Lookup("json"); ok {
			name = strings.Split(value, ";")[0]
		} else {
			name = toSnakeCase(typ.Name)
		}

		switch f.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			values := req.Form[name]
			if len(values) >= 1 {
				i, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil {
					return err
				} else if f.OverflowInt(i) {
					return fmt.Errorf("value %d will overflow the integer type", i)
				}
				f.SetInt(i)
			}
		case reflect.String:
			values := req.Form[name]
			if len(values) >= 1 {
				f.SetString(values[0])
			}
		}
	}
	return nil
}

func toSnakeCase(name string) string {
	var buf bytes.Buffer
	for _, ch := range name {
		if unicode.IsUpper(ch) {
			if buf.Len() > 0 {
				buf.WriteRune('_')
			}
			buf.WriteRune(unicode.ToLower(ch))
			continue
		}
		buf.WriteRune(ch)
	}
	return buf.String()
}
