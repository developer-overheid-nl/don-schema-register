package schema_client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func TestInvalidParamsFromBindingFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/schemas?q=", nil)

	invalids := invalidParamsFromBinding(ctx, errors.New("bad query"))
	if len(invalids) != 1 {
		t.Fatalf("invalids = %#v, want one fallback error", invalids)
	}
	if invalids[0].In != "query" || invalids[0].Location != "#/" || invalids[0].Code != "invalid" {
		t.Fatalf("invalid = %#v, want query fallback", invalids[0])
	}
}

func TestInvalidParamsFromBindingValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/schemas", nil)

	type request struct {
		SchemaURL string `validate:"required"`
		Website   string `validate:"url"`
	}
	err := validator.New().Struct(request{Website: "not-a-url"})
	invalids := invalidParamsFromBinding(ctx, err)
	if len(invalids) != 2 {
		t.Fatalf("invalids = %#v, want two validation errors", invalids)
	}

	byCode := map[string]string{}
	for _, invalid := range invalids {
		byCode[invalid.Code] = invalid.Detail
		if invalid.In != "body" {
			t.Fatalf("invalid = %#v, want body location", invalid)
		}
	}
	if byCode["required"] != "is required" {
		t.Fatalf("required detail = %q, want is required", byCode["required"])
	}
	if byCode["url"] != "must be a valid URL" {
		t.Fatalf("url detail = %q, want URL message", byCode["url"])
	}
}

func TestInferLocation(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/schemas/schema-1", nil)
	if got := inferLocation(ctx, "id"); got != "path" {
		t.Fatalf("inferLocation(id) = %q, want path", got)
	}
	if got := inferLocation(ctx, "schemaUrl"); got != "body" {
		t.Fatalf("inferLocation(schemaUrl) = %q, want body", got)
	}
}
