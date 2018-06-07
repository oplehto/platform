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
	w.Header().Set("X-Influx-Reference", fmt.Sprintf("%d", e.Type()))
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

var httpCode = map[Type]int{
	InternalError: http.StatusInternalServerError,
	MalformedData: http.StatusBadRequest,
	InvalidData:   http.StatusUnprocessableEntity,
	Forbidden:     http.StatusForbidden,
	NotFound:      http.StatusNotFound,
}

var httpStrMap = map[Type]string{
	InternalError: "Internal Error",
	MalformedData: "Malformed Data",
	InvalidData:   "Invalid Data",
	Forbidden:     "Forbidden",
	NotFound:      "Not Found",
}

type httpError struct {
	HTTPTyp  Type       `json:"http_type"`
	HasType  bool       `json:"has_type"`
	TypedErr TypedError `json:"typed_err,omitempty"`
	Msg      string     `json:"message"`
}

func (e httpError) Type() Type {
	return e.HTTPTyp
}

func (e httpError) InnerType() Type {
	if e.HasType {
		return e.TypedErr.Type()
	}
	return e.HTTPTyp
}

func (e httpError) InnerErr() TypedError {
	if e.HasType {
		return e.TypedErr
	}
	return nil
}

func (e httpError) Error() string {
	if e.HasType && e.TypedErr.Type()/baseConst < caseHTTP {
		return e.TypedErr.Error()
	}
	return fmt.Sprintf("%s: %s", e.Type().Reference(), e.Msg)
}

func (e httpError) HTTPCode() int {
	return httpCode[e.HTTPTyp]
}

func newHTTPErrGenerator(typ Type) func(e error) HTTPError {
	return func(e error) HTTPError {
		if e == nil {
			return nil
		}
		he := &httpError{
			HTTPTyp: typ,
			Msg:     e.Error(),
		}
		if typedErr, ok := e.(TypedError); ok {
			he.HasType = true
			he.TypedErr = typedErr
		}
		return he
	}
}

// funcs to create new HTTPError
// example: errHTTP := NewInternalError(err)
var (
	NewInternalError = newHTTPErrGenerator(InternalError)
	NewMalformedData = newHTTPErrGenerator(MalformedData)
	NewInvalidData   = newHTTPErrGenerator(InvalidData)
	NewForbidden     = newHTTPErrGenerator(Forbidden)
	NewNotFound      = newHTTPErrGenerator(NotFound)
)
