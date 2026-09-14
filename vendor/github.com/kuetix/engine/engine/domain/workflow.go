package domain

import (
	"github.com/kuetix/helpers"
)

type Action struct {
	ID    string `json:"id" mapstructure:"id"`
	Type  string `json:"type" mapstructure:"type"`
	Value string `json:"value" mapstructure:"value"`
}

type Workflow struct {
	ID           string   `json:"id" mapstructure:"id"`
	Title        string   `json:"title" mapstructure:"title"`
	ChannelId    string   `json:"channelId" mapstructure:"channelId"`
	ChannelTitle string   `json:"channelTitle" mapstructure:"channelTitle"`
	Trigger      string   `json:"trigger" mapstructure:"trigger"`
	Actions      []Action `json:"actions" mapstructure:"actions"`
	TemplateId   string   `json:"templateId" mapstructure:"templateId"`
}

func (w *Workflow) FromMap(record map[string]interface{}) error {
	return helpers.FromMap(w, record)
}
