package sport

import "encoding/json"

// Sport is the Sport JSON in docs/api.md. Active and SortOrder are only set for the admin
// catalogue (includeInactive).
type Sport struct {
	ID            int64          `json:"id"`
	Code          string         `json:"code"`
	Name          string         `json:"name"`
	IsTeamSport   bool           `json:"isTeamSport"`
	Positions     []Position     `json:"positions"`
	StatTemplates []StatTemplate `json:"statTemplates"`
	Active        *bool          `json:"active,omitempty"`
	SortOrder     *int           `json:"sortOrder,omitempty"`
}

// Position is a sport_positions row.
type Position struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	AppliesTo string `json:"appliesTo"`
	Active    *bool  `json:"active,omitempty"`
	SortOrder *int   `json:"sortOrder,omitempty"`
}

// StatTemplate is a stat_templates row.
type StatTemplate struct {
	ID         int64       `json:"id"`
	PositionID *int64      `json:"positionId"`
	Name       string      `json:"name"`
	Version    int         `json:"version"`
	Fields     []StatField `json:"fields"`
	Active     *bool       `json:"active,omitempty"`
}

// StatField is one entry of stat_templates.fields.
type StatField struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Type  string  `json:"type"`
	Unit  *string `json:"unit"`
}

// FieldTypes are the allowed StatField.Type values.
var FieldTypes = []string{"int", "decimal", "text", "duration_ms", "percent"}

// ParseFields decodes a stat_templates.fields JSON value (bad JSON gives an empty list).
func ParseFields(b []byte) []StatField {
	out := []StatField{}
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = []StatField{}
	}
	return out
}
