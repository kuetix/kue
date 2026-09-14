package domain

import (
	"time"

	"github.com/kuetix/engine/engine/domain/interfaces"
)

type Trace struct {
	From     string        `json:"from"`
	To       string        `json:"to"`
	Date     time.Time     `json:"date"`
	Duration time.Duration `json:"duration"`
}

//goland:noinspection GoUnusedParameter
func NewTrace(previous interfaces.TraceInterface, from, to string, values ...interface{}) interfaces.TraceInterface {
	date := time.Now()
	var duration time.Duration
	previousDate := date
	if previous != nil {
		previousDate = previous.GetDate()
	}
	duration = date.Sub(previousDate)

	return &Trace{
		From:     from,
		To:       to,
		Date:     date,
		Duration: duration,
	}
}

func (t *Trace) GetFrom() string {
	return t.From
}

func (t *Trace) GetTo() string {
	return t.To
}

func (t *Trace) GetDate() time.Time {
	return t.Date
}

func (t *Trace) GetDuration() time.Duration {
	return t.Duration
}
