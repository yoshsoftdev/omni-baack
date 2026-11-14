// Command event-consumer runs the backend's event-consumer role (see internal/app/role.go).
package main

import "github.com/omnichannel/omni-chan-v2/internal/app"

func main() { app.Run(app.RoleEventConsumer) }
