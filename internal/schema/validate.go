package schema

import (
	"fmt"
	"slices"
	"sort"
)

// Violation is one way an artifact's fields disagree with its schema.
//
// Validation is lenient in a specific sense: it reports and never rejects, and
// the value that produced a violation is still the value the caller reads. A
// field nobody declared is not a violation at all — see Schema.Declares. See
// docs/decision/0002.
type Violation struct {
	Kind   ViolationKind
	Field  string
	Detail string
}

func (v Violation) String() string {
	if v.Detail == "" {
		return fmt.Sprintf("%s: %s", v.Field, v.Kind)
	}
	return fmt.Sprintf("%s: %s", v.Field, v.Detail)
}

// ViolationKind is why a field disagrees with its schema.
type ViolationKind string

const (
	// MissingRequired is a required field that is absent or empty. Absent
	// stays absent: a schema's default is recorded, never substituted.
	MissingRequired ViolationKind = "missing-required"
	// WrongType is a value that is not the type the schema declared.
	WrongType ViolationKind = "wrong-type"
	// NotInEnum is a value outside an enum's declared values.
	NotInEnum ViolationKind = "not-in-enum"
)

// Check reports how an artifact's fields disagree with the schema. The fields
// map is read, never written: an absent field with a declared default stays
// absent, because filling one in would put a value in the artifact that its
// author did not write.
func (s *Schema) Check(fields map[string]any) []Violation {
	var out []Violation

	// Declared order first, so a reader sees the schema's own presentation
	// rather than map iteration order.
	for _, name := range s.order {
		f := s.Fields[name]
		value, present := fields[name]

		if !present || isEmpty(value) {
			if f.Required {
				out = append(out, Violation{
					Kind:   MissingRequired,
					Field:  name,
					Detail: fmt.Sprintf("required by %s and %s", s.Type, absence(present)),
				})
			}
			continue
		}
		if v, ok := checkType(f, value); !ok {
			out = append(out, v)
		}
	}
	return out
}

func absence(present bool) string {
	if present {
		return "empty"
	}
	return "not set"
}

// isEmpty treats an explicitly empty value as absent for the purpose of a
// required field. A field that is present and says nothing has not been
// answered, and reporting it as set would hide the one thing worth saying.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	}
	return false
}

func checkType(f Field, value any) (Violation, bool) {
	switch f.Type {
	case TypeString, TypeReference:
		if _, ok := value.(string); !ok {
			return typeViolation(f, value, "a string"), false
		}
	case TypeEnum:
		s, ok := value.(string)
		if !ok {
			return typeViolation(f, value, "a string"), false
		}
		if len(f.Values) > 0 && !slices.Contains(f.Values, s) {
			return Violation{
				Kind:   NotInEnum,
				Field:  f.Name,
				Detail: fmt.Sprintf("%q is not one of %v", s, f.Values),
			}, false
		}
	case TypeMap:
		if !isMap(value) {
			return typeViolation(f, value, "a map"), false
		}
	case TypeList:
		if _, ok := value.([]any); !ok {
			return typeViolation(f, value, "a list"), false
		}
	default:
		// A type this package does not know is not checked. The schema is the
		// corpus author's, and a kind may want a type kept has no opinion
		// about; refusing it would make the schema less useful than the
		// leniency the artifacts already get.
	}
	return Violation{}, true
}

func typeViolation(f Field, value any, want string) Violation {
	return Violation{
		Kind:   WrongType,
		Field:  f.Name,
		Detail: fmt.Sprintf("declared %s, so %s was expected and %T was found", f.Type, want, value),
	}
}

func isMap(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any:
		return true
	}
	return false
}

// Undeclared returns the field names an artifact carries that the schema does
// not name, sorted. They are not violations: they survive the read and are
// available to the caller, which is the whole of what 0029 requires of them.
func (s *Schema) Undeclared(fields map[string]any) []string {
	var out []string
	for name := range fields {
		if !s.Declares(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
