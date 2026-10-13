package wat

import _ "embed"

// Native source attribution travels with standalone binaries as well as source.
//
//go:embed LICENSE
var License string

//go:embed ORIGIN.json
var Origin string
