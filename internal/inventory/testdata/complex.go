package fixtures

import "fmt"

// Asset section
// AssetHandler handles assets.
type AssetHandler struct{}

// ErrNotFound is returned when missing.
var ErrNotFound = fmt.Errorf("missing")

const DefaultLimit = 10

type (
	Grouped struct{}
)

//go:noinline
func Slow() {}

// NewAssetHandler builds a handler.
func NewAssetHandler() *AssetHandler { return &AssetHandler{} }

func (h *AssetHandler) Create() {} // trailing create

// ---- queries ----

// List lists assets.
func (h *AssetHandler) List() {}

func init() {}
