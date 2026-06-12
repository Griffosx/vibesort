package plan

import (
	"vibesort/internal/inventory"
)

// Validate checks p against the current contents of its target source file.
func Validate(p *Plan) (*Result, error) {
	if err := validateFields(p); err != nil {
		return nil, err
	}

	doc, err := inventory.Build(p.File)
	if err != nil {
		return nil, err
	}
	if err := inventory.ValidateOrder(doc.Entities, orderIDs(p.Order)); err != nil {
		return nil, err
	}

	return &Result{
		Valid:       true,
		File:        p.File,
		EntityCount: len(doc.Entities),
	}, nil
}

func orderIDs(order []inventory.OrderItem) []string {
	ids := make([]string, 0, len(order))
	for _, item := range order {
		ids = append(ids, item.ID)
	}
	return ids
}
