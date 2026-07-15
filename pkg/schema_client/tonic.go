package schema_client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/loopfz/gadgeto/tonic"
)

// init registreert de tonic error hook zodat zowel de server als de tests
// dezelfde ProblemJSON foutafhandeling gebruiken.
func init() {
	tonic.SetErrorHook(func(c *gin.Context, err error) (int, interface{}) {
		// 1) Bind/validate errors → 400 met correcte invalidParams
		var be tonic.BindError
		if errors.As(err, &be) || isValidationErr(err) {
			invalids := invalidParamsFromBinding(c, err)
			apiErr := problem.NewBadRequest("Request validation failed", invalids...)
			c.Header("Content-Type", "application/problem+json")
			return apiErr.Status, apiErr
		}

		// 2) Eigen ProblemJSON → pass-through
		var apiErr problem.ProblemJSON
		if errors.As(err, &apiErr) {
			c.Header("Content-Type", "application/problem+json")
			return apiErr.Status, apiErr
		}

		// 3) Alles anders → 500
		internal := problem.NewInternalServerError("Internal server error")
		c.Header("Content-Type", "application/problem+json")
		return internal.Status, internal
	})
}

func invalidParamsFromBinding(c *gin.Context, err error) []problem.ErrorDetail {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return []problem.ErrorDetail{{
			In:       inferLocation(c, ""),
			Location: "#/",
			Code:     "invalid",
			Detail:   err.Error(),
		}}
	}

	out := make([]problem.ErrorDetail, 0, len(verrs))
	for _, fe := range verrs {
		field := normalizeFieldName(fe.Field())
		out = append(out, problem.ErrorDetail{
			In:       inferLocation(c, field),
			Location: fmt.Sprintf("#/%s", field),
			Code:     fe.Tag(),
			Detail:   humanReason(fe),
		})
	}
	return out
}

func humanReason(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "url":
		return "must be a valid URL"
	default:
		return fe.Error()
	}
}

func normalizeFieldName(name string) string {
	if name == "" {
		return "body"
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func inferLocation(c *gin.Context, field string) string {
	if strings.EqualFold(field, "id") {
		return "path"
	}
	if c.Request != nil && c.Request.Method == http.MethodGet {
		return "query"
	}
	return "body"
}

func isValidationErr(err error) bool {
	var verrs validator.ValidationErrors
	return errors.As(err, &verrs)
}
