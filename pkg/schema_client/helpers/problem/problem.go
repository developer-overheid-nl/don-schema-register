package problem

import (
	"net/http"
	"strings"

	commonproblem "github.com/developer-overheid-nl/don-register-common/problem"
)

type InvalidParam = commonproblem.InvalidParam
type ErrorDetail = commonproblem.ErrorDetail

// ProblemJSON implementeert de RFC 9457 error-envelope met de door de
// NL GOV API Design Rules verplichte velden status, title en detail.
type ProblemJSON struct {
	Status int           `json:"status"`
	Title  string        `json:"title"`
	Detail string        `json:"detail"`
	Errors []ErrorDetail `json:"errors,omitempty"`
}

func (e ProblemJSON) Error() string { return e.Title }

// New maakt een ProblemJSON. Detail wordt afgeleid van de meegegeven
// foutdetails; zonder details wordt de title gebruikt.
func New(status int, title string, details ...ErrorDetail) ProblemJSON {
	return ProblemJSON{
		Status: status,
		Title:  title,
		Detail: detailFrom(title, details),
		Errors: details,
	}
}

func NewBadRequest(title string, details ...ErrorDetail) ProblemJSON {
	return New(http.StatusBadRequest, title, details...)
}

func NewNotFound(title string, details ...ErrorDetail) ProblemJSON {
	return New(http.StatusNotFound, title, details...)
}

func NewInternalServerError(title string, details ...ErrorDetail) ProblemJSON {
	return New(http.StatusInternalServerError, title, details...)
}

func NewForbidden(title string, details ...ErrorDetail) ProblemJSON {
	return New(http.StatusForbidden, title, details...)
}

func detailFrom(title string, details []ErrorDetail) string {
	parts := make([]string, 0, len(details))
	for _, d := range details {
		if d.Detail != "" {
			parts = append(parts, d.Detail)
		}
	}
	if len(parts) == 0 {
		return title
	}
	return strings.Join(parts, "; ")
}
