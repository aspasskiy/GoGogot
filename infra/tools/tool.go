package tools

import (
	"context"
	"fmt"
)

type Result struct {
	Output string
	IsErr  bool
}

func ErrResult(err error) Result {
	return Result{Output: err.Error(), IsErr: true}
}

type Handler func(ctx context.Context, input map[string]any) Result

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Required    []string
	Handler     Handler
}

func GetString(input map[string]any, key string) (string, error) {
	v, _ := input[key].(string)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func GetStringOpt(input map[string]any, key string) string {
	v, _ := input[key].(string)
	return v
}

func GetInt(input map[string]any, key string) (int, error) {
	v, ok := input[key].(float64)
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	return int(v), nil
}

func GetBool(input map[string]any, key string) bool {
	v, _ := input[key].(bool)
	return v
}
