package api

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type NullInt struct {
	Value  int
	IsNull bool
}

func NewNullInt(value int) NullInt {
	return NullInt{
		Value:  value,
		IsNull: false,
	}
}

func (ni NullInt) MarshalJSON() ([]byte, error) {
	if ni.IsNull {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(int64(ni.Value), 10)), nil
}

func (ni *NullInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("UnmarshalJSON: no data")
	}

	switch data[0] {
	case 'n':
		ni.Value = 0
		ni.IsNull = true
		return nil

	case '"':
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return fmt.Errorf("null: couldn't unmarshal number string: %w", err)
		}
		n, err := strconv.ParseInt(str, 10, strconv.IntSize)
		if err != nil {
			return fmt.Errorf("null: couldn't convert string to int: %w", err)
		}
		ni.Value = int(n)
		ni.IsNull = false
		return nil

	default:
		err := json.Unmarshal(data, &ni.Value)
		ni.IsNull = err != nil
		return err
	}
}
