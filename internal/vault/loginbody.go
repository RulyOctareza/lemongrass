package vault

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Login field types a ResterUser.FieldTypes entry can hold; a field with no entry is text.
const (
	FieldNumber  = "number"
	FieldBoolean = "boolean"
)

type loginFieldError struct {
	Key    string
	Reason string
}

func (e *loginFieldError) Error() string {
	return fmt.Sprintf("vault: login field %q %s", e.Key, e.Reason)
}

// LoginBody is the JSON body a login POSTs: each field typed by its FieldTypes entry.
func (u ResterUser) LoginBody() ([]byte, error) {
	body := make(map[string]any, len(u.Fields))
	for key, value := range u.Fields {
		v, err := loginFieldValue(key, value, u.FieldTypes[key])
		if err != nil {
			return nil, err
		}
		body[key] = v
	}
	return json.Marshal(body)
}

func loginFieldValue(key, value, kind string) (any, error) {
	switch kind {
	case "":
		return value, nil
	case FieldNumber:
		v := strings.TrimSpace(value)
		if !isJSONNumber(v) {
			return nil, &loginFieldError{Key: key, Reason: "is not a number"}
		}
		return json.Number(v), nil
	case FieldBoolean:
		switch strings.TrimSpace(value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, &loginFieldError{Key: key, Reason: "is not true or false"}
	default:
		return nil, &loginFieldError{Key: key, Reason: fmt.Sprintf("has an unknown type %q", kind)}
	}
}

// isJSONNumber holds because the only valid JSON values starting with '-' or a digit are numbers.
func isJSONNumber(v string) bool {
	return v != "" && (v[0] == '-' || (v[0] >= '0' && v[0] <= '9')) && json.Valid([]byte(v))
}
