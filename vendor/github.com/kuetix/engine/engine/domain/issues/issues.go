package issues

type Issues struct {
	Issues []*Issue
}

func NewIssues(issues ...*Issue) *Issues {
	if len(issues) == 0 {
		return &Issues{
			Issues: make([]*Issue, 0),
		}
	}
	return &Issues{
		Issues: issues,
	}
}

func (e *Issues) Error() string {
	result := ""
	if e == nil {
		return result
	}
	if e.Issues == nil {
		e.Issues = make([]*Issue, 0)
	}
	for _, err := range e.Issues {
		result += "\n" + err.Error()
	}

	return result
}

func (e *Issues) Errors() []*Issue {
	if e.Issues == nil {
		e.Issues = make([]*Issue, 0)
	}
	return e.Issues
}

func (e *Issues) Another(issue *Issue) {
	if e.Issues == nil {
		e.Issues = make([]*Issue, 0)
	}
	e.Issues = append(e.Issues, issue)
}

func (e *Issues) More(issues ...*Issue) {
	if e.Issues == nil {
		e.Issues = make([]*Issue, 0)
	}
	e.Issues = append(e.Issues, issues...)
}

func (e *Issues) HasIssues() bool {
	if e.Issues == nil {
		e.Issues = make([]*Issue, 0)
	}
	return len(e.Issues) > 0
}
