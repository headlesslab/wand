package main

import (
	"strings"

	"github.com/headlesslab/lazyjson"
)

type objType int

const (
	objTypeStruct    objType = iota // such as object
	objTypePrimitive                // such as string, bool
)

type cdpType string

const (
	cdpTypeTypes    cdpType = "types"
	cdpTypeCommands cdpType = "commands"
	cdpTypeEvents   cdpType = "events"
)

type domain struct {
	name         string
	experimental bool
	deprecated   bool
	description  string
	definitions  []*definition
	global       lazyjson.JSON
}

func (schema *domain) find(id string) lazyjson.JSON {
	domain := schema.name
	list := strings.Split(id, ".")
	if len(list) == 2 {
		domain, id = list[0], list[1]
	}

	for _, schema := range schema.global.Get("domains").Arr() {
		if schema.Get("domain").Str() == domain {
			for _, s := range schema.Get("types").Arr() {
				if s.Get("id").Str() == id {
					return s
				}
			}
		}
	}
	panic("cannot find: " + domain + "." + id)
}

func (schema *domain) ref(id string) bool {
	return schema.find(id).Has("properties")
}

type definition struct {
	domain       *domain
	objType      objType
	cdpType      cdpType
	typeName     string
	enum         []string
	name         string
	originName   string
	description  string
	experimental bool
	deprecation  string // the Deprecated paragraph, without its "Deprecated: " prefix; empty when not deprecated
	optional     bool
	command      bool
	returnValue  bool
	props        []*definition
	skip         bool
}

func (d *definition) deprecated() bool {
	return d.deprecation != ""
}

// parse applies the patches and the marker-less binary fields to the schema
// and turns it into the domains to generate.
func parse(schema lazyjson.JSON, binary []binaryField) ([]*domain, error) {
	if err := patch(schema); err != nil {
		return nil, err
	}
	if err := restoreBinary(schema, binary); err != nil {
		return nil, err
	}

	list := []*domain{}

	for _, domainSchema := range schema.Get("domains").Arr() {
		list = append(list, parseDomain(schema, domainSchema))
	}

	return list, nil
}

func parseDomain(global, schema lazyjson.JSON) *domain {
	domain := &domain{
		name:         schema.Get("domain").Str(),
		experimental: schema.Get("experimental").Bool(),
		deprecated:   schema.Get("deprecated").Bool(),
		definitions:  []*definition{},
		global:       global,
	}

	if schema.Has("description") {
		domain.description = schema.Get("description").Str()
	}

	for _, cdpType := range []cdpType{cdpTypeTypes, cdpTypeCommands, cdpTypeEvents} {
		for _, typeSchame := range schema.Get(string(cdpType)).Arr() {
			domain.definitions = append(domain.definitions, parseDef(domain, cdpType, typeSchame)...)
		}
	}

	return domain
}

// deprecatedInProtocol is the Deprecated paragraph of anything the protocol
// names, such as Page.setDownloadBehavior or Network.setBlockedURLs.urls.
func deprecatedInProtocol(cdpName string) string {
	return cdpName + " is deprecated in the Chrome DevTools Protocol"
}

// entityDeprecation is the Deprecated paragraph of a type, command or
// event: the whole domain when the domain is deprecated, otherwise what the
// schema flags, or nothing (ADR-0004).
func entityDeprecation(d *domain, cdpName string, schema lazyjson.JSON) string {
	switch {
	case d.deprecated:
		return "the " + d.name + " domain is deprecated in the Chrome DevTools Protocol"
	case schema.Get("deprecated").Bool():
		return deprecatedInProtocol(cdpName)
	}
	return ""
}

// fieldDeprecation is the Deprecated paragraph of a parameter, property or
// return value: only what the schema flags on the field itself. A field of a
// deprecated entity is not flagged on its own; using the entity is what a Go
// tool warns about.
func fieldDeprecation(cdpName string, schema lazyjson.JSON) string {
	if schema.Get("deprecated").Bool() {
		return deprecatedInProtocol(cdpName)
	}
	return ""
}

func parseDef(domain *domain, cdpType cdpType, schema lazyjson.JSON) []*definition {
	list := []*definition{}

	switch cdpType {
	case cdpTypeTypes:
		if schema.Has("properties") {
			list = append(list, parseStruct(domain, cdpType, schema.Get("id").Str(), false, schema, "properties")...)
		} else {
			id := schema.Get("id").Str()
			list = append(list, &definition{
				domain:       domain,
				typeName:     typeName(domain, schema),
				name:         domain.name + symbol(id),
				description:  schema.Get("description").Str(),
				deprecation:  entityDeprecation(domain, domain.name+"."+id, schema),
				experimental: schema.Get("experimental").Bool(),
				objType:      objTypePrimitive,
				enum:         enumList(schema),
				skip:         schema.Get("skip").Bool(),
			})
		}
	case cdpTypeCommands:
		list = append(list, parseStruct(domain, cdpType, schema.Get("name").Str(), true, schema, "parameters")...)
		if schema.Has("returns") {
			list = append(list, parseStruct(domain, cdpType, schema.Get("name").Str()+"Result", false, schema, "returns")...)
		}

	case cdpTypeEvents:
		list = append(list, parseStruct(domain, cdpType, schema.Get("name").Str(), false, schema, "parameters")...)

	default:
		panic("type error: " + schema.Str())
	}

	return list
}

func parseStruct(domain *domain, cdpType cdpType, name string, isCommand bool, schema lazyjson.JSON, propsPath string) []*definition {
	list := []*definition{}

	// cdpName is the entity as the protocol names it; a command's result
	// shares the command's name, since the schema flags the command as a
	// whole.
	cdpName := domain.name + "." + schema.Get("name").Str()
	if schema.Has("id") {
		cdpName = domain.name + "." + schema.Get("id").Str()
	}
	deprecation := entityDeprecation(domain, cdpName, schema)

	props := []*definition{}
	for _, propSchema := range schema.Get(propsPath).Arr() {
		typeName := typeName(domain, propSchema)

		prop := &definition{
			objType:      objTypePrimitive,
			name:         symbol(propSchema.Get("name").Str()),
			originName:   propSchema.Get("name").Str(),
			description:  propSchema.Get("description").Str(),
			optional:     propSchema.Get("optional").Bool(),
			deprecation:  fieldDeprecation(cdpName+"."+propSchema.Get("name").Str(), propSchema),
			experimental: propSchema.Get("experimental").Bool(),
			typeName:     typeName,
		}

		props = append(props, prop)

		if propSchema.Has("enum") {
			// The enum type exists for this one field, so it is deprecated
			// with the field, or with the entity the field belongs to.
			enumDeprecation := prop.deprecation
			if enumDeprecation == "" {
				enumDeprecation = deprecation
			}
			enum := &definition{
				domain:      domain,
				name:        domain.name + symbol(name) + symbol(propSchema.Get("name").Str()),
				objType:     objTypePrimitive,
				description: "enum",
				deprecation: enumDeprecation,
				enum:        enumList(propSchema),
				typeName:    typeName,
			}
			list = append(list, enum)

			prop.typeName = enum.name
		}
	}

	desc := schema.Get("description").Str()
	if !isCommand && schema.Has("returns") {
		desc = "..."
	}

	list = append(list, &definition{
		domain:       domain,
		cdpType:      cdpType,
		objType:      objTypeStruct,
		typeName:     typeName(domain, schema),
		name:         domain.name + symbol(name),
		originName:   name,
		description:  desc,
		optional:     schema.Get("optional").Bool(),
		deprecation:  deprecation,
		experimental: schema.Get("experimental").Bool(),
		props:        props,
		command:      isCommand,
		returnValue:  schema.Has("returns"),
		skip:         schema.Get("skip").Bool(),
	})

	return list
}

// byteFields lists every []byte the definitions carry, by Go name: a
// primitive type declared as bytes, or a struct field of bytes or of a slice
// of bytes. The PDL must use binary exactly that many times (ADR-0004).
func byteFields(domains []*domain) []string {
	list := []string{}
	for _, d := range domains {
		for _, def := range d.definitions {
			if def.skip {
				continue
			}
			if def.objType == objTypePrimitive && isBytes(def.typeName) {
				list = append(list, def.name)
			}
			for _, prop := range def.props {
				if isBytes(prop.typeName) {
					list = append(list, def.name+"."+prop.name)
				}
			}
		}
	}
	return list
}

func isBytes(typeName string) bool {
	return typeName == "[]byte" || typeName == "[][]byte"
}

// usesLazyjson reports whether any definition of the domain has a lazyjson
// type, so that the generated file imports the package only when it uses
// it.
func usesLazyjson(d *domain) bool {
	for _, def := range d.definitions {
		if def.skip {
			continue
		}
		if strings.Contains(def.typeName, "lazyjson.") {
			return true
		}
		for _, prop := range def.props {
			if strings.Contains(prop.typeName, "lazyjson.") {
				return true
			}
		}
	}
	return false
}
