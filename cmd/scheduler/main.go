// Command scheduler runs the backend's scheduler role (see internal/app/role.go).
package main

import "github.com/omnichannel/omni-chan-v2/internal/app"

func main() { app.Run(app.RoleScheduler) }
