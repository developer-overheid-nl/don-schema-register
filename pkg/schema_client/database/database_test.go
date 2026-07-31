package database

import (
	"strings"
	"testing"
)

func TestConnectReturnsErrorForInvalidConnectionString(t *testing.T) {
	_, err := Connect("not a postgres connection string")
	if err == nil {
		t.Fatal("Connect() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to connect to database") {
		t.Fatalf("error = %v, want connection error", err)
	}
}
