package httpx

import (
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Report JSON field names rather than Go identifiers.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" || name == "" {
			return f.Name
		}
		return name
	})
	return v
}()

// Validate runs struct-tag validation and converts failures into a 400 with
// per-field messages under error.details.
func Validate(v any) error {
	err := validate.Struct(v)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return ErrBadRequest
	}
	details := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		details[fe.Field()] = describe(fe)
	}
	return &Error{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_ERROR",
		Message: "request validation failed",
		Details: details,
	}
}

func describe(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	case "gte":
		return "must be >= " + fe.Param()
	case "gt":
		return "must be > " + fe.Param()
	case "lte":
		return "must be <= " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	case "alphanumunicode", "alphanum":
		return "must contain only letters and digits"
	default:
		return "is invalid (" + fe.Tag() + ")"
	}
}
