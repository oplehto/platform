package errors

// ConstError implements TypedError interface, is a constant int type error.
type ConstError Type

// Error implement the error interface and returns the string value.
func (e ConstError) Error() string {
	return constStrMap[e]
}

// Type implement the TypedError interface.
func (e ConstError) Type() Type {
	return Type(e)
}

// InnerErr returns nil for ConstError, implements TypedError interface.
func (e ConstError) InnerErr() TypedError {
	return nil
}

// const errors
const (
	// AuthorizationNotFound indicate an error when authorization is not found.
	AuthorizationNotFound ConstError = baseConst + iota
	// AuthorizationNotFoundContext indicate an error when authorization is not found in context.
	AuthorizationNotFoundContext
	// OrganizationNotFound indicate an error when organization is not found.
	OrganizationNotFound
	// UserNotFound indicate an error when user is not found
	UserNotFound
	// TokenNotFoundContext indicate an error when token is not found in context
	TokenNotFoundContext
	// URLMissingID indicate the request URL missing id parameter.
	URLMissingID
	// EmptyValue indicate an error of empty value.
	EmptyValue
)

// common phases
const (
	notFound        = " not found"
	notFoundContext = " not found on context"
)

var constStrMap = map[ConstError]string{
	AuthorizationNotFound:        "authorization" + notFound,
	AuthorizationNotFoundContext: "authorization" + notFoundContext,
	OrganizationNotFound:         "organization" + notFound,
	UserNotFound:                 "user" + notFound,
	TokenNotFoundContext:         "token" + notFoundContext,
	URLMissingID:                 "url missing id",
	EmptyValue:                   "empty value",
}

// MarshalJSON implements json.Marshaler interface.
func (e ConstError) MarshalJSON() ([]byte, error) {
	return []byte("{}"), nil
}
