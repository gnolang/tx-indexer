package model

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

// Cursor represents a string key of an element position in a sequential list of edges.
type Cursor string

// Create a cursor by base64-encoding the ID.
func NewCursor(id string) Cursor {
	return Cursor(base64.StdEncoding.EncodeToString([]byte(id)))
}

// UnmarshalGraphQL unmarshal incoming cursor into a local variable.
func (c *Cursor) UnmarshalGraphQL(input interface{}) error {
	var err error

	switch input := input.(type) {
	case string:
		*c = Cursor(input)
	case int32:
		*c = Cursor(strconv.Itoa(int(input)))
	default:
		err = errors.New("wrong cursor type")
	}

	return err
}

// MarshalJSON encodes a cursor to JSON for transport.
func (c Cursor) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, string(c)), nil
}

// Decodes and returns the base64-encoded ID, which is the value of the cursor.
func (c *Cursor) ID() (string, error) {
	if c == nil {
		return "", nil
	}

	base64Str := string(*c)

	decoded, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

// Get the block height from the ID of the Cursor.
func (c *Cursor) BlockHeight() (uint64, error) {
	id, err := c.ID()
	if err != nil {
		return 0, err
	}

	if id == "" {
		return 0, nil
	}

	blockHeight, err := strconv.ParseUint(id, 0, 64)
	if err != nil {
		return 0, err
	}

	return blockHeight, nil
}

// Get the values of block height and transaction index by the ID of the Cursor.
func (c *Cursor) BlockHeightWithIndex() (uint64, uint32, error) {
	id, err := c.ID()
	if err != nil {
		return 0, 0, err
	}

	if id == "" {
		return 0, 0, nil
	}

	afterParams := strings.Split(id, "_")
	if len(afterParams) < 2 {
		return 0, 0, errors.New("wrong cursor type")
	}

	blockHeight, err := strconv.ParseUint(afterParams[0], 0, 64)
	if err != nil {
		return 0, 0, err
	}

	index, err := strconv.ParseUint(afterParams[1], 0, 32)
	if err != nil {
		return 0, 0, err
	}

	return blockHeight, uint32(index), nil
}
