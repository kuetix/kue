package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/logger"
)

func execWorkflow(wfConfig domain.WorkflowConfigItem, app domain.Application, workflowName string, workflowContext *map[string]interface{}, transitions []string, options []map[string]interface{}) (HandleWorkflow, map[string]*WorkerResponse) {
	handler := NewHandleWorkflow(wfConfig, app, workflowName, transitions, options...)

	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	done := handler.ProcessWorkflow(ctx, workflowName, workflowContext)
	return handler, done
}

func ExecuteWorkflow(wfConfig domain.WorkflowConfigItem, app domain.Application, workflowName string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) (responses map[string]*WorkerResponse, err *issues.Issues) {
	_, responses = execWorkflow(wfConfig, app, workflowName, workflowContext, transitions, options)

	return responses, nil
}

func ExecuteWorkflowRoutine(wfConfig domain.WorkflowConfigItem, app domain.Application, workflowName string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) map[string]*WorkerResponse {
	handler, done := execWorkflow(wfConfig, app, workflowName, workflowContext, transitions, options)
	logger.Debug(fmt.Sprintf("[workflow] %s start", workflowName))
	logger.Debug(fmt.Sprintf("[workflow] %s stop", workflowName))
	instance := handler.GetEngine()
	flow := (*instance).GetFlow()
	if (*(*instance).GetWorker()).IsDebug() {
		logger.Debug(fmt.Sprintf("[workflow] trace: %s", flow.GetTraceString()))
	}

	return done
}

//goland:noinspection GoUnusedExportedFunction
func JsonResponse(response *WorkerResponse) string {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // <-- disables \u003e escaping
	enc.SetIndent("", "  ")

	if response.Error != nil {
		var o []interface{}
		for _, e := range response.Error.Issues {
			o = append(o, recursiveIssue(e))
		}
		err := enc.Encode(map[string]interface{}{
			"errors": o,
		})
		if err != nil {
			logger.Errorf("enc.Encode error: %s", err)
		}

		return buf.String()
	}
	if err := enc.Encode(response); err != nil {
		logger.Errorf("enc.Encode error: %s", err)
	}

	return buf.String()
}

func JsonResponses(responses map[string]*WorkerResponse) string {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false) // <-- disables \u003e escaping
	enc.SetIndent("", "  ")

	isError := false
	for _, resp := range responses {
		if resp.IsError() {
			isError = true
			break
		}
	}

	if isError {
		var o []interface{}
		for _, response := range responses {
			if response.Error != nil {
				for _, e := range response.Error.Issues {
					o = append(o, recursiveIssue(e))
				}
			}
		}
		err := enc.Encode(map[string]interface{}{
			"errors": o,
		})
		if err != nil {
			logger.Errorf("enc.Encode error: %s", err)
		}

		return buf.String()
	}

	if err := enc.Encode(responses); err != nil {
		logger.Errorf("enc.Encode error: %s", err)
	}

	return buf.String()
}

// recursiveError processes all unwrapped errors
func recursiveError(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

// recursiveErrors processes all unwrapped errors
func recursiveErrors(err []error) map[string]interface{} {
	if err == nil {
		return nil
	}

	result := make(map[string]interface{}, len(err))
	for i, e := range err {
		if reflect.TypeOf(e).String() == "*errors.joinError" {
			result[strconv.Itoa(i)] = recursiveError(errors.Unwrap(e))
		} else {
			result[strconv.Itoa(i)] = recursiveError(e)
		}
	}

	return result
}

func recursiveIssue(e *issues.Issue) map[string]interface{} {
	var o = make(map[string]interface{})
	if e.Json != nil {
		o = e.Json.(map[string]interface{})
	} else if e.Message != nil {
		o["error"] = e.Message
	} else if e.Errors != nil {
		if reflect.TypeOf(e.Errors).String() == "*issues.Issue" {
			for _, i := range e.Errors {
				o = recursiveIssue(i.(*issues.Issue))
			}
		} else if reflect.TypeOf(e.Errors).Kind() == reflect.Slice {
			o = recursiveErrors(e.Errors)
		} else {
			l := make(map[string]interface{}, len(e.Errors))
			for i, x := range e.Errors {
				l[strconv.Itoa(i)] = x.Error()
			}
			o["errors"] = l
		}
	}
	return o
}
