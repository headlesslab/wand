package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/headlesslab/lazyjson"
)

func mapType(n string) string {
	return map[string]string{
		"boolean": "bool",
		"number":  "float64",
		"integer": "int",
		"string":  "string",
		"binary":  "[]byte",
		"object":  "map[string]lazyjson.JSON",
		"any":     "lazyjson.JSON",
	}[n]
}

// binaryMarker is what devtools-protocol's JSON appends to the description
// of a PDL binary field, whose type it lowers to string
// (--map_binary_to_string in its roll script). A browser's own /json/protocol
// keeps binary, so the generator restores []byte from the marker (ADR-0004);
// fields without a description have no marker and are listed in
// binaryFields by hand.
const binaryMarker = "Encoded as a base64 string when passed over JSON"

func binaryMarked(schema lazyjson.JSON) bool {
	return schema.Has("description") && strings.Contains(schema.Get("description").Str(), binaryMarker)
}

func typeName(domain *domain, schema lazyjson.JSON) string {
	typeName := ""
	if schema.Has("type") {
		typeName = schema.Get("type").Str()
	}

	if typeName == "array" { //nolint: nestif
		item := schema.Get("items")

		if item.Has("type") {
			typeName = "[]" + mapType(item.Get("type").Str())
			if typeName == "[]string" && binaryMarked(schema) {
				typeName = "[][]byte"
			}
		} else {
			ref := item.Get("$ref").Str()
			if domain.ref(ref) {
				typeName = "[]*" + refName(domain.name, ref)
			} else {
				typeName = "[]" + refName(domain.name, ref)
			}
		}
	} else if schema.Has("$ref") {
		ref := schema.Get("$ref").Str()
		if domain.ref(ref) {
			typeName += "*"
		}
		typeName += refName(domain.name, ref)
	} else {
		typeName = mapType(typeName)
		if typeName == "string" && binaryMarked(schema) {
			typeName = "[]byte"
		}
	}

	switch typeName {
	case "NetworkTimeSinceEpoch", "InputTimeSinceEpoch":
		typeName = "TimeSinceEpoch"
	case "NetworkMonotonicTime":
		typeName = "MonotonicTime"
	}

	return typeName
}

func enumList(schema lazyjson.JSON) []string {
	var enum []string
	if schema.Has("enum") {
		enum = []string{}
		for _, v := range schema.Get("enum").Arr() {
			if _, ok := v.Val().(string); !ok {
				panic("enum type error")
			}
			enum = append(enum, v.Str())
		}
	}

	return enum
}

func jsonTag(name string, optional bool) string {
	jsonTagValue := name
	if optional {
		jsonTagValue += ",omitempty"
	}
	return fmt.Sprintf("`json:\"%s\"`", jsonTagValue)
}

func refName(domain, id string) string {
	if strings.Contains(id, ".") {
		return symbol(id)
	}
	return domain + symbol(id)
}

// make sure golint works fine.
func symbol(n string) string {
	if n == "" {
		return ""
	}

	n = strings.ReplaceAll(n, ".", "")

	dashed := regexp.MustCompile(`[-_]`).Split(n, -1)
	if len(dashed) > 1 {
		converted := []string{}
		for _, part := range dashed {
			converted = append(converted, strings.ToUpper(part[:1])+part[1:])
		}
		n = strings.Join(converted, "")
	}

	n = strings.ToUpper(n[:1]) + n[1:]

	n = replaceLower(n, "Id")
	n = replaceLower(n, "Css")
	n = replaceLower(n, "Url")
	n = replaceLower(n, "Uuid")
	n = replaceLower(n, "Xml")
	n = replaceLower(n, "Http")
	n = replaceLower(n, "Dns")
	n = replaceLower(n, "Cpu")
	n = replaceLower(n, "Mime")
	n = replaceLower(n, "Json")
	n = replaceLower(n, "Html")
	n = replaceLower(n, "Guid")
	n = replaceLower(n, "Sql")
	n = replaceLower(n, "Eof")
	n = replaceLower(n, "Api")
	n = replaceLower(n, "Ui")
	n = replaceLower(n, "Https")

	n = strings.ReplaceAll(n, "Ids", "IDs")

	return n
}

func replaceLower(n, word string) string {
	return regexp.MustCompile(word+`([A-Z-_]|$)`).ReplaceAllStringFunc(n, strings.ToUpper)
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
