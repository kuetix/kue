package interfaces

import "time"

type TraceInterface interface {
	GetFrom() string
	GetTo() string
	GetDate() time.Time
	GetDuration() time.Duration
}
