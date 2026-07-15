package util

import (
	"net/http/httptest"
	"testing"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func TestSetPaginationHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.org/v1/schemas?page=2&perPage=10", nil)
	headers := make(map[string]string)

	SetPaginationHeaders(req, func(key, val string) {
		headers[key] = val
	}, models.Pagination{
		TotalRecords:  30,
		TotalPages:    3,
		CurrentPage:   2,
		RecordsPerPage: 10,
		Previous:      intPtr(1),
		Next:          intPtr(3),
	})

	if headers["Total-Count"] != "30" || headers["Total-Pages"] != "3" || headers["Current-Page"] != "2" {
		t.Fatalf("pagination headers = %#v", headers)
	}
	if headers["Link"] == "" {
		t.Fatalf("Link header is empty")
	}
}

func intPtr(v int) *int {
	return &v
}
