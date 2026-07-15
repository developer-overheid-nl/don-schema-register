package models

// DialectLabels bevat de labels en omschrijvingen per JSON Schema dialect waarde.
var DialectLabels = map[string][2]string{
	"2020-12":  {"Draft 2020-12", "JSON Schema dialect 2020-12."},
	"2019-09":  {"Draft 2019-09", "JSON Schema dialect 2019-09."},
	"draft-07": {"Draft-07", "JSON Schema dialect draft-07."},
	"draft-06": {"Draft-06", "JSON Schema dialect draft-06."},
	"draft-04": {"Draft-04", "JSON Schema dialect draft-04."},
	"oas-3.1":  {"OpenAPI 3.1", "OpenAPI 3.1 base dialect."},
	"unknown":  {"Onbekend", "Geen herkenbaar $schema dialect."},
}

// RootTypeLabels bevat de labels en omschrijvingen per root type waarde.
var RootTypeLabels = map[string][2]string{
	"object":  {"Object", "Schema beschrijft een object."},
	"array":   {"Array", "Schema beschrijft een array."},
	"string":  {"String", "Schema beschrijft een string."},
	"number":  {"Number", "Schema beschrijft een getal."},
	"integer": {"Integer", "Schema beschrijft een geheel getal."},
	"boolean": {"Boolean", "Schema beschrijft een boolean."},
	"null":    {"Null", "Schema beschrijft null."},
	"unknown": {"Onbekend", "Geen expliciet root type (bijv. allOf of $ref)."},
}
