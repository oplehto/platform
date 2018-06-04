package errors

import (
	"encoding/json"
	"strconv"
)

// Type of an error.
type Type int

// Reference returns the string value of the error type.
func (typ Type) Reference() string {
	switch typ / baseConst {
	case caseHTTP:
		return httpStr[typ-baseHTTP]
	case caseWithValue:
		return withValueStr[typ-baseWithValue]
	case caseConst:
		return constStr[typ-baseConst]
	default:
		return withErrStr[typ-baseErr]
	}
}

// TypedError wraps error with a reference type.
type TypedError interface {
	// Code returns the integer value of the error type.
	Code() Type
	error
}

const baseErr = 1

// base type integers
const (
	baseConst = 20000 * (1 + iota)
	baseWithValue
	baseHTTP
)

// switch case typ/baseConst
const (
	caseConst = 1 + iota
	caseWithValue
	caseHTTP
)

type marshaler struct {
	Code Type            `json:"code"`
	Raw  json.RawMessage `json:"message"`
}

// MarshalJSON is the method used to embed a cutomized json.Marshaler interface.
func MarshalJSON(typedErr TypedError) ([]byte, error) {
	b, err := json.Marshal(typedErr)
	if err != nil {
		return b, err
	}
	m := marshaler{
		Code: typedErr.Code(),
		Raw:  json.RawMessage(b),
	}
	return json.Marshal(m)
}

// UnmarshalJSON is the method used to embed a cutomized json.Unmarshaler interface.
func UnmarshalJSON(b []byte) (TypedError, error) {
	m := new(marshaler)
	var result TypedError
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}
	switch m.Code / baseConst {
	case caseHTTP:
		result = new(httpError)
	case caseWithValue:
		result = new(withValue)
	case caseConst:
		i, err := strconv.Atoi(string(m.Raw))
		if err != nil {
			return nil, err
		}
		return ConstError(i), nil
	default:
		result = new(withErr)
	}
	err := json.Unmarshal([]byte(m.Raw), result)
	return result, err
}
