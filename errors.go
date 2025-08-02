package gozstd

import (
	"errors"
	"fmt"
)

// Common errors returned by the library
var (
	// ErrEmptyDictionary is returned when attempting to create a dictionary with empty data
	ErrEmptyDictionary = errors.New("dictionary cannot be empty")
	
	// ErrNilPointer is returned when a required pointer is nil
	ErrNilPointer = errors.New("nil pointer provided")
	
	// ErrInvalidParameter is returned when an invalid parameter value is provided
	ErrInvalidParameter = errors.New("invalid parameter value")
)

// ParameterError represents an error related to compression parameters
type ParameterError struct {
	Parameter CParameter
	Value     int
	Message   string
}

func (e *ParameterError) Error() string {
	return fmt.Sprintf("parameter %v: invalid value %d: %s", e.Parameter, e.Value, e.Message)
}

// DecompressionError represents an error during decompression
type DecompressionError struct {
	Code    int
	Message string
}

func (e *DecompressionError) Error() string {
	return fmt.Sprintf("decompression error (code %d): %s", e.Code, e.Message)
}