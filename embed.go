package kue

import (
	"embed"
	"os"
)

//go:embed workflows
var WorkflowsFS embed.FS

const WorkflowsFSPath = "workflows"
const CacheDir = ".kue"

var HomeDir, _ = os.UserHomeDir()
