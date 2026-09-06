package main

import (
	"errors"
	"fmt"

	"github.com/headlesslab/lazyjson"
)

// entry finds the element of a schema array whose key holds value.
func entry(list lazyjson.JSON, key, value string) (lazyjson.JSON, error) {
	for _, el := range list.Arr() {
		if el.Get(key).Str() == value {
			return el, nil
		}
	}
	return lazyjson.JSON{}, fmt.Errorf("no %s %q", key, value)
}

// at walks a schema path of (list, key, value) triples: domains/<domain>,
// then <section>/<id or name>, then <list>/<name>.
func at(schema lazyjson.JSON, steps ...string) (lazyjson.JSON, error) {
	if len(steps)%3 != 0 {
		return lazyjson.JSON{}, errors.New("a schema path is list, key, value triples")
	}
	j := schema
	for i := 0; i < len(steps); i += 3 {
		list, key, value := steps[i], steps[i+1], steps[i+2]
		el, err := entry(j.Get(list), key, value)
		if err != nil {
			return lazyjson.JSON{}, fmt.Errorf("%s: %w", list, err)
		}
		j = el
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
	j, err := at(schema, "domains", "domain", "Target", "types", "id", "TargetInfo", "properties", "name", "type")
	if err != nil {
		return err
	}
	j.Set("enum", []string{
		"page", "background_page", "service_worker", "shared_worker", "browser", "other",
	})

	// PageLifecycleEventName
	j, err = at(schema, "domains", "domain", "Page", "events", "name", "lifecycleEvent", "parameters", "name", "name")
	if err != nil {
		return err
	}
	j.Set("enum", []string{
		"init", "firstPaint", "firstContentfulPaint", "firstImagePaint", "firstMeaningfulPaintCandidate",
		"DOMContentLoaded", "load", "networkAlmostIdle", "firstMeaningfulPaint", "networkIdle",
	})

	// replace these with better type definition
	for _, t := range [][2]string{{"Input", "TimeSinceEpoch"}, {"Network", "TimeSinceEpoch"}, {"Network", "MonotonicTime"}} {
		j, err = at(schema, "domains", "domain", t[0], "types", "id", t[1])
		if err != nil {
			return err
		}
		j.Set("skip", true)
	}

	// fix Cookie.Expires
	j, err = at(schema, "domains", "domain", "Network", "types", "id", "Cookie", "properties", "name", "expires")
	if err != nil {
		return err
	}
	j.Del("type")
	j.Set("$ref", "TimeSinceEpoch")
	j.Set("description", "Cookie expiration date")

	// deltaX and deltaY are not optional for mouseWheel events
	for _, name := range []string{"deltaX", "deltaY"} {
		j, err = at(schema, "domains", "domain", "Input", "commands", "name", "dispatchMouseEvent", "parameters", "name", name)
		if err != nil {
			return err
		}
		j.Del("optional")
	}

	// removing the optional for the body as we need to distinguish between no body and empty body
	// with that fix we can send an 'empty body' using `SetBody([]byte{})`
	// and 'no body' by not calling using 'SetBody()' on the response
	j, err = at(schema, "domains", "domain", "Fetch", "commands", "name", "fulfillRequest", "parameters", "name", "body")
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
		idKey := "name"
		if f.section == "types" {
			idKey = "id"
		}
		j, err := at(schema, "domains", "domain", f.domain, f.section, idKey, f.container, f.list, "name", f.field)
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
