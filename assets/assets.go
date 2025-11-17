//go:build windows

package assets

import (
	_ "embed"
)

//go:embed AzureAutoHibernate.png
var IconData []byte
