// Package scripts embeds the host bootstrap script template.
package scripts

import _ "embed"

// BootstrapTemplate is scripts/bootstrap.sh.tmpl (Go text/template).
//
//go:embed bootstrap.sh.tmpl
var BootstrapTemplate string
