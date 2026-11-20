package resource_world

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// searchPageSize is the default number of matches in one search page.
const searchPageSize = 100

// searchObjectsWithType streams the objects of the request's type whose key or
// title holds the query, ignoring case. It walks the type index in steps of
// one page size, reads titles only for the step at hand, and sends each page as
// it fills, so the first page costs the index steps that fill it and the
// client's flow control bounds how far the walk runs ahead.
func searchObjectsWithType(
	ctx context.Context,
	b bus.Bus,
	ws world.WorldState,
	req *s4wave_world.SearchObjectsWithTypeRequest,
	send func(*s4wave_world.SearchObjectsWithTypeResponse) error,
) error {
	// Validate the type and choose the page size.
	typeID := req.GetTypeId()
	if typeID == "" {
		return world_types.ErrTypeIDEmpty
	}
	pageSize := req.GetPageSize()
	if pageSize == 0 {
		pageSize = searchPageSize
	}
	pageSize = min(pageSize, world_types.ObjectTypePageLimit)
	tr, err := lookupTypeTraits(ctx, b, typeID)
	if err != nil {
		return err
	}
	query := strings.ToLower(req.GetQuery())

	// Walk the type index one step at a time, sending each full page.
	page := &s4wave_world.SearchObjectsWithTypeResponse{}
	var startAfter string
	for {
		// Read one index step and what its bodies declare.
		keys, more, err := ws.ListObjectsWithType(ctx, typeID, startAfter, pageSize)
		if err != nil {
			return err
		}
		ds, err := objectDeclarations(ctx, ws, tr, keys)
		if err != nil {
			return err
		}

		// Collect the step's matches, sending the page when it fills.
		for i, key := range keys {
			var d *s4wave_world.ObjectDeclaration
			if ds != nil {
				d = ds[i]
			}
			title := d.GetTitle()
			if !strings.Contains(strings.ToLower(key), query) && !strings.Contains(strings.ToLower(title), query) {
				continue
			}
			page.ObjectKeys = append(page.ObjectKeys, key)
			if tr.traits.GetTitled() {
				page.Titles = append(page.Titles, title)
			}
			if tr.traits.GetStateful() {
				page.States = append(page.States, d.GetState())
			}
			if len(page.ObjectKeys) == int(pageSize) {
				if err := send(page); err != nil {
					return err
				}
				page = &s4wave_world.SearchObjectsWithTypeResponse{}
			}
		}

		// Stop at the end of the type, or resume after the step's last key.
		if !more || len(keys) == 0 {
			break
		}
		startAfter = keys[len(keys)-1]
	}

	// Send the last partial page.
	if len(page.ObjectKeys) == 0 {
		return nil
	}
	return send(page)
}
