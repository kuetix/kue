package workflow

import "sync"

type WorkerContext interface {
	Context() *map[string]interface{}
	SetContext(context *map[string]interface{})
	Value(name string) interface{}
	SetValue(name string, value interface{})
}
type workflowContext struct {
	// mu sync.Mutex
	mu sync.RWMutex
	c  *map[string]interface{}
}

func NewWorkflowContext(context *map[string]interface{}) WorkerContext {
	if context == nil {
		context = &map[string]interface{}{}
	}
	wfContext := &workflowContext{
		c: context,
	}

	return wfContext
}

func (wfc *workflowContext) Context() *map[string]interface{} {
	return wfc.c
}

func (wfc *workflowContext) SetContext(context *map[string]interface{}) {
	wfc.c = context
}

func (wfc *workflowContext) Value(name string) interface{} {
	wfc.mu.RLock()
	i := (*wfc.c)[name]
	wfc.mu.RUnlock()
	return i
}

func (wfc *workflowContext) SetValue(name string, value interface{}) {
	wfc.mu.Lock()
	(*wfc.c)[name] = value
	wfc.mu.Unlock()
}
