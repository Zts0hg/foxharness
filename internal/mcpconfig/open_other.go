//go:build !unix

package mcpconfig

import "os"

const configurationReadFlags = os.O_RDONLY
