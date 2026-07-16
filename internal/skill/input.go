package skill

import "strings"

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
