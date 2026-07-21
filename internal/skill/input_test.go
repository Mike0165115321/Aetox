package skill

import (
	"reflect"
	"testing"
)

func TestStringSlice(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  []string
	}{
		{"nil value", nil, nil},
		{"wrong type", 123, nil},
		{"trims and drops blanks", []string{" a ", "", "  ", "b"}, []string{"a", "b"}},
		{"empty slice", []string{}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stringSlice(c.value)
			if !reflect.DeepEqual(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
				t.Errorf("stringSlice(%#v) = %#v, want %#v", c.value, got, c.want)
			}
		})
	}
}
