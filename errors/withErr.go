package errors

import (
	"fmt"
)

const (
	// FailedToGetStorageHost indicate failed to get the storage host.
	FailedToGetStorageHost Type = baseErr + iota
	// FailedToGetBucketName indicate failed to get the bucket name.
	FailedToGetBucketName
	// JSONInnerErrMarshal indicate errors happened at innerErr mashal.
	JSONInnerErrMarshal
	// JSONMarshal indicate errors happened in json marshal.
	JSONMarshal
	// JSONUnmarshal indicate errors happened in json unmarshal.
	JSONUnmarshal
)

var withErrStr = map[Type]string{
	FailedToGetStorageHost: "Failed to get the storage host",
	FailedToGetBucketName:  "Failed to get the bucket name",
	JSONInnerErrMarshal:    "JSON innerErr Mashal",
	JSONMarshal:            "error happened in JSON marshal",
	JSONUnmarshal:          "error happened in JSON unmarshal",
}

type withErr struct {
	Typ      Type       `json:"code"`
	HasType  bool       `json:"has_type"`
	TypedErr TypedError `json:"typed_err,omitempty"`
	Msg      string     `json:"message"`
}

func (e withErr) Error() string {
	return fmt.Sprintf("%s: %v", e.Typ.Reference(), e.Msg)
}

func (e withErr) Type() Type {
	return e.Typ
}

func (e withErr) InnerErr() TypedError {
	if e.HasType {
		return e.TypedErr
	}
	return nil
}

// newWithError is the generic func to wrap an error into TypedError
func newWithError(typ Type) func(e error) TypedError {
	return func(e error) TypedError {
		if e == nil {
			return nil
		}
		he := &withErr{
			Msg: e.Error(),
			Typ: typ,
		}
		if typedErr, ok := e.(TypedError); ok {
			he.HasType = true
			he.TypedErr = typedErr
		}
		return he
	}
}

// funcs to create a new withErr
// example: tyedErr := NewFailedToGetStorageHost(err)
var (
	NewFailedToGetStorageHost = newWithError(FailedToGetStorageHost)
	NewFailedToGetBucketName  = newWithError(FailedToGetBucketName)
	NewJSONInnerErrMarshal    = newWithError(JSONInnerErrMarshal)
	NewJSONMarshal            = newWithError(JSONMarshal)
	NewJSONUnmarshal          = newWithError(JSONUnmarshal)
)
