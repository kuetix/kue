package workflow

import (
	"errors"
	"fmt"

	"github.com/kuetix/engine/engine/domain/issues"
)

type WorkerResponse struct {
	StatusCode int            `json:"status_code" yaml:"status_code"`
	Error      *issues.Issues `json:"error" yaml:"error"`
	Response   any            `json:"response" yaml:"response"`
}

func (wr *WorkerResponse) GetError() error {
	var errs []error

	if wr.Error == nil {
		return nil
	}

	for _, e := range wr.Error.Issues {
		errs = append(errs, e)
	}

	return errors.Join(errs...)
}

func (wr *WorkerResponse) RiseAnIssueFromString(msg string, o ...map[string]interface{}) {
	issue := issues.NewIssue(msg, fmt.Errorf(msg, o), o...)
	if wr.Error == nil {
		wr.Error = issues.NewIssues(issue)
	} else {
		wr.Error.Another(issue)
	}
}

func (wr *WorkerResponse) RiseAnIssueFromError(err error, o ...map[string]interface{}) {
	issue := issues.NewIssue(err.Error(), err, o...)
	if wr.Error == nil {
		wr.Error = issues.NewIssues(issue)
	} else {
		wr.Error.Another(issue)
	}
}

func (wr *WorkerResponse) RiseAnIssue(issue *issues.Issue) {
	if wr.Error == nil {
		wr.Error = issues.NewIssues(issue)
	} else {
		wr.Error.Another(issue)
	}
}

func (wr *WorkerResponse) GetResponse() any {
	return wr.Response
}

func (wr *WorkerResponse) GetStatusCode() int {
	return wr.StatusCode
}

func (wr *WorkerResponse) IsError() bool {
	return wr.Error != nil
}

func (wr *WorkerResponse) IsSuccess() bool {
	return wr.Error == nil
}
