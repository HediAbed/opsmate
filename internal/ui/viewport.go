package ui

import "charm.land/bubbles/v2/viewport"

func newViewport(width, height int) viewport.Model {
	return viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
}
