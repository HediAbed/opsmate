package component

import "charm.land/bubbles/v2/viewport"

func NewViewport(width, height int) viewport.Model {
	return viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
}
