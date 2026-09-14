package domain

type Route struct {
	Method         string             `json:"method"`
	Uri            string             `json:"uri"`
	Name           string             `json:"name"`
	Workflow       string             `json:"workflow"`
	ConfigName     string             `json:"configName"`
	ResponseType   string             `json:"responseType"`
	HandleWorkflow func() interface{} `json:"handleWorkflow"`
}
