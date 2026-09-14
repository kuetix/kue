package issues

import (
	"reflect"

	"github.com/kuetix/helpers"
)

type Issue struct {
	Message interface{}
	Errors  []error
	Json    interface{}
}

func (wwe *Issue) Error() string {
	// if reflect.TypeOf(wwe.Message).String() == "*errors.errorString" {
	//     return wwe.Message.(error).Error()
	// }
	if reflect.TypeOf(wwe.Message).String() == "string" {
		return wwe.Message.(string)
	}

	return wwe.Message.(error).Error()
}

func (wwe *Issue) Add(err error) {
	wwe.Errors = append(wwe.Errors, err)
}

func NewIssue(message interface{}, err error, json ...map[string]interface{}) *Issue {
	var w *Issue
	if len(json) > 0 {
		w = &Issue{
			Message: message,
			Json:    helpers.MergeMapsLevel0(json...),
		}
	} else {
		w = &Issue{
			Message: message,
		}
	}
	if err != nil {
		w.Add(err)
	}
	return w
}

//goland:noinspection GoUnusedExportedFunction
func NewIssueFromError(err error, json ...map[string]interface{}) *Issue {
	var w *Issue
	if err != nil {
		if len(json) > 0 {
			w = &Issue{
				Message: err.Error(),
				Json:    helpers.MergeMapsLevel0(json...),
			}
		} else {
			w = &Issue{
				Message: err.Error(),
			}
		}
		w.Add(err)
	}
	return w
}
