package problem

import commonproblem "github.com/developer-overheid-nl/don-register-common/problem"

type InvalidParam = commonproblem.InvalidParam
type ErrorDetail = commonproblem.ErrorDetail

// ProblemJSON implementeert de RFC 7807 error-envelope (gedeeld via don-register-common).
type ProblemJSON = commonproblem.Problem

func New(status int, title string, details ...ErrorDetail) ProblemJSON {
	return commonproblem.New(status, title, details...)
}

func NewBadRequest(title string, details ...ErrorDetail) ProblemJSON {
	return commonproblem.NewBadRequest(title, details...)
}

func NewNotFound(title string, details ...ErrorDetail) ProblemJSON {
	return commonproblem.NewNotFound(title, details...)
}

func NewInternalServerError(title string, details ...ErrorDetail) ProblemJSON {
	return commonproblem.NewInternalServerError(title, details...)
}

func NewForbidden(title string, details ...ErrorDetail) ProblemJSON {
	return commonproblem.NewForbidden(title, details...)
}
