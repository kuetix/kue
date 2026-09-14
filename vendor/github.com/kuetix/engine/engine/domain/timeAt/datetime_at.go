package timeAt

import "time"

type DateTimeAt struct {
	CreatedAt *time.Time `json:"createdAt" mapstructure:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt" mapstructure:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt" mapstructure:"deletedAt"`
}

func (d *DateTimeAt) JustCreated() *DateTimeAt {
	now := time.Now()
	d.CreatedAt = &now
	d.UpdatedAt = nil
	d.DeletedAt = nil

	return d
}

func (d *DateTimeAt) UpdatedAtFrom(from *DateTimeAt) *DateTimeAt {
	now := time.Now()
	if from != nil {
		d.CreatedAt = from.CreatedAt
		d.UpdatedAt = &now
		d.DeletedAt = from.DeletedAt
	} else {
		d.CreatedAt = &now
		d.UpdatedAt = nil
		d.DeletedAt = nil
	}

	return d
}
