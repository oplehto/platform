#### Purpose for the errors package
    * Place all errors in one package
    * All errors should have a reference code
    * Errors could be serialized and deserialized without string parsing
    * HTTP errors should have a http status code

#### Error hierarchy
    We inplemented 2 interfaces

    ```go
       type TypedError interface {
	        Code() Type
	        error
        }
    ```
    All errors in errors package are Typed Error:
    withErr, withValue, ConstErr, httpError 


    ```go
        type HTTPError interface {
	        TypedError
	        HTTPCode() int
        }
    ```
    All HTTPError are TypedError: httpError



    - format vs no format
    - Standard Go Errors (withErr)
    - http errors

#### Creating/Updating an Error in errors package
       - Is the error needs to wrap with another error?
         like `errors.Wrap("a label", err)`
         Yes? Create an error in withErr.go
         * Append the Enum name in const
         * Append the label in withErrStr.
         * Append a NewFunc in the vars func
         * Append a testcode in errors_test.go TestIota 
       
       - Is the error needs to wrap one or more value?
         like `fmt.Errof("The name %s already exist", name)`
         Yes? Create an error in withValue.go
         * Append the Enum name in const
         * Append the label format in withValueStr
         * Create the New func 
         * Append a testcode in errors_test.go TestIota
       
       - Is the error just a const string
         like `fmt.Errof("Organization not found")`
         Yes? Create an error in const.go
         * Append the enum name to const
         * Append the string value to constStr
         * Append a testcode in errors_test.go TestIota 

       - Is the error needs an http code
         Yes? Create an error in http.go
         * Append the enum name to const
         * Append the http code in httpCode
         * Append the string value in httpStr
         * Append the New func in func var
         * Append a testcode in errors_test.go TestIota
          