package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"vibesort/internal/inventory"
)

// Plan is a complete post-preamble entity order for one source file.
type Plan struct {
	SchemaVersion int                   `json:"schemaVersion"`
	File          string                `json:"file"`
	Order         []inventory.OrderItem `json:"order"`
}

// Result is the successful validation response emitted by the CLI.
type Result struct {
	Valid       bool   `json:"valid"`
	File        string `json:"file"`
	EntityCount int    `json:"entityCount"`
}

type rawPlan struct {
	SchemaVersion *int            `json:"schemaVersion"`
	File          *string         `json:"file"`
	Order         *[]rawOrderItem `json:"order"`
}

type rawOrderItem struct {
	ID *string `json:"id"`
}

func (p *rawPlan) UnmarshalJSON(data []byte) error {
	if err := rejectUnknownOrDuplicateFields(data, map[string]struct{}{
		"schemaVersion": {},
		"file":          {},
		"order":         {},
	}); err != nil {
		return err
	}

	type rawPlanAlias rawPlan
	return json.Unmarshal(data, (*rawPlanAlias)(p))
}

func (item *rawOrderItem) UnmarshalJSON(data []byte) error {
	if err := rejectUnknownOrDuplicateFields(data, map[string]struct{}{
		"id": {},
	}); err != nil {
		return err
	}

	type rawOrderItemAlias rawOrderItem
	return json.Unmarshal(data, (*rawOrderItemAlias)(item))
}

func rejectUnknownOrDuplicateFields(data []byte, allowed map[string]struct{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		var obj map[string]json.RawMessage
		return json.Unmarshal(data, &obj)
	}

	seen := make(map[string]struct{})
	var unknown []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("expected object field name")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = struct{}{}
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown field %q", unknown[0])
	}
	return nil
}

// Load reads and decodes a plan JSON file.
func Load(path string) (*Plan, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read plan %s: %w", path, err)
	}
	defer file.Close()

	return Decode(file)
}

// Decode reads one strict plan JSON object from r.
func Decode(r io.Reader) (*Plan, error) {
	dec := json.NewDecoder(r)

	var raw rawPlan
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode plan: %w", err)
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("plan JSON contains trailing data")
		}
		return nil, fmt.Errorf("decode plan trailing data: %w", err)
	}

	return normalize(raw)
}

func normalize(raw rawPlan) (*Plan, error) {
	if raw.SchemaVersion == nil {
		return nil, errors.New("missing schemaVersion")
	}
	if err := validateSchemaVersion(*raw.SchemaVersion); err != nil {
		return nil, err
	}
	if raw.File == nil {
		return nil, errors.New("missing file")
	}
	if err := validateFile(*raw.File); err != nil {
		return nil, err
	}
	if raw.Order == nil {
		return nil, errors.New("missing order")
	}

	order := make([]inventory.OrderItem, 0, len(*raw.Order))
	for i, item := range *raw.Order {
		if item.ID == nil {
			return nil, fmt.Errorf("missing order id at index %d", i)
		}
		order = append(order, inventory.OrderItem{ID: *item.ID})
	}
	if err := validateOrderItems(order); err != nil {
		return nil, err
	}

	return &Plan{
		SchemaVersion: *raw.SchemaVersion,
		File:          *raw.File,
		Order:         order,
	}, nil
}

func validateFields(p *Plan) error {
	if p == nil {
		return errors.New("missing plan")
	}
	if err := validateSchemaVersion(p.SchemaVersion); err != nil {
		return err
	}
	if err := validateFile(p.File); err != nil {
		return err
	}
	return validateOrderItems(p.Order)
}

func validateSchemaVersion(version int) error {
	if version != inventory.PlanSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d", version)
	}
	return nil
}

func validateFile(file string) error {
	trimmed := strings.TrimSpace(file)
	if trimmed == "" {
		return errors.New("file must not be empty")
	}
	if trimmed != file {
		return errors.New("file must not have surrounding whitespace")
	}
	return nil
}

func validateOrderItems(order []inventory.OrderItem) error {
	if order == nil {
		return errors.New("missing order")
	}
	for i, item := range order {
		trimmed := strings.TrimSpace(item.ID)
		if trimmed == "" {
			return fmt.Errorf("order id at index %d must not be empty", i)
		}
		if trimmed != item.ID {
			return fmt.Errorf("order id at index %d must not have surrounding whitespace", i)
		}
	}
	return nil
}
