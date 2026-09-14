package domain

import (
	"strconv"
	"time"

	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/helpers"
)

type DebugTrace struct {
	From           string        `json:"from"`
	To             string        `json:"to"`
	Date           time.Time     `json:"date"`
	Duration       time.Duration `json:"duration"`
	DurationString string        `json:"duration_string"`
	MemorySize     uintptr       `json:"memorySize"`
}

func NewDebugTrace(previous interfaces.TraceInterface, from, to string, values ...interface{}) interfaces.TraceInterface {
	date := time.Now()
	var duration time.Duration
	previousDate := date
	if previous != nil {
		previousDate = previous.GetDate()
	}
	duration = date.Sub(previousDate)
	usage := helpers.CalculateMemoryUsage(values, map[uintptr]bool{})
	return &DebugTrace{
		From:           from,
		To:             to,
		Date:           date,
		Duration:       duration,
		DurationString: strconv.FormatInt(duration.Nanoseconds(), 10),
		MemorySize:     usage,
	}
}

func (dt *DebugTrace) GetFrom() string {
	return dt.From
}

func (dt *DebugTrace) GetTo() string {
	return dt.To
}

func (dt *DebugTrace) GetDate() time.Time {
	return dt.Date
}

func (dt *DebugTrace) GetDuration() time.Duration {
	return dt.Duration
}

func (dt *DebugTrace) GetDurationString() string {
	return dt.DurationString
}

func (dt *DebugTrace) GetMemorySize() uintptr {
	return dt.MemorySize
}
