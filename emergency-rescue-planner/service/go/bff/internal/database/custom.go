package database

import (
	"encoding/json"
	"fmt"
)

// Scan implements sql.Scanner, converting string or []byte from the database into GeoJSON.
func (g *GeoJSON) Scan(value any) error {
	switch v := value.(type) {
	case string:
		*g = GeoJSON(v)
	case []byte:
		*g = GeoJSON(v)
	default:
		return fmt.Errorf("GeoJSON scan: unsupported type %T", value)
	}
	return nil
}

// MarshalJSON implements json.Marshaler, serializing GeoJSON as a raw JSON object.
func (g *GeoJSON) MarshalJSON() ([]byte, error) {
	return json.RawMessage(*g).MarshalJSON()
}
