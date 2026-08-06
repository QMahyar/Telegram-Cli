package cli

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gotd/td/tdp"
	"github.com/gotd/td/tg"
)

// rawMethodNames returns the TL method names exposed by the compiled schema
// (e.g. "messages.getDialogs"). Methods are lowercase dotted names with a
// "#id" suffix in TypesMap; constructors/interfaces are capitalized.
func rawMethodNames() []string {
	var names []string
	for _, name := range tg.TypesMap() {
		if isTLMethodName(name) {
			names = append(names, strings.SplitN(name, "#", 2)[0])
		}
	}
	sort.Strings(names)
	return names
}

// isTLMethodName reports whether a TypesMap entry is a method ("a.b#id")
// rather than a constructor ("Message#id") or interface ("Message").
func isTLMethodName(name string) bool {
	seg := strings.SplitN(name, "#", 2)[0]
	parts := strings.Split(seg, ".")
	if len(parts) < 2 {
		return false
	}
	// Methods keep their interface segment lowercase (getDialogs); constructors
	// are capitalized type names.
	for _, p := range parts {
		if p == "" {
			return false
		}
		if p[0] >= 'A' && p[0] <= 'Z' {
			return false
		}
	}
	return true
}

// rawMethodExists checks the compiled registry for a method name.
func rawMethodExists(method string) bool {
	_, ok := tg.NamesMap()[method]
	return ok
}

// rawParam describes one field of a TL method request.
type rawParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// rawMethodSchema reflects over the generated request struct for a method and
// returns its field list from the tdp.TypeInfo (SchemaName + flags-gated
// optionality), enriched with the Go type from reflection.
func rawMethodSchema(method string) ([]rawParam, error) {
	id, ok := tg.NamesMap()[method]
	if !ok {
		return nil, fmt.Errorf("unknown method %q — run `raw list` or check the spelling", method)
	}
	mk, ok := tg.TypesConstructorMap()[id]
	if !ok {
		return nil, fmt.Errorf("no constructor registered for %q", method)
	}
	obj := mk()
	ti := obj.(tdp.Object).TypeInfo()
	// Go types by exported field name (for a richer schema view).
	goTypes := map[string]string{}
	if rv := reflect.ValueOf(obj); rv.Kind() == reflect.Ptr && rv.Elem().Kind() == reflect.Struct {
		st := rv.Elem().Type()
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			if f.IsExported() {
				goTypes[f.Name] = f.Type.String()
			}
		}
	}
	var params []rawParam
	for _, f := range ti.Fields {
		goType := goTypes[f.Name]
		p := rawParam{
			Name: f.SchemaName,
			// Flags-gated fields (tdp Null=true) are optional. Plain scalar
			// fields (limit, hash, ...) carry zero-defaults and are filled by
			// the tdjson encoder when omitted, so only interface/object fields
			// (peers, filters, markup) are genuinely required.
			Required: !f.Null && strings.Contains(goType, "Class"),
		}
		if goType != "" {
			p.Type = goType
		}
		params = append(params, p)
	}
	sort.SliceStable(params, func(i, j int) bool {
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})
	return params, nil
}

// validateRawParams checks a params map against the method schema: unknown
// keys and missing required fields are usage errors; scalar types are checked
// loosely (numbers accept any JSON number, interface fields accept any value).
func validateRawParams(method string, params map[string]any) error {
	schema, err := rawMethodSchema(method)
	if err != nil {
		return err
	}
	byName := make(map[string]rawParam, len(schema))
	for _, p := range schema {
		byName[p.Name] = p
	}
	for k := range params {
		if _, ok := byName[k]; !ok {
			valid := make([]string, 0, len(schema))
			for _, p := range schema {
				valid = append(valid, p.Name)
			}
			return fmt.Errorf("unknown parameter %q for %s (valid: %s)", k, method, strings.Join(valid, ", "))
		}
	}
	// No field is strictly enforced as present: the tdjson encoder fills
	// zero-defaults for omitted params and the server performs the final
	// required-field check. This layer catches typos and shape errors.
	for _, p := range schema {
		val, present := params[p.Name]
		if !present {
			continue
		}
		if err := checkRawParamType(p, val); err != nil {
			return err
		}
	}
	return nil
}

// checkRawParamType does a light type check on one param value. Interface
// fields (peers, filters, markup, ...) accept anything — InvokeJSON encodes
// them from the JSON shape. Scalar fields are checked strictly.
func checkRawParamType(p rawParam, val any) error {
	// Interface / pointer / slice-of-interface fields accept any JSON value.
	if strings.Contains(p.Type, "Class") || strings.HasPrefix(p.Type, "[]") {
		return nil
	}
	base := p.Type
	if strings.HasPrefix(base, "[]") {
		if _, ok := val.([]any); !ok {
			return fmt.Errorf("parameter %q wants an array ([]%s), got %T", p.Name, strings.TrimPrefix(base, "[]"), val)
		}
		return nil
	}
	switch base {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameter %q wants a string, got %T", p.Name, val)
		}
	case "bool":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameter %q wants a boolean, got %T", p.Name, val)
		}
	case "int", "int32", "int64", "long":
		return checkJSONNumber(p, val)
	case "float64", "double":
		return checkJSONNumber(p, val)
	default:
		// Unknown scalar kind — accept; InvokeJSON will reject on the wire.
		return nil
	}
	return nil
}

func checkJSONNumber(p rawParam, val any) error {
	switch val.(type) {
	case float64, json.Number:
		return nil
	default:
		return fmt.Errorf("parameter %q wants a number, got %T", p.Name, val)
	}
}
