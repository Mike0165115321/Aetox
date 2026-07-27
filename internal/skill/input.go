package skill

import "strings"

// stringArg reads an optional string option out of an Input, tolerating the
// absent key and a wrong type the same way — as "not given".
func stringArg(value any) string {
	s, _ := value.(string)
	return s
}

// anyStringSlice reads a JSON array of strings out of a model's tool-call
// arguments. Decoded JSON arrives as []any, never []string, so a plain
// stringSlice on the same value silently sees nothing.
func anyStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return stringSlice(value) // already []string, from a non-JSON caller
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		result = append(result, strings.TrimSpace(s))
	}
	return result
}

func stringSlice(value any) []string {
	raw, ok := value.([]string)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if strings.TrimSpace(item) == "" {
			continue
		}
		result = append(result, strings.TrimSpace(item))
	}
	return result
}
