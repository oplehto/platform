package errors

import (
	"fmt"
)

const (
	// FailedToGetStorageHost indicate failed to get the storage host
	FailedToGetStorageHost Type = baseErr + iota
	// FailedToGetBucketName indicate failed to get the bucket name
	FailedToGetBucketName
)

var withErrStr = []string{
	"Failed to get the storage host",
	"Failed to get the bucket name",
}

type withErr struct {
	typ Type
	Err error `json:"err"`
}

func (e withErr) Error() string {
	return fmt.Sprintf("%s: %v", e.typ.Reference(), e.Err)
}

func (e withErr) Code() Type {
	return e.typ
}

// newWithError is the generic func to wrap an error into TypedError
func newWithError(typ Type) func(err error) TypedError {
	return func(err error) TypedError {
		if err == nil {
			return nil
		}
		return withErr{
			typ: typ,
			Err: err,
		}
	}
}

// funcs to create a new withErr
// example: tyedErr := NewFailedToGetStorageHost(err)
var (
	NewFailedToGetStorageHost = newWithError(FailedToGetStorageHost)
	NewFailedToGetBucketName  = newWithError(FailedToGetBucketName)
)
