package event

import "github.com/asaskevich/EventBus"

var Bus EventBus.Bus

func init() {
	// Create EventBus
	Bus = EventBus.New()
}
