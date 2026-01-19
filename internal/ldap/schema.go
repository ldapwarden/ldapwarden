package ldap

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type Schema struct {
	ObjectClasses  map[string]ObjectClass  `json:"objectClasses"`
	AttributeTypes map[string]AttributeType `json:"attributeTypes"`
}

type ObjectClass struct {
	OID         string   `json:"oid"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Superior    []string `json:"superior,omitempty"`
	Kind        string   `json:"kind"` // STRUCTURAL, AUXILIARY, ABSTRACT
	Must        []string `json:"must,omitempty"`
	May         []string `json:"may,omitempty"`
}

type AttributeType struct {
	OID          string `json:"oid"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Syntax       string `json:"syntax,omitempty"`
	SingleValue  bool   `json:"singleValue"`
	NoUserMod    bool   `json:"noUserModification"`
	Usage        string `json:"usage,omitempty"`
	Equality     string `json:"equality,omitempty"`
	Substr       string `json:"substr,omitempty"`
	Ordering     string `json:"ordering,omitempty"`
}

var (
	oidRegex    = regexp.MustCompile(`^\(\s*([0-9.]+)\s+`)
	nameRegex   = regexp.MustCompile(`NAME\s+(?:'([^']+)'|\(\s*([^)]+)\s*\))`)
	descRegex   = regexp.MustCompile(`DESC\s+'([^']*)'`)
	supRegex    = regexp.MustCompile(`SUP\s+(?:'([^']+)'|\(\s*([^)]+)\s*\)|(\S+))`)
	mustRegex   = regexp.MustCompile(`MUST\s+(?:\(\s*([^)]+)\s*\)|(\S+))`)
	mayRegex    = regexp.MustCompile(`MAY\s+(?:\(\s*([^)]+)\s*\)|(\S+))`)
	syntaxRegex = regexp.MustCompile(`SYNTAX\s+([^\s{]+)`)
)

func (c *Client) GetSchema() (*Schema, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		"cn=Subschema",
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"objectClasses", "attributeTypes"},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		searchRequest.BaseDN = ""
		result, err = conn.Search(searchRequest)
		if err != nil {
			return nil, fmt.Errorf("fetch schema: %w", err)
		}
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("schema not found")
	}

	entry := result.Entries[0]
	schema := &Schema{
		ObjectClasses:  make(map[string]ObjectClass),
		AttributeTypes: make(map[string]AttributeType),
	}

	for _, raw := range entry.GetAttributeValues("objectClasses") {
		oc := parseObjectClass(raw)
		if oc.Name != "" {
			schema.ObjectClasses[strings.ToLower(oc.Name)] = oc
		}
	}

	for _, raw := range entry.GetAttributeValues("attributeTypes") {
		at := parseAttributeType(raw)
		if at.Name != "" {
			schema.AttributeTypes[strings.ToLower(at.Name)] = at
		}
	}

	return schema, nil
}

func parseObjectClass(raw string) ObjectClass {
	oc := ObjectClass{}

	if m := oidRegex.FindStringSubmatch(raw); len(m) > 1 {
		oc.OID = m[1]
	}

	if m := nameRegex.FindStringSubmatch(raw); len(m) > 1 {
		if m[1] != "" {
			oc.Name = m[1]
		} else if m[2] != "" {
			names := parseList(m[2])
			if len(names) > 0 {
				oc.Name = names[0]
			}
		}
	}

	if m := descRegex.FindStringSubmatch(raw); len(m) > 1 {
		oc.Description = m[1]
	}

	if m := supRegex.FindStringSubmatch(raw); len(m) > 1 {
		if m[1] != "" {
			oc.Superior = []string{m[1]}
		} else if m[2] != "" {
			oc.Superior = parseList(m[2])
		} else if m[3] != "" {
			oc.Superior = []string{m[3]}
		}
	}

	if strings.Contains(raw, "STRUCTURAL") {
		oc.Kind = "STRUCTURAL"
	} else if strings.Contains(raw, "AUXILIARY") {
		oc.Kind = "AUXILIARY"
	} else if strings.Contains(raw, "ABSTRACT") {
		oc.Kind = "ABSTRACT"
	} else {
		oc.Kind = "STRUCTURAL"
	}

	if m := mustRegex.FindStringSubmatch(raw); len(m) > 1 {
		if m[1] != "" {
			oc.Must = parseList(m[1])
		} else if m[2] != "" {
			oc.Must = []string{m[2]}
		}
	}

	if m := mayRegex.FindStringSubmatch(raw); len(m) > 1 {
		if m[1] != "" {
			oc.May = parseList(m[1])
		} else if m[2] != "" {
			oc.May = []string{m[2]}
		}
	}

	return oc
}

func parseAttributeType(raw string) AttributeType {
	at := AttributeType{}

	if m := oidRegex.FindStringSubmatch(raw); len(m) > 1 {
		at.OID = m[1]
	}

	if m := nameRegex.FindStringSubmatch(raw); len(m) > 1 {
		if m[1] != "" {
			at.Name = m[1]
		} else if m[2] != "" {
			names := parseList(m[2])
			if len(names) > 0 {
				at.Name = names[0]
			}
		}
	}

	if m := descRegex.FindStringSubmatch(raw); len(m) > 1 {
		at.Description = m[1]
	}

	if m := syntaxRegex.FindStringSubmatch(raw); len(m) > 1 {
		at.Syntax = m[1]
	}

	at.SingleValue = strings.Contains(raw, "SINGLE-VALUE")
	at.NoUserMod = strings.Contains(raw, "NO-USER-MODIFICATION")

	if strings.Contains(raw, "USAGE userApplications") {
		at.Usage = "userApplications"
	} else if strings.Contains(raw, "USAGE directoryOperation") {
		at.Usage = "directoryOperation"
	} else if strings.Contains(raw, "USAGE distributedOperation") {
		at.Usage = "distributedOperation"
	} else if strings.Contains(raw, "USAGE dSAOperation") {
		at.Usage = "dSAOperation"
	}

	return at
}

func parseList(s string) []string {
	var result []string
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '$' || r == ' ' || r == '\t'
	})
	for _, p := range parts {
		p = strings.Trim(p, "' ")
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (s *Schema) GetObjectClass(name string) (ObjectClass, bool) {
	oc, ok := s.ObjectClasses[strings.ToLower(name)]
	return oc, ok
}

func (s *Schema) GetAttributeType(name string) (AttributeType, bool) {
	at, ok := s.AttributeTypes[strings.ToLower(name)]
	return at, ok
}

func (s *Schema) GetAllAttributes(objectClasses []string) (must, may []string) {
	seen := make(map[string]bool)
	mustSet := make(map[string]bool)

	var traverse func(string)
	traverse = func(ocName string) {
		oc, ok := s.GetObjectClass(ocName)
		if !ok || seen[ocName] {
			return
		}
		seen[ocName] = true

		for _, m := range oc.Must {
			mustSet[m] = true
		}
		for _, m := range oc.May {
			if !mustSet[m] {
				may = append(may, m)
			}
		}

		for _, sup := range oc.Superior {
			traverse(sup)
		}
	}

	for _, ocName := range objectClasses {
		traverse(ocName)
	}

	for m := range mustSet {
		must = append(must, m)
	}

	return must, may
}
