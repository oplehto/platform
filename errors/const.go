package errors

// compile time interface detection.
var _ TypedError = ConstError(0)

// ConstError implements TypedError interface, is a constant int type error.
type ConstError Type

// Error implement the error interface and returns the string value.
func (e ConstError) Error() string {
	return constStr[e-baseConst]
}

// Code implement the TypedError interface.
func (e ConstError) Code() Type {
	return Type(e)
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

var constStr = []string{
	"authorization" + notFound,
	"authorization" + notFoundContext,
	"organization" + notFound,
	"user" + notFound,
	"token" + notFound,
	"url missing id",
	"empty value",
}
