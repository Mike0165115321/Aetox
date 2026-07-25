package skill

import "strings"

// stringArg reads an optional string option out of an Input, tolerating the
// absent key and a wrong type the same way — as "not given".
func stringArg(value any) string {
	s, _ := value.(string)
	return s
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
