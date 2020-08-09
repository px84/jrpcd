package main

import (
	"bytes"
	"fmt"
)

type Response map[string]interface{}
type Responses []Response

func decodeResponses(data []byte) (Responses, error) {
	var dec interface{}
	je := fjson.NewDecoder(bytes.NewReader(data))
	je.UseNumber()
	if err := je.Decode(&dec); err != nil {
		return nil, err
	}

	var resps Responses
	switch v := dec.(type) {
	case []interface{}:
		for _, ent := range v {
			if raw, ok := ent.(map[string]interface{}); ok {
				resps = append(resps, raw)
			}
		}
	case map[string]interface{}:
		resps = append(resps, v)
	default:
		return nil, fmt.Errorf("Unsupported response object: %T", v)
	}

	return resps, nil
}
