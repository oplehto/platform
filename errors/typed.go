package errors

import (
	"encoding/json"
)

// Type of an error.
type Type int

// Reference returns the string value of the error type.
func (typ Type) Reference() string {
	switch typ / baseConst {
	case caseHTTP:
		return httpStrMap[typ]
	case caseWithValue:
		return withValueStr[typ]
	case caseConst:
		return constStrMap[ConstError(typ)]
	default:
		return withErrStr[typ]
	}
}

// TypedError wraps error with a reference type.
type TypedError interface {
	// Type returns the integer value of the error type.
	Type() Type
	InnerErr() TypedError
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
	Code    Type   `json:"code"`
	Message string `json:"message"`
	// place another marshaler to do recursive deep unmarshal.
	Marshaler *marshaler       `json:"embed,omitempty"`
	Raw       *json.RawMessage `json:"raw,omitempty"`
}

// MarshalJSON is the method used to embed a cutomized json.Marshaler interface.
func MarshalJSON(typedErr TypedError) ([]byte, TypedError) {
	if typedErr == nil {
		return []byte{}, nil
	}
	var b []byte
	var err error
	var raw json.RawMessage
	m := &marshaler{
		Code:    typedErr.Type(),
		Message: typedErr.Error(),
	}

	if typedErr.InnerErr() == nil {
		if b, err = json.Marshal(typedErr); err != nil {
			return b, NewJSONInnerErrMarshal(err)
		}
		raw = json.RawMessage(b)
		m.Raw = &raw
		goto returnMarshal
	}

	if b, err = MarshalJSON(typedErr.InnerErr()); err != nil {
		return b, NewJSONMarshal(err)
	}
	raw = json.RawMessage(b)
	m.Marshaler = &marshaler{
		Code:    typedErr.InnerErr().Type(),
		Message: typedErr.InnerErr().Error(),
		Raw:     &raw,
	}
returnMarshal:
	result, err := json.Marshal(m)
	return result, NewJSONMarshal(err)
}

// UnmarshalJSON is the method used to embed a cutomized json.Unmarshaler interface.
func UnmarshalJSON(b []byte) (e, errTyped TypedError) {
	m := new(marshaler)
	err := json.Unmarshal(b, m)
	if err != nil {
		return nil, NewJSONUnmarshal(err)
	}
	switch m.Code / baseConst {
	case caseWithValue:
		item := new(withValue)
		err = json.Unmarshal(*m.Raw, item)
		return errWithValue(m.Code)(item.Values...), NewJSONUnmarshal(err)
	case caseConst:
		return ConstError(m.Code), nil
	case caseHTTP:
		item := new(httpError)
		if m.Raw != nil {
			// no type
			err = json.Unmarshal(*m.Raw, item)
			return item, NewJSONUnmarshal(err)
		}
		if m.Marshaler != nil {
			item.TypedErr, err = UnmarshalJSON(*m.Marshaler.Raw)
			if err != nil {
				return nil, NewJSONUnmarshal(err)
			}
			item.HTTPTyp = m.Code
			item.Msg = item.TypedErr.Error()
			item.HasType = true
		}
		return item, NewJSONUnmarshal(err)
	default:
		item := new(withErr)
		if m.Raw != nil {
			// no type
			err = json.Unmarshal(*m.Raw, item)
			return item, NewJSONUnmarshal(err)
		}
		if m.Marshaler != nil {
			item.TypedErr, err = UnmarshalJSON(*m.Marshaler.Raw)
			if err != nil {
				return nil, NewJSONUnmarshal(err)
			}
			item.Typ = m.Code
			item.Msg = item.TypedErr.Error()
			item.HasType = true
		}
		return item, NewJSONUnmarshal(err)
	}

}
