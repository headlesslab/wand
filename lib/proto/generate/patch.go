package main

import (
	"fmt"

	"github.com/headlesslab/lazyjson"
)

// step is one hop of a schema path: the element of the array under list
// whose key holds value.
type step struct {
	list, key, value string
}

// The steps of a schema path: a domain, one of its sections keyed the way
// the protocol keys it (types by id, commands and events by name), and one
// of an entity's lists of fields.
func inDomain(name string) step        { return step{"domains", "domain", name} }
func inSection(section, id string) step { return step{section, sectionKey(section), id} }
func inList(list, name string) step     { return step{list, "name", name} }

// sectionKey is what names an entity in a section of the schema.
func sectionKey(section string) string {
	if section == "types" {
		return "id"
	}
	return "name"
}

// at walks a schema path.
func at(schema lazyjson.JSON, steps ...step) (lazyjson.JSON, error) {
	j := schema
	for _, s := range steps {
		found := false
		for _, el := range j.Get(s.list).Arr() {
			if el.Get(s.key).Str() == s.value {
				j, found = el, true
				break
			}
		}
		if !found {
			return lazyjson.JSON{}, fmt.Errorf("%s: no %s %q", s.list, s.key, s.value)
		}
	}
	return j, nil
}

// patch is upstream's list of hand adjustments to the schema: enums the
// protocol leaves as plain strings, the time types replaced by hand-written
// ones in a_patch.go, and three optional markers dropped where wand needs to
// tell an absent value from a zero one. A patch whose target the pinned roll
// no longer has is an error, so a roll that removes one shows up here rather
// than as a silently changed type.
func patch(schema lazyjson.JSON) error {
	// TargetTargetInfoType
	j, err := at(schema, inDomain("Target"), inSection("types", "TargetInfo"), inList("properties", "type"))
	if err != nil {
		return err
	}
	j.Set("enum", []string{
		"page", "background_page", "service_worker", "shared_worker", "browser", "other",
	})

	// PageLifecycleEventName
	j, err = at(schema, inDomain("Page"), inSection("events", "lifecycleEvent"), inList("parameters", "name"))
	if err != nil {
		return err
	}
	j.Set("enum", []string{
		"init", "firstPaint", "firstContentfulPaint", "firstImagePaint", "firstMeaningfulPaintCandidate",
		"DOMContentLoaded", "load", "networkAlmostIdle", "firstMeaningfulPaint", "networkIdle",
	})

	// replace these with better type definition
	for _, t := range [][2]string{{"Input", "TimeSinceEpoch"}, {"Network", "TimeSinceEpoch"}, {"Network", "MonotonicTime"}} {
		j, err = at(schema, inDomain(t[0]), inSection("types", t[1]))
		if err != nil {
			return err
		}
		j.Set("skip", true)
	}

	// fix Cookie.Expires
	j, err = at(schema, inDomain("Network"), inSection("types", "Cookie"), inList("properties", "expires"))
	if err != nil {
		return err
	}
	j.Del("type")
	j.Set("$ref", "TimeSinceEpoch")
	j.Set("description", "Cookie expiration date")

	// deltaX and deltaY are not optional for mouseWheel events
	for _, name := range []string{"deltaX", "deltaY"} {
		j, err = at(schema, inDomain("Input"), inSection("commands", "dispatchMouseEvent"), inList("parameters", name))
		if err != nil {
			return err
		}
		j.Del("optional")
	}

	// removing the optional for the body as we need to distinguish between no body and empty body
	// with that fix we can send an 'empty body' using `SetBody([]byte{})`
	// and 'no body' by not calling using 'SetBody()' on the response
	j, err = at(schema, inDomain("Fetch"), inSection("commands", "fulfillRequest"), inList("parameters", "body"))
	if err != nil {
		return err
	}
	j.Del("optional")

	return nil
}

// binaryField names a PDL binary field whose JSON carries no marker: the
// JSON lowers PDL binary to string, and only a field with a description gets
// the "Encoded as a base64 string when passed over JSON" marker the
// generator reads, so a field without a description is indistinguishable
// from a real string.
type binaryField struct {
	domain, section, container, list, field string
}

func (f binaryField) String() string {
	return fmt.Sprintf("%s.%s.%s (%s/%s)", f.domain, f.container, f.field, f.section, f.list)
}

func (f binaryField) path() []step {
	return []step{inDomain(f.domain), inSection(f.section, f.container), inList(f.list, f.field)}
}

// binaryFields is the hand-kept list of marker-less binary fields of the
// pinned roll (ADR-0004), started from usesigil/rod's list and kept exact by
// the generator: a listed field the roll does not have fails the run, and
// the count of []byte fields generated must equal the binary occurrences in
// the roll's PDL files, so a marker-less field the list lacks fails it too.
var binaryFields = []binaryField{
	{"BluetoothEmulation", "commands", "simulateCharacteristicOperationResponse", "parameters", "data"},
	{"BluetoothEmulation", "commands", "simulateDescriptorOperationResponse", "parameters", "data"},
	{"BluetoothEmulation", "events", "characteristicOperationReceived", "parameters", "data"},
	{"BluetoothEmulation", "events", "descriptorOperationReceived", "parameters", "data"},
	{"Network", "events", "directTCPSocketChunkReceived", "parameters", "data"},
	{"Network", "events", "directTCPSocketChunkSent", "parameters", "data"},
	{"Network", "types", "DirectUDPMessage", "properties", "data"},
	{"Network", "types", "PostDataEntry", "properties", "bytes"},
	{"Page", "commands", "getManifestIcons", "returns", "primaryIcon"},
	{"SmartCardEmulation", "commands", "reportDataResult", "parameters", "data"},
	{"SmartCardEmulation", "commands", "reportStatusResult", "parameters", "atr"},
	{"SmartCardEmulation", "events", "controlRequested", "parameters", "data"},
	{"SmartCardEmulation", "events", "setAttribRequested", "parameters", "data"},
	{"SmartCardEmulation", "events", "transmitRequested", "parameters", "data"},
	{"SmartCardEmulation", "types", "ReaderStateOut", "properties", "atr"},
	{"WebAuthn", "commands", "getCredential", "parameters", "credentialId"},
	{"WebAuthn", "commands", "removeCredential", "parameters", "credentialId"},
	{"WebAuthn", "commands", "setCredentialProperties", "parameters", "credentialId"},
	{"WebAuthn", "events", "credentialDeleted", "parameters", "credentialId"},
	{"WebAuthn", "types", "Credential", "properties", "credentialId"},
	{"WebAuthn", "types", "Credential", "properties", "cmtgKeys"},
}

// restoreBinary retypes the listed fields to binary, the item type for an
// array, so that typeName maps them to []byte as the browser's own
// /json/protocol would.
func restoreBinary(schema lazyjson.JSON, fields []binaryField) error {
	for _, f := range fields {
		j, err := at(schema, f.path()...)
		if err != nil {
			return fmt.Errorf("%s is listed as a marker-less binary field but the schema has no such field (%w); "+
				"drop it from binaryFields in lib/proto/generate/patch.go if the roll removed it", f, err)
		}
		if items, has := j.Gets("items"); has {
			items.Set("type", "binary")
		} else {
			j.Set("type", "binary")
		}
	}
	return nil
}
