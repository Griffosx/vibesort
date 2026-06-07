package fixtures

//go:noinline
func KnownDirective() {}

//go:somethingmadeup
func UnknownDirective() {}

// Same-line go directive text stays ordinary.
func SameLineDirectiveText() {} //go:noinline

func TrailingGenerateOwner() {}

//go:generate echo trailing
