package problem

import (
	"net/http"
	"testing"
)

func TestProblemConstructors(t *testing.T) {
	detail := ErrorDetail{In: "body", Location: "#/schemaUrl", Code: "required", Detail: "is required"}

	tests := []struct {
		name    string
		problem ProblemJSON
		status  int
		title   string
	}{
		{name: "new", problem: New(http.StatusTeapot, "teapot", detail), status: http.StatusTeapot, title: "teapot"},
		{name: "bad request", problem: NewBadRequest("bad", detail), status: http.StatusBadRequest, title: "bad"},
		{name: "not found", problem: NewNotFound("missing", detail), status: http.StatusNotFound, title: "missing"},
		{name: "internal", problem: NewInternalServerError("boom", detail), status: http.StatusInternalServerError, title: "boom"},
		{name: "forbidden", problem: NewForbidden("nope", detail), status: http.StatusForbidden, title: "nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.problem.Status != tt.status {
				t.Fatalf("status = %d, want %d", tt.problem.Status, tt.status)
			}
			if tt.problem.Title != tt.title {
				t.Fatalf("title = %q, want %q", tt.problem.Title, tt.title)
			}
			if len(tt.problem.Errors) != 1 || tt.problem.Errors[0] != detail {
				t.Fatalf("errors = %#v, want %#v", tt.problem.Errors, []ErrorDetail{detail})
			}
		})
	}
}
