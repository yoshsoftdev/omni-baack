// Command api runs the backend's api role (see internal/app/role.go).
package main

import "github.com/omnichannel/omni-chan-v2/internal/app"

func main() { app.Run(app.RoleAPI) }
