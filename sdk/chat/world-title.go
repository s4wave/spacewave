package spacewave_chat

import "github.com/s4wave/spacewave/db/world"

// WorldTitle returns the name, which object browsers show for the object.
func (c *ChatChannel) WorldTitle() string {
	return c.GetName()
}

// _ is a type assertion
var _ world.TitledBlock = (*ChatChannel)(nil)
