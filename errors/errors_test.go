package errors

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithErr(t *testing.T) {
	src := []TypedError{
		NewFailedToGetStorageHost(errors.New("1")),
		NewFailedToGetBucketName(errors.New("2")),
	}
	codes := []Type{
		FailedToGetStorageHost,
		FailedToGetBucketName,
	}
	strs := []string{
		"Failed to get the storage host",
		"Failed to get the bucket name",
	}
	errStr := []string{
		"Failed to get the storage host: 1",
		"Failed to get the bucket name: 2",
	}
	for k, v := range src {
		item := v.(withErr)
		assert.Equal(t, codes[k], item.Code())
		assert.Equal(t, strs[k], item.typ.Reference())
		assert.Equal(t, errStr[k], item.Error())
	}
	// test nil
	assert.True(t, NewFailedToGetBucketName(nil) == nil)
}

func TestWithValue(t *testing.T) {
	src := []TypedError{
		NewOrganizationNameAlreadyExist("1"),
		NewUserNameAlreadyExist("2"),
	}
	codes := []Type{
		OrganizationNameAlreadyExist,
		UserNameAlreadyExist,
	}
	strs := []string{
		"organization with name %s already exists",
		"user with name %s already exists",
	}
	errStr := []string{
		"organization with name 1 already exists",
		"user with name 2 already exists",
	}
	for k, v := range src {
		item := v.(withValue)
		assert.Equal(t, codes[k], item.Code())
		assert.Equal(t, strs[k], item.typ.Reference())
		assert.Equal(t, errStr[k], item.Error())
	}
}

func TestConst(t *testing.T) {
	src := []TypedError{
		AuthorizationNotFound,
		AuthorizationNotFoundContext,
	}
	codes := []Type{
		Type(AuthorizationNotFound),
		Type(AuthorizationNotFoundContext),
	}
	strs := []string{
		"authorization not found",
		"authorization not found on context",
	}

	for k, v := range src {
		item := v.(ConstError)
		assert.Equal(t, codes[k], item.Code())
		assert.Equal(t, strs[k], item.Code().Reference())
		assert.Equal(t, strs[k], item.Error())
	}
}

func TestHTTP(t *testing.T) {
	src := []HTTPError{
		NewInternalError(errors.New("1")),
		NewMalformedData(errors.New("2")),
		NewInvalidData(errors.New("3")),
		NewForbidden(errors.New("4")),
		NewForbidden(NewFailedToGetBucketName(errors.New("5"))),
	}
	codes := []Type{
		Type(InternalError),
		Type(MalformedData),
		Type(InvalidData),
		Type(Forbidden),
		Type(FailedToGetBucketName),
	}
	strs := []string{
		"Internal Error",
		"Malformed Data",
		"Invalid Data",
		"Forbidden",
		"Failed to get the bucket name",
	}
	errStrs := []string{
		"Internal Error: 1",
		"Malformed Data: 2",
		"Invalid Data: 3",
		"Forbidden: 4",
		"Failed to get the bucket name: 5",
	}
	httpCodes := []int{
		http.StatusInternalServerError,
		http.StatusBadRequest,
		http.StatusUnprocessableEntity,
		http.StatusForbidden,
		http.StatusForbidden,
	}
	for k, v := range src {
		assert.Equal(t, codes[k], v.Code())
		assert.Equal(t, strs[k], v.Code().Reference())
		assert.Equal(t, errStrs[k], v.Error())
		assert.Equal(t, httpCodes[k], v.HTTPCode())
	}
}

type mockResp struct {
	header     http.Header
	StatusCode int
	bytes.Buffer
}

func (m *mockResp) Header() http.Header {
	return m.header
}

func (m *mockResp) WriteHeader(statusCode int) {
	m.StatusCode = statusCode
}

func TestHandleHTTP(t *testing.T) {
	e := NewInternalError(errors.New("1"))
	w := &mockResp{
		header: make(http.Header),
	}
	HandleHTTP(context.Background(), e, w)
	assert.Equal(t, http.StatusInternalServerError, w.StatusCode)
	assert.Equal(t, http.Header{
		"X-Influx-Error":     []string{"Internal Error: 1"},
		"X-Influx-Reference": []string{"60000"},
	}, w.header)
}

// TestIota will be used as a document map from int code -> enum name
func TestIota(t *testing.T) {
	// withErr
	assert.Equal(t, Type(1), FailedToGetStorageHost)
	assert.Equal(t, Type(2), FailedToGetBucketName)

	// const
	assert.Equal(t, ConstError(20000), AuthorizationNotFound)
	assert.Equal(t, ConstError(20001), AuthorizationNotFoundContext)
	assert.Equal(t, ConstError(20002), OrganizationNotFound)
	assert.Equal(t, ConstError(20003), UserNotFound)
	assert.Equal(t, ConstError(20004), TokenNotFoundContext)
	assert.Equal(t, ConstError(20005), URLMissingID)
	assert.Equal(t, ConstError(20006), EmptyValue)

	// withValue
	assert.Equal(t, Type(40000), OrganizationNameAlreadyExist)
	assert.Equal(t, Type(40001), UserNameAlreadyExist)

	// withHTTP
	assert.Equal(t, Type(60000), InternalError)
	assert.Equal(t, Type(60001), MalformedData)
	assert.Equal(t, Type(60002), InvalidData)
	assert.Equal(t, Type(60003), Forbidden)
	assert.Equal(t, Type(60004), NotFound)
}
