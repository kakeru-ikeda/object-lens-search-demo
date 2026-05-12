package middleware

import (
	"encoding/json"
	"io"
)

func jsonNewEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}
