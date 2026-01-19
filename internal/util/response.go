package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type APIResponse struct {
	Success    bool        `json:"success"`
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
	Error      *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Success(c *gin.Context, status int, data interface{}) {
	c.JSON(status, APIResponse{
		Success:    true,
		StatusCode: status,
		Message:    statusMessage(status),
		Data:       data,
	})
}

func SuccessWithMessage(c *gin.Context, status int, message string, data interface{}) {
	c.JSON(status, APIResponse{
		Success:    true,
		StatusCode: status,
		Message:    message,
		Data:       data,
	})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, APIResponse{
		Success:    false,
		StatusCode: status,
		Message:    message,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

func ValidationError(c *gin.Context, err error) {
	var message string

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, fieldError := range validationErrors {
			message = formatValidationError(fieldError)
			break
		}
	} else {
		message = err.Error()
	}

	c.JSON(http.StatusBadRequest, APIResponse{
		Success:    false,
		StatusCode: http.StatusBadRequest,
		Message:    message,
		Error: &ErrorInfo{
			Code:    "VALIDATION_ERROR",
			Message: message,
		},
	})
}

func statusMessage(status int) string {
	switch status {
	case http.StatusOK:
		return "Request successful"
	case http.StatusCreated:
		return "Resource created successfully"
	case http.StatusAccepted:
		return "Request accepted"
	case http.StatusNoContent:
		return "Request successful"
	default:
		return "Success"
	}
}

func formatValidationError(fe validator.FieldError) string {
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return field + " is required"
	case "e164":
		return field + " must be a valid phone number in E.164 format"
	case "email":
		return field + " must be a valid email address"
	case "min":
		return field + " must be at least " + fe.Param() + " characters"
	case "max":
		return field + " must be at most " + fe.Param() + " characters"
	case "len":
		return field + " must be exactly " + fe.Param() + " characters"
	case "uuid":
		return field + " must be a valid UUID"
	case "numeric":
		return field + " must contain only numbers"
	default:
		return field + " is invalid"
	}
}
