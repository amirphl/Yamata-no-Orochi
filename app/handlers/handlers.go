// Package handlers contains HTTP request handlers and presentation layer logic for the API endpoints
package handlers

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

func getValidationErrorMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return err.Field() + " is required"
	case "email":
		return "Invalid email format"
	case "min":
		return err.Field() + " must be at least " + err.Param() + " characters"
	case "max":
		return err.Field() + " must be at most " + err.Param() + " characters"
	case "len":
		return err.Field() + " must be exactly " + err.Param() + " characters"
	case "oneof":
		return err.Field() + " must be one of: " + err.Param()
	case "eqfield":
		return err.Field() + " must match " + err.Param()
	case "alpha_space":
		return err.Field() + " must contain only letters and spaces"
	case "mobile_format":
		return "Mobile number must be in format +989xxxxxxxxx"
	case "password_strength":
		return "Password must contain at least 1 uppercase letter and 1 number"
	case "numeric":
		return err.Field() + " must contain only numbers"
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", err.Field(), err.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", err.Field(), err.Param())
	default:
		return err.Field() + " is invalid"
	}
}
