package errors

import (
	"context"
	"fmt"
	"net/http"
)

// HTTPError has an additional HTTPCode method compare to TypedError.
type HTTPError interface {
	TypedError
	HTTPCode() int
}

// HandleHTTP sets the X-Influx-Error and X-Influx-Reference headers on the response,
func HandleHTTP(ctx context.Context, e HTTPError, w http.ResponseWriter) {
	if e == nil {
		return
	}

	w.Header().Set("X-Influx-Error", e.Error())
	w.Header().Set("X-Influx-Reference", fmt.Sprintf("%d", e.Code()))
	w.WriteHeader(e.HTTPCode())
}

const (
	// InternalError indicates an unexpected error condition.
	InternalError Type = baseHTTP + iota
	// MalformedData indicates malformed input, such as unparsable JSON.
	MalformedData
	// InvalidData indicates that data is well-formed, but invalid.
	InvalidData
	// Forbidden indicates a forbidden operation.
	Forbidden
	// NotFound indicate a not found operation.
	NotFound
)

var httpCode = []int{
	http.StatusInternalServerError,
	http.StatusBadRequest,
	http.StatusUnprocessableEntity,
	http.StatusForbidden,
	http.StatusNotFound,
}

var httpStr = []string{
	"Internal Error",
	"Malformed Data",
	"Invalid Data",
	"Forbidden",
	"Not Found",
}

type httpError struct {
	HTTPTyp Type  `json:"http_type"`
	Typ     Type  `json:"code"`
	Err     error `json:"error"`
}

func (e httpError) Code() Type {
	return e.Typ
}

func (e httpError) Error() string {
	if e.Typ/baseConst < caseHTTP {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Typ.Reference(), e.Err)
}

func (e httpError) HTTPCode() int {
	return httpCode[e.HTTPTyp-baseHTTP]
}

func newHTTP(typ Type) func(e error) HTTPError {
	return func(e error) HTTPError {
		he := &httpError{
			HTTPTyp: typ,
			Typ:     typ,
			Err:     e,
		}
		if typedErr, ok := e.(TypedError); ok {
			he.Typ = typedErr.Code()
		}
		return *he
	}
}

// funcs to create new HTTPError
// example: errHTTP := NewInternalError(err)
var (
	NewInternalError = newHTTP(InternalError)
	NewMalformedData = newHTTP(MalformedData)
	NewInvalidData   = newHTTP(InvalidData)
	NewForbidden     = newHTTP(Forbidden)
	NewNotFound      = newHTTP(NotFound)
)
