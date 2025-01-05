package routes

import (
	"log"
	"net/http"

	todosController "github.com/abdullahpichoo/crappy-loadbalancer/backend/controllers/todo-controller"
	"github.com/abdullahpichoo/crappy-loadbalancer/backend/helpers/responses"
)

func AddRoutes(mux *http.ServeMux, logger *log.Logger) {
	mux.HandleFunc("GET /api/health", func(rw http.ResponseWriter, r *http.Request) {
		responses.SuccessResponse(rw, r, "ok", "ok", false)
	})

	mux.HandleFunc("GET /api/todos", func(rw http.ResponseWriter, r *http.Request) {
		todos, err := todosController.Index()
		if err != nil {
			responses.ErrorResponse(rw, r, 422, "Unable to fetch todos")
		}

		responses.SuccessResponse(rw, r, todos, "fetched todos successfully", false)
	})

	mux.Handle("/", http.NotFoundHandler())
}
