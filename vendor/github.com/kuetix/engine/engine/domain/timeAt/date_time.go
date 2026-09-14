package timeAt

import (
	"encoding/json"
	"time"
)

type DateTime struct {
	time.Time
}

const dateTimeFormat = "2006-01-02 15:04:05"

// MarshalJSON implements the json.Marshaler interface
func (ct *DateTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(ct.Format(dateTimeFormat))
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (ct *DateTime) UnmarshalJSON(data []byte) error {
	// Remove the surrounding quotes
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}

	parsedTime, err := time.Parse(dateTimeFormat, str)
	if err != nil {
		return err
	}

	ct.Time = parsedTime

	return nil
}
