package responses

import (
	"net/http"

	"github.com/abdullahpichoo/crappy-loadbalancer/backend/helpers/encode"
)

type Response[T any] struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Success bool   `json:"success"`
	Data    T      `json:"data"`
}

func SuccessResponse[T any](w http.ResponseWriter, r *http.Request, data T, message string, isCreated bool) error {
	statusCode := 200
	if isCreated {
		statusCode = 201
	}

	response := Response[T]{
		Message: message,
		Status:  statusCode,
		Success: true,
		Data:    data,
	}

	return encode.Encode(w, r, statusCode, response)
}

func ErrorResponse(w http.ResponseWriter, r *http.Request, status int, message string) error {
	response := Response[error]{
		Message: message,
		Status:  status,
		Success: false,
		Data:    nil,
	}

	return encode.Encode(w, r, status, response)
}
