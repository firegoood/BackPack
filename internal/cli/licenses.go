package cli

import (
	"encoding/json"
	"fmt"

	wat "github.com/backpack/backpack/internal/tunnel/waticmp/engine"
)

func runLicenses(args []string) Result {
	asJSON, args := takeJSONFlag(args)
	if len(args) != 0 {
		return fail(CodeUsage, "licenses takes no arguments\n")
	}
	if asJSON {
		b, _ := json.MarshalIndent(map[string]any{"native_waticmp_license": wat.License, "origin": json.RawMessage(wat.Origin)}, "", "  ")
		return ok(string(b) + "\n")
	}
	return ok(fmt.Sprintf("Native Wat ICMP adapted source license:\n%s\nSource inventory and modifications:\n%s\nFullPack's root LICENSE/NOTICE apply to the full program.\n", wat.License, wat.Origin))
}
